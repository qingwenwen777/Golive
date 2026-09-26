package hub_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
)

// fakeBroker is an in-memory pubsub for tests. Each Subscribe gets its own
// channel; Publish records the call and broadcasts to all subscribers of
// that channel.
type fakeBroker struct {
	mu           sync.Mutex
	subs         map[string][]*fakeSub
	subscribes   int
	unsubscribes int
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{subs: make(map[string][]*fakeSub)}
}

func (b *fakeBroker) Subscribe(_ context.Context, channel string) (pubsub.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &fakeSub{
		broker:  b,
		channel: channel,
		ch:      make(chan []byte, 16),
	}
	b.subs[channel] = append(b.subs[channel], s)
	b.subscribes++
	return s, nil
}

func (b *fakeBroker) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	subs := append([]*fakeSub(nil), b.subs[channel]...)
	b.mu.Unlock()
	for _, s := range subs {
		select {
		case s.ch <- payload:
		default:
		}
	}
	return nil
}

func (b *fakeBroker) RecordViewerCount(_ context.Context, _ string, _ int64) error {
	return nil
}

func (b *fakeBroker) onUnsubscribe(channel string, s *fakeSub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[channel]
	for i, x := range subs {
		if x == s {
			b.subs[channel] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(b.subs[channel]) == 0 {
		delete(b.subs, channel)
	}
	b.unsubscribes++
}

type fakeSub struct {
	broker  *fakeBroker
	channel string
	ch      chan []byte
	closed  bool
	mu      sync.Mutex
}

func (s *fakeSub) Channel() <-chan []byte { return s.ch }
func (s *fakeSub) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.ch)
	s.mu.Unlock()
	s.broker.onUnsubscribe(s.channel, s)
	return nil
}

// fakeSink records what fanout pushed to it.
type fakeSink struct {
	id       string
	mu       sync.Mutex
	received [][]byte
	closed   bool
	full     bool // simulate slow consumer
}

func newFakeSink(id string) *fakeSink { return &fakeSink{id: id} }

func (f *fakeSink) ID() string { return f.id }
func (f *fakeSink) Send(p []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || f.full {
		return false
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	f.received = append(f.received, cp)
	return true
}
func (f *fakeSink) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}
func (f *fakeSink) snapshot() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.received))
	copy(out, f.received)
	return out
}

func joinRoom(h *hub.Hub, roomID string, s *fakeSink, profiles ...hub.ViewerProfile) (*hub.Room, error) {
	profile := hub.ViewerProfile{User: "Guest"}
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	return h.Join(roomID, s, profile)
}

// helpers

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}

// fastFlush keeps change-driven viewer pushes quick in tests.
var fastFlush = hub.WithViewerFlushInterval(10 * time.Millisecond)

func countType(payloads [][]byte, typ string) int {
	n := 0
	for _, payload := range payloads {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &probe) == nil && probe.Type == typ {
			n++
		}
	}
	return n
}

// tests --------------------------------------------------------------------

// Every viewer_profile / join / leave used to broadcast viewer_count and
// viewer_list to the whole room immediately; ~130 tiny frames filled every
// viewer's 256-slot queue and evicted the room. Pushes are now coalesced.
func TestHub_ViewerPushesAreCoalesced(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, hub.WithViewerFlushInterval(200*time.Millisecond))

	watcher := newFakeSink("watcher")
	_, err := joinRoom(h, "R1", watcher, hub.ViewerProfile{UserID: "w", User: "W"})
	require.NoError(t, err)
	attacker := newFakeSink("attacker")
	_, err = joinRoom(h, "R1", attacker, hub.ViewerProfile{UserID: "a", User: "A"})
	require.NoError(t, err)

	for i := 0; i < 300; i++ {
		h.UpdateViewer("R1", "attacker", hub.ViewerProfile{UserID: "a", User: fmt.Sprintf("A%d", i)})
	}
	// The final state still reaches viewers.
	waitFor(t, func() bool {
		for _, payload := range watcher.snapshot() {
			var list hub.ViewerListMsg
			if json.Unmarshal(payload, &list) == nil && list.Type == "viewer_list" {
				for _, v := range list.Viewers {
					if v.User == "A299" {
						return true
					}
				}
			}
		}
		return false
	})
	require.LessOrEqual(t, countType(watcher.snapshot(), "viewer_list"), 4)
	require.LessOrEqual(t, countType(watcher.snapshot(), "viewer_count"), 4)
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	require.False(t, watcher.closed)
}

func TestHub_UnchangedProfileDoesNotPush(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)
	a := newFakeSink("a")
	profile := hub.ViewerProfile{UserID: "u", User: "U"}
	_, err := joinRoom(h, "R1", a, profile)
	require.NoError(t, err)
	waitFor(t, func() bool { return countType(a.snapshot(), "viewer_list") == 1 })

	for i := 0; i < 20; i++ {
		h.UpdateViewer("R1", "a", profile)
	}
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, 1, countType(a.snapshot(), "viewer_list"))
}

func TestHub_LazySubscribeFirstJoinOnly(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush) // disable viewer ticker

	a := newFakeSink("a")
	b := newFakeSink("b")

	_, err := joinRoom(h, "R1", a)
	require.NoError(t, err)
	require.Equal(t, 1, br.subscribes)

	_, err = joinRoom(h, "R1", b)
	require.NoError(t, err)
	require.Equal(t, 1, br.subscribes, "second join must reuse the existing room")

	require.Equal(t, 1, h.RoomCount())
}

