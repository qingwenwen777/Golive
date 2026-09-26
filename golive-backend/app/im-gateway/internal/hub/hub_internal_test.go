package hub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
)

// gateBroker blocks Subscribe for channels listed in gates until the gate is
// closed, and fails channels listed in fail.
type gateBroker struct {
	mu    sync.Mutex
	gates map[string]chan struct{}
	fail  map[string]error
	subs  map[string][]*gateSub
}

func newGateBroker() *gateBroker {
	return &gateBroker{gates: map[string]chan struct{}{}, fail: map[string]error{}, subs: map[string][]*gateSub{}}
}

func (b *gateBroker) Subscribe(ctx context.Context, channel string) (pubsub.Subscription, error) {
	b.mu.Lock()
	gate, err := b.gates[channel], b.fail[channel]
	b.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	s := &gateSub{ch: make(chan []byte, 16)}
	b.mu.Lock()
	b.subs[channel] = append(b.subs[channel], s)
	b.mu.Unlock()
	return s, nil
}

func (b *gateBroker) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subs[channel] {
		s.deliver(payload)
	}
	return nil
}

func (b *gateBroker) RecordViewerCount(context.Context, string, int64) error { return nil }

type gateSub struct {
	mu     sync.Mutex
	ch     chan []byte
	closed bool
}

func (s *gateSub) deliver(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.ch <- p
	}
}
func (s *gateSub) Channel() <-chan []byte { return s.ch }
func (s *gateSub) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
	return nil
}

type sink struct {
	id  string
	mu  sync.Mutex
	got []string
}

func (s *sink) ID() string { return s.id }
func (s *sink) Send(p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, string(p))
	return true
}
func (s *sink) Close() {}
func (s *sink) received(want string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.got {
		if p == want {
			return true
		}
	}
	return false
}

// The last viewer leaving while the owner joins used to tear the room down
// (the emptiness re-check used the viewer count, which excludes owners),
// leaving the owner connected to a room with no subscription.
func TestHub_OwnerJoiningAsLastViewerLeavesIsNotOrphaned(t *testing.T) {
	br := newGateBroker()
	h := New(context.Background(), br, 0)

	viewer := &sink{id: "viewer"}
	r, err := h.Join("R1", viewer, ViewerProfile{UserID: "v1", User: "V"})
	require.NoError(t, err)

	// Interleave Leave by hand: remove the viewer, let the owner join, then
	// run the reaping half of Leave.
	removed, empty := r.remove("viewer")
	require.True(t, removed)
	require.True(t, empty)
	owner := &sink{id: "owner"}
	r2, err := h.Join("R1", owner, ViewerProfile{UserID: "o1", User: "Host", IsOwner: true})
	require.NoError(t, err)
	require.Same(t, r, r2)
	h.reapIfEmpty(r)

	require.Equal(t, 1, h.RoomCount(), "room with the owner must survive")
	require.NoError(t, h.Broadcast(context.Background(), "R1", []byte("to-owner")))
	require.Eventually(t, func() bool { return owner.received("to-owner") }, time.Second, 5*time.Millisecond)
}

// A join racing with the reap of an empty room must end up in a live room.
func TestHub_JoinAfterReapGetsFreshRoom(t *testing.T) {
	br := newGateBroker()
	h := New(context.Background(), br, 0)

	a := &sink{id: "a"}
	r, err := h.Join("R1", a, ViewerProfile{})
	require.NoError(t, err)
	_, empty := r.remove("a")
	require.True(t, empty)
	h.reapIfEmpty(r)
	require.False(t, r.add(&sink{id: "late"}, ViewerProfile{}), "reaped room refuses members")

	b := &sink{id: "b"}
	r2, err := h.Join("R1", b, ViewerProfile{})
	require.NoError(t, err)
	require.NotSame(t, r, r2)
	require.NoError(t, h.Broadcast(context.Background(), "R1", []byte("hi")))
	require.Eventually(t, func() bool { return b.received("hi") }, time.Second, 5*time.Millisecond)
}

// The hub lock used to be held across the broker SUBSCRIBE round trip, so a
// slow subscribe for one room blocked joins and leaves in every room.
func TestHub_SlowSubscribeDoesNotBlockOtherRooms(t *testing.T) {
	br := newGateBroker()
	gate := make(chan struct{})
	br.gates[pubsub.RoomChannel("slow")] = gate
	h := New(context.Background(), br, 0)

	slowDone := make(chan error, 1)
	go func() {
		_, err := h.Join("slow", &sink{id: "s"}, ViewerProfile{})
		slowDone <- err
	}()
	// Let the slow join reach the broker.
	time.Sleep(20 * time.Millisecond)

	fastDone := make(chan error, 1)
	go func() {
		_, err := h.Join("fast", &sink{id: "f"}, ViewerProfile{})
		fastDone <- err
	}()
	select {
	case err := <-fastDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("join of an unrelated room blocked behind a slow subscribe")
	}

	close(gate)
	require.NoError(t, <-slowDone)
	require.Equal(t, 2, h.RoomCount())
}

func TestHub_SubscribeFailureIsReturnedAndRetried(t *testing.T) {
	br := newGateBroker()
	br.fail[pubsub.RoomChannel("R1")] = errors.New("redis down")
	h := New(context.Background(), br, 0)

	_, err := h.Join("R1", &sink{id: "a"}, ViewerProfile{})
	require.Error(t, err)
	require.Equal(t, 0, h.RoomCount())

	br.mu.Lock()
	delete(br.fail, pubsub.RoomChannel("R1"))
	br.mu.Unlock()
	_, err = h.Join("R1", &sink{id: "b"}, ViewerProfile{})
	require.NoError(t, err)
	require.Equal(t, 1, h.RoomCount())
}

// The gift-service outbox publishes at least once; a redelivered event (same
// eventId) must not be counted into the leaderboard again.
func TestRoom_ContributionIgnoresRedeliveredEvent(t *testing.T) {
	r := newRoom(context.Background(), nil, "R1")
	gift := func(eventID int) []byte {
		return []byte(fmt.Sprintf(`{"type":"gift","userId":"u1","user":"A","totalCoin":500,"eventId":%d}`, eventID))
	}
	r.applyContribution(gift(1))
	r.applyContribution(gift(1))
	r.applyContribution(gift(2))
	r.applyContribution([]byte(`{"type":"super_chat","userId":"u1","amount":"¥30","eventId":2}`))
	// Events without an id (older publishers) are always counted.
	r.applyContribution([]byte(`{"type":"gift","userId":"u1","totalCoin":100}`))
	r.applyContribution([]byte(`{"type":"gift","userId":"u1","totalCoin":100}`))
	require.Equal(t, int64(1200), r.contributions["id:u1"])

	// Only the most recent ids are remembered.
	for id := 3; id < 3+maxRecentEventIDs; id++ {
		require.True(t, r.appliedEvents.add(uint64(id)))
	}
	require.Len(t, r.appliedEvents.seen, maxRecentEventIDs)
	require.True(t, r.appliedEvents.add(1), "evicted id is new again")
	require.False(t, r.appliedEvents.add(uint64(2+maxRecentEventIDs)))
}
