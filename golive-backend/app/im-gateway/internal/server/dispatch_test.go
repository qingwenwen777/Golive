package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/profile"
	"github.com/qingwenwen777/golive/pkg/chatfilter"
	"github.com/qingwenwen777/golive/pkg/chatlimit"
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
		done:     make(chan struct{}),
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

// fakeProfiles resolves every user to p (with the user's id) and badge.
type fakeProfiles struct {
	p     profile.Profile
	badge *profile.FanBadge
}

func (f fakeProfiles) Profile(_ context.Context, userID string) (profile.Profile, bool) {
	p := f.p
	p.UserID = userID
	return p, true
}

func (f fakeProfiles) Refresh(ctx context.Context, userID string) (profile.Profile, bool) {
	return f.Profile(ctx, userID)
}

func (f fakeProfiles) FanBadge(context.Context, string, string) *profile.FanBadge { return f.badge }

// captureBroker records what the hub publishes.
type captureBroker struct {
	memBroker
	mu        sync.Mutex
	published [][]byte
}

func (b *captureBroker) Publish(_ context.Context, _ string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, payload)
	return nil
}

func (b *captureBroker) snapshot() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([][]byte(nil), b.published...)
}

// spoofedChat carries every identity field a malicious client could set.
const spoofedChat = `{"type":"chat","text":"hello","clientId":"c-1","userId":"owner-1",` +
	`"user":"TheStreamer","avatar":"/evil.png","userLevel":99,"fanBadge":{"creatorId":"owner-1","level":99}}`

var kabun = fakeProfiles{
	p:     profile.Profile{Name: "Kabun", Avatar: "/kabun.png", Level: 5},
	badge: &profile.FanBadge{CreatorID: "owner-1", Level: 2},
}

func TestDispatch_Chat_Authenticated_PublishesToProducer(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hello"})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, "u-7", events[0].UserID)
	require.Equal(t, "Creator u-7", events[0].Username, "no profile source: id-derived name")
	require.Equal(t, "R1", events[0].RoomID)
	require.Equal(t, "hello", events[0].Text)
	require.NotZero(t, events[0].Ts)
}

// Name, avatar, level, fan badge and message id used to be taken from the
// client, so anyone could post as the streamer with level 99 and any badge,
// or reuse a visible message id to rewrite that message for every viewer.
func TestDispatch_Chat_IgnoresClientIdentity(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	c.profiles = kabun
	require.True(t, c.handleFrame(context.Background(), []byte(spoofedChat)))

	events := p.snapshot()
	require.Len(t, events, 1)
	ev := events[0]
	require.Equal(t, "u-7", ev.UserID)
	require.Equal(t, "Kabun", ev.Username)
	require.Equal(t, "/kabun.png", ev.Avatar)
	require.Equal(t, 5, ev.UserLevel)
	require.Equal(t, &producer.FanBadgePayload{CreatorID: "owner-1", Level: 2}, ev.FanBadge)
	_, err := uuid.Parse(ev.ID)
	require.NoError(t, err, "message id is server-generated")

	// Only the sender learns which id its clientId got.
	var ack hub.ChatAckMsg
	require.NoError(t, json.Unmarshal(<-c.send, &ack))
	require.Equal(t, hub.ChatAckMsg{Type: "chat_ack", ClientID: "c-1", ID: ev.ID}, ack)
}

func TestDispatch_Chat_LocalEchoBroadcastsServerIdentity(t *testing.T) {
	br := &captureBroker{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, producer.NewNoop())
	c.hub = hub.New(context.Background(), br, 0)
	c.profiles = kabun
	require.True(t, c.handleFrame(context.Background(), []byte(spoofedChat)))

	published := br.snapshot()
	require.Len(t, published, 1)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(published[0], &raw))
	require.Equal(t, "chat", raw["type"])
	require.Equal(t, "u-7", raw["userId"])
	require.Equal(t, "Kabun", raw["user"])
	require.Equal(t, "/kabun.png", raw["avatar"])
	require.EqualValues(t, 5, raw["userLevel"])
	require.Equal(t, map[string]any{"creatorId": "owner-1", "level": float64(2)}, raw["fanBadge"])
	require.NotEqual(t, "c-1", raw["id"])
	require.NotContains(t, raw, "clientId", "clientId is never broadcast")
}

