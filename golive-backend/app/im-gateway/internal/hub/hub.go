// Package hub manages the room → connections topology and the broker
// subscription lifecycle. Designed to host ~50k concurrent connections per
// instance on a single Go process.
//
// Concurrency model:
//
//	rooms map           — sync.RWMutex (read-heavy: every join/leave/lookup)
//	per-room conns map  — sync.RWMutex (written on add/remove, read on fanout)
//	per-conn send chan  — single producer (fanout) / single consumer (writePump)
//
// We avoid sharding the rooms map: typical traffic concentrates on hot rooms,
// so lock contention is dominated by per-room operations, not the global map.
package hub

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
)

type Hub struct {
	ctx                context.Context
	broker             pubsub.Broker
	viewerPushInterval time.Duration

	mu    sync.RWMutex
	rooms map[string]*Room
}

func New(ctx context.Context, broker pubsub.Broker, viewerPushInterval time.Duration) *Hub {
	return &Hub{
		ctx:                ctx,
		broker:             broker,
		viewerPushInterval: viewerPushInterval,
		rooms: make(map[string]*Room),
	}
}

// Join attaches sink to roomID, creating the room (and its Redis subscription)
// if this is the first member. Returns the live Room so callers can inspect it.
func (h *Hub) Join(roomID string, c Sink) (*Room, error) {
	h.mu.Lock()
	r, ok := h.rooms[roomID]
	if !ok {
		var err error
		r, err = newRoom(h.ctx, h, roomID)
		if err != nil {
			h.mu.Unlock()
			return nil, err
		}
		h.rooms[roomID] = r
	}
	h.mu.Unlock()

	r.add(c)
	metrics.ConnectionsActive.Inc()
	metrics.ConnectionsTotal.Inc()
	return r, nil
}

// Leave detaches sink from roomID. If the room becomes empty, its Redis
// subscription is torn down.
func (h *Hub) Leave(roomID, connID string) {
	h.mu.RLock()
	r, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	empty := r.remove(connID)
	metrics.ConnectionsActive.Dec()
	if !empty {
		return
	}

	// Re-acquire under write lock and re-check size — another connection
	// could have joined between remove() returning true and us getting here.
	h.mu.Lock()
	r2, stillThere := h.rooms[roomID]
	if stillThere && r2 == r && r.size() == 0 {
		delete(h.rooms, roomID)
		h.mu.Unlock()
		r.shutdown()
		return
	}
	h.mu.Unlock()
}

// Broadcast publishes a payload to a room's broker channel. The hub itself
// receives this back via its subscription and fans out — this keeps the
// fan-out path uniform whether the message originated locally or from
// another instance.
func (h *Hub) Broadcast(ctx context.Context, roomID string, payload []byte) error {
	return h.broker.Publish(ctx, pubsub.RoomChannel(roomID), payload)
}

// SendDirect bypasses the broker and writes only to local sinks in roomID.
// Used for per-connection greetings (welcome system message, initial viewer
// count) so we never echo them to other instances.
func (h *Hub) SendDirect(roomID string, c Sink, payload []byte) bool {
	return c.Send(payload)
}

// Snapshot returns a copy of (roomID, size) tuples, sorted desc, capped at top.
// Used by /debug/rooms.
func (h *Hub) Snapshot(top int) []RoomStat {
	h.mu.RLock()
	out := make([]RoomStat, 0, len(h.rooms))
	for id, r := range h.rooms {
		out = append(out, RoomStat{ID: id, Size: r.size()})
	}
	h.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

func (h *Hub) RoomCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

type RoomStat struct {
	ID   string `json:"id"`
	Size int64  `json:"size"`
}

// encodeViewerCount is here (not in messages.go) to keep room.go from
// importing encoding/json directly — a cosmetic concern but keeps the
// fan-out hot path's imports tight.
func encodeViewerCount(n int64) []byte {
	b, _ := json.Marshal(struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}{Type: "viewer_count", Count: n})
	return b
}
