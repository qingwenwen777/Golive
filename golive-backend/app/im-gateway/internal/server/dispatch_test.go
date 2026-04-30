package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
)

// fakeProducer captures PublishChat calls.
type fakeProducer struct {
	mu     sync.Mutex
	events []producer.ChatEvent
}

func (f *fakeProducer) PublishChat(_ context.Context, ev producer.ChatEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
	return nil
}
func (f *fakeProducer) LocalEcho() bool { return false }
func (f *fakeProducer) Close() error    { return nil }
func (f *fakeProducer) snapshot() []producer.ChatEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]producer.ChatEvent, len(f.events))
	copy(out, f.events)
	return out
}

// newTestConn builds a Conn without a real websocket — readPump won't run, we
// invoke dispatchInbound directly.
func newTestConn(identity auth.Identity, p producer.Producer) *Conn {
	c := &Conn{
		id:       "c1",
		roomID:   "R1",
		identity: identity,
		send:     make(chan []byte, 16),
		producer: p,
		cfg:      WSConfig{MaxMessageRate: 5},
	}
	c.limiter = rate.NewLimiter(rate.Limit(c.cfg.MaxMessageRate), int(c.cfg.MaxMessageRate))
	return c
}

func TestDispatch_Heartbeat_Ignored(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u1"}, &fakeProducer{})
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "heartbeat"})
	select {
	case payload := <-c.send:
		t.Fatalf("heartbeat should not produce output, got %s", string(payload))
	case <-time.After(20 * time.Millisecond):
	}
}

func TestDispatch_Resume_AcksWithSystem(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u1"}, &fakeProducer{})
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "resume"})
	payload := <-c.send
	var sys hub.SystemMsg
	require.NoError(t, json.Unmarshal(payload, &sys))
	require.Equal(t, "system", sys.Type)
	require.Equal(t, "resumed", sys.Text)
}

func TestDispatch_Chat_Anonymous_Rejected(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{Anonymous: true}, p)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hi"})
	require.Empty(t, p.snapshot(), "anonymous user must not reach producer")
	// They should get a system nudge.
	payload := <-c.send
	require.Contains(t, string(payload), "login required")
}

func TestDispatch_Chat_Authenticated_PublishesToProducer(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hello", User: "Kabun"})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, "u-7", events[0].UserID)
	require.Equal(t, "Kabun", events[0].Username)
	require.Equal(t, "R1", events[0].RoomID)
	require.Equal(t, "hello", events[0].Text)
	require.NotZero(t, events[0].Ts)
}

func TestDispatch_Chat_BadUsername_DroppedFromEvent(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hello", User: "bad\nname"})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Empty(t, events[0].Username)
}

func TestDispatch_Chat_RateLimited(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	// Burst is int(MaxMessageRate)=5; 10 sends should yield ≤6 events.
	for i := 0; i < 10; i++ {
		c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "x"})
	}
	require.LessOrEqual(t, len(p.snapshot()), 6)
}

func TestDispatch_Chat_TooLong_Dropped(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: string(long)})
	require.Empty(t, p.snapshot())
}

func TestDispatch_Chat_EmojiCountsAsCharacters(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	text := "hello 😀😀😀"
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: text, User: "Kabun"})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, text, events[0].Text)
}

func TestDispatch_Unknown_Type_Dropped(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "wat"})
	select {
	case payload := <-c.send:
		t.Fatalf("unexpected output for unknown type: %s", string(payload))
	case <-time.After(20 * time.Millisecond):
	}
}
