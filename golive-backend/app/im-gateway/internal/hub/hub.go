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
		rooms:              make(map[string]*Room),
	}
}

// subscribeTimeout bounds the broker SUBSCRIBE for a new room.
const subscribeTimeout = 3 * time.Second

// Join attaches sink to roomID, creating the room (and its Redis subscription)
// if this is the first member. Returns the live Room so callers can inspect it.
//
// The hub lock is never held across the broker round trip: the room is
// published in the map first and concurrent joiners wait on its ready
// channel, so one slow SUBSCRIBE can't stall joins/leaves of other rooms.
func (h *Hub) Join(roomID string, c Sink, profile ViewerProfile) (*Room, error) {
	for {
		h.mu.Lock()
		r, ok := h.rooms[roomID]
		if !ok {
			r = newRoom(h.ctx, h, roomID)
			h.rooms[roomID] = r
		}
		h.mu.Unlock()
		if !ok {
			r.start()
		}

		<-r.ready
		if r.err != nil {
			h.forget(r)
			return nil, r.err
		}
		if r.add(c, profile) {
			metrics.ConnectionsActive.Inc()
			metrics.ConnectionsTotal.Inc()
			return r, nil
		}
		// The room was torn down between lookup and add (its last member
		// left); retry so we land in a live room.
	}
}

// forget removes r from the map if it is still the entry for its id.
func (h *Hub) forget(r *Room) {
	h.mu.Lock()
	if h.rooms[r.id] == r {
		delete(h.rooms, r.id)
	}
	h.mu.Unlock()
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
	removed, empty := r.remove(connID)
	if removed {
		metrics.ConnectionsActive.Dec()
	}
	if empty {
		h.reapIfEmpty(r)
	}
}

// reapIfEmpty tears r down if it still has no connections. Another
// connection could have joined between remove() and here, so the check is
// repeated under the room lock — on the connection count, not the viewer
// count, which excludes owners and would orphan an owner that just joined.
// Marking the room closed makes any later add() fail and retry.
func (h *Hub) reapIfEmpty(r *Room) {
	h.mu.Lock()
	if h.rooms[r.id] != r || !r.closeIfEmpty() {
		h.mu.Unlock()
		return
	}
	delete(h.rooms, r.id)
	h.mu.Unlock()
	r.shutdown()
}

func (h *Hub) UpdateViewer(roomID, connID string, profile ViewerProfile) {
	h.mu.RLock()
	r, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	r.updateViewer(connID, profile)
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