func TestHub_LastLeaveTearsDownSubscription(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	a := newFakeSink("a")
	b := newFakeSink("b")
	_, _ = joinRoom(h, "R1", a)
	_, _ = joinRoom(h, "R1", b)

	h.Leave("R1", "a")
	require.Equal(t, 0, br.unsubscribes, "still has b")

	h.Leave("R1", "b")
	waitFor(t, func() bool { return br.unsubscribes == 1 })
	require.Equal(t, 0, h.RoomCount())
}

func TestHub_BroadcastFansOutToAllSinks(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	a, b, c := newFakeSink("a"), newFakeSink("b"), newFakeSink("c")
	_, _ = joinRoom(h, "R1", a)
	_, _ = joinRoom(h, "R1", b)
	_, _ = joinRoom(h, "R1", c)

	require.NoError(t, h.Broadcast(context.Background(), "R1",
		[]byte(`{"type":"chat","text":"hi"}`)))

	waitFor(t, func() bool {
		return len(a.snapshot()) >= 1 && len(b.snapshot()) >= 1 && len(c.snapshot()) >= 1
	})
}

func TestHub_BroadcastIsolatesPerRoom(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	a := newFakeSink("a")
	b := newFakeSink("b")
	_, _ = joinRoom(h, "R1", a)
	_, _ = joinRoom(h, "R2", b)

	_ = h.Broadcast(context.Background(), "R1", []byte(`hello-r1`))
	waitFor(t, func() bool { return len(a.snapshot()) >= 1 })
	for _, payload := range b.snapshot() {
		require.NotEqual(t, "hello-r1", string(payload), "R2 must not receive R1 messages")
	}
}

func TestHub_EvictsSlowConsumer(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	good := newFakeSink("good")
	slow := newFakeSink("slow")
	slow.full = true // every Send returns false

	_, _ = joinRoom(h, "R1", good)
	_, _ = joinRoom(h, "R1", slow)

	_ = h.Broadcast(context.Background(), "R1", []byte(`x`))
	waitFor(t, func() bool {
		slow.mu.Lock()
		defer slow.mu.Unlock()
		return slow.closed
	})
	require.NotEmpty(t, good.snapshot(), "good consumer still receives")
}

func TestHub_ViewerCountPushedPeriodically(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 50*time.Millisecond, fastFlush)

	a := newFakeSink("a")
	_, _ = joinRoom(h, "R1", a)

	waitFor(t, func() bool {
		for _, payload := range a.snapshot() {
			var probe struct {
				Type  string `json:"type"`
				Count int64  `json:"count"`
			}
			if json.Unmarshal(payload, &probe) == nil && probe.Type == "viewer_count" && probe.Count == 1 {
				return true
			}
		}
		return false
	})
}

func TestHub_DeduplicatesAuthenticatedViewerConnections(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	a := newFakeSink("a")
	b := newFakeSink("b")
	_, _ = joinRoom(h, "R1", a, hub.ViewerProfile{UserID: "u-1", User: "Luna"})
	_, _ = joinRoom(h, "R1", b, hub.ViewerProfile{UserID: "u-1", User: "Luna"})

	require.EqualValues(t, 1, h.Snapshot(1)[0].Size)

	var list struct {
		Type    string `json:"type"`
		Total   int    `json:"total"`
		Viewers []struct {
			UserID string `json:"userId"`
			User   string `json:"user"`
		} `json:"viewers"`
	}
	waitFor(t, func() bool {
		for _, payload := range b.snapshot() {
			if json.Unmarshal(payload, &list) == nil && list.Type == "viewer_list" && list.Total == 1 {
				return true
			}
		}
		return false
	})
	require.Len(t, list.Viewers, 1)
	require.Equal(t, "u-1", list.Viewers[0].UserID)
}

func TestHub_ExcludesOwnerFromViewerMetrics(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)

	owner := newFakeSink("owner")
	viewer := newFakeSink("viewer")
	_, _ = joinRoom(h, "R1", owner, hub.ViewerProfile{UserID: "owner-1", User: "Host", IsOwner: true})
	_, _ = joinRoom(h, "R1", viewer, hub.ViewerProfile{UserID: "viewer-1", User: "Fan"})

	require.EqualValues(t, 1, h.Snapshot(1)[0].Size)

	var list struct {
		Type    string `json:"type"`
		Total   int    `json:"total"`
		Viewers []struct {
			UserID string `json:"userId"`
			User   string `json:"user"`
		} `json:"viewers"`
	}
	waitFor(t, func() bool {
		for _, payload := range viewer.snapshot() {
			if json.Unmarshal(payload, &list) == nil && list.Type == "viewer_list" && list.Total == 1 {
				return true
			}
		}
		return false
	})
	require.Len(t, list.Viewers, 1)
	require.Equal(t, "viewer-1", list.Viewers[0].UserID)
}

func TestHub_SnapshotTopN(t *testing.T) {
	br := newFakeBroker()
	h := hub.New(context.Background(), br, 0, fastFlush)
	for _, room := range []struct {
		id string
		n  int
	}{{"a", 3}, {"b", 1}, {"c", 5}} {
		for i := 0; i < room.n; i++ {
			id := room.id + string(rune('0'+i))
			_, _ = joinRoom(h, room.id, newFakeSink(id))
		}
	}
	stats := h.Snapshot(2)
	require.Len(t, stats, 2)
	require.Equal(t, "c", stats[0].ID)
	require.EqualValues(t, 5, stats[0].Size)
	require.Equal(t, "a", stats[1].ID)
}
