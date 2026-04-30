package hub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// Room owns the set of connections in a single live room and bridges Redis
// pub/sub fan-out into per-conn write queues. Lifecycle is "lazy":
//
//   - First connection joining triggers a Redis SUBSCRIBE.
//   - Last connection leaving triggers UNSUBSCRIBE + room destruction.
//
// Without lazy lifecycle, an idle process would carry one open redis
// subscription per ever-seen room — at scale that overwhelms redis.
type Room struct {
	id     string
	hub    *Hub
	cancel context.CancelFunc

	mu    sync.RWMutex
	conns map[string]Sink

	viewers atomic.Int64

	sub pubsub.Subscription
}

func newRoom(parent context.Context, h *Hub, id string) (*Room, error) {
	ctx, cancel := context.WithCancel(parent)
	sub, err := h.broker.Subscribe(ctx, pubsub.RoomChannel(id))
	if err != nil {
		cancel()
		return nil, err
	}
	r := &Room{
		id:     id,
		hub:    h,
		cancel: cancel,
		conns:  make(map[string]Sink),
		sub:    sub,
	}
	go r.pumpFromBroker(ctx)
	go r.pumpViewerCount(ctx, h.viewerPushInterval)
	metrics.RoomsActive.Inc()
	return r, nil
}

// add returns true if this is the first connection (caller already locked
// the hub-level rooms mutex via Hub.Join, so the room itself is reachable).
func (r *Room) add(c Sink) {
	r.mu.Lock()
	r.conns[c.ID()] = c
	n := int64(len(r.conns))
	r.mu.Unlock()
	r.viewers.Store(n)
}

// remove returns true when the room is now empty and should be destroyed.
func (r *Room) remove(connID string) bool {
	r.mu.Lock()
	if _, ok := r.conns[connID]; !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.conns, connID)
	n := int64(len(r.conns))
	r.mu.Unlock()
	r.viewers.Store(n)
	return n == 0
}

// size is exported for /debug/rooms top-N.
func (r *Room) size() int64 { return r.viewers.Load() }

// pumpFromBroker fans out broker payloads to all sinks. A slow sink whose
// send buffer is full is evicted (closed) — backpressure cannot propagate
// upstream because that would stall every other viewer in the same room.
func (r *Room) pumpFromBroker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-r.sub.Channel():
			if !ok {
				return
			}
			start := time.Now()
			r.fanout(payload)
			metrics.BroadcastLatency.Observe(time.Since(start).Seconds())
		}
	}
}

func (r *Room) fanout(payload []byte) {
	// Snapshot under read lock, send outside lock — never hold the room
	// lock across a network-bound op.
	r.mu.RLock()
	dead := []Sink(nil)
	for _, c := range r.conns {
		if !c.Send(payload) {
			dead = append(dead, c)
			metrics.MessagesDropped.WithLabelValues("slow_consumer").Inc()
		}
	}
	r.mu.RUnlock()

	if len(dead) > 0 {
		// Evict slow consumers. The conn's read loop will notice the closed
		// state and call Hub.Leave, cleaning up under the hub-level lock.
		for _, c := range dead {
			c.Close()
		}
		logger.L().Warn("evicted slow consumers", zap.String("room", r.id), zap.Int("count", len(dead)))
	}
}

// pumpViewerCount pushes the current local viewer count to all sinks in this
// room every interval. In a multi-instance deployment this should read from
// a Redis HINCRBY counter rather than the local count; MVP uses local.
func (r *Room) pumpViewerCount(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.fanout(encodeViewerCount(r.size()))
			metrics.MessagesSent.WithLabelValues("viewer_count").Inc()
		}
	}
}

func (r *Room) shutdown() {
	r.cancel()
	_ = r.sub.Close()
	metrics.RoomsActive.Dec()
}