// The room owner doesn't wear a badge of their own fan club.
func TestDispatch_Chat_OwnerHasNoFanBadge(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "owner-1"}, p)
	c.ownerID = "owner-1"
	c.profiles = kabun
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hi"})
	require.Len(t, p.snapshot(), 1)
	require.Nil(t, p.snapshot()[0].FanBadge)
}

func TestDispatch_ViewerProfile_IgnoresPayload(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	joinTestRoom(t, c)
	c.profiles = kabun
	require.True(t, c.handleFrame(context.Background(), []byte(`{"type":"viewer_profile","user":"TheStreamer","avatar":"/evil.png","userLevel":99}`)))
	require.Equal(t, hub.ViewerListItem{UserID: "u-7", User: "Kabun", Avatar: "/kabun.png", UserLevel: 5}, waitForViewer(t, c, "Kabun"))
}

func TestDispatch_Chat_RateLimited(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	// Burst is int(MaxMessageRate)=5; 10 sends should yield ≤6 events.
	for i := 0; i < 10; i++ {
		require.True(t, c.handleFrame(context.Background(), []byte(`{"type":"chat","text":"x"}`)))
	}
	require.LessOrEqual(t, len(p.snapshot()), 6)
}

// Only chat used to be limited, so a flood of other frames (viewer_profile
// fans out to the whole room) went straight through.
func TestHandleFrame_RateLimitsEveryType(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	for i := 0; i < 20; i++ {
		c.handleFrame(context.Background(), []byte(`{"type":"resume"}`))
	}
	require.LessOrEqual(t, len(c.send), 6, "each accepted resume frame produces one reply")
}

func TestHandleFrame_ClosesFloodingConnection(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	keepOpen := true
	for i := 0; i < 200 && keepOpen; i++ {
		keepOpen = c.handleFrame(context.Background(), []byte(`{"type":"heartbeat"}`))
	}
	require.False(t, keepOpen)
}

func seriesCount(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric)
	go func() {
		c.Collect(ch)
		close(ch)
	}()
	n := 0
	for range ch {
		n++
	}
	return n
}

// The frame type is client-controlled; using it as a label verbatim created
// one Prometheus series per distinct value.
func TestDispatch_MetricLabelIsBounded(t *testing.T) {
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "warmup-unknown"})
	before := seriesCount(metrics.MessagesReceived)
	for i := 0; i < 50; i++ {
		c.dispatchInbound(context.Background(), hub.Inbound{Type: fmt.Sprintf("junk-%d", i)})
	}
	require.Equal(t, before, seriesCount(metrics.MessagesReceived))
	require.Equal(t, "chat", inboundTypeLabel("chat"))
	require.Equal(t, "unknown", inboundTypeLabel("junk"))
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
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: text})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, text, events[0].Text)
}

// With Kafka off, chat-service's filter never ran and live chat reached
// viewers unmasked.
func TestDispatch_Chat_MasksSensitiveWords(t *testing.T) {
	p := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, p)
	c.filter = chatfilter.New([]string{"fuck", "sb"}, chatfilter.WithSkipChars(chatfilter.DefaultSkipChars))
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "you ｆ\u200bｕｃｋ, plug in the usb"})
	events := p.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, "you ***, plug in the usb", events[0].Text)
}

// The chat limit used to be per connection, so opening more sockets raised a
// user's budget. It is now per user, shared through Redis.
func TestDispatch_Chat_PerUserRateLimitSpansConnections(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	limiter := chatlimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "rl:test:", 2, time.Minute)

	p := &fakeProducer{}
	a := newTestConn(auth.Identity{UserID: "u-7"}, p)
	b := newTestConn(auth.Identity{UserID: "u-7"}, p)
	other := newTestConn(auth.Identity{UserID: "u-8"}, p)
	for _, c := range []*Conn{a, b, other} {
		c.chatLimiter = limiter
	}

	a.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "1"})
	b.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "2"})
	b.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "3"})
	other.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "4"})

	var texts []string
	for _, ev := range p.snapshot() {
		texts = append(texts, ev.Text)
	}
	require.Equal(t, []string{"1", "2", "4"}, texts)
	require.Contains(t, string(<-b.send), "too fast")
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
