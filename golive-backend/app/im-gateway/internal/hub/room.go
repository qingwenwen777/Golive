package hub

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
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

	mu              sync.RWMutex
	conns           map[string]Sink
	viewerProfiles  map[string]ViewerProfile
	contributions   map[string]int64
	contributionDay string

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
		id:              id,
		hub:             h,
		cancel:          cancel,
		conns:           make(map[string]Sink),
		viewerProfiles:  make(map[string]ViewerProfile),
		contributions:   make(map[string]int64),
		contributionDay: contributionBucketDay(),
		sub:             sub,
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
	r.resetContributionIfNeededLocked()
	r.viewerProfiles[c.ID()] = ViewerProfile{User: "Guest"}
	n := int64(len(r.conns))
	r.mu.Unlock()
	r.viewers.Store(n)
	r.persistViewerCount(n)
	r.broadcastViewerList()
}

// remove returns true when the room is now empty and should be destroyed.
func (r *Room) remove(connID string) bool {
	r.mu.Lock()
	if _, ok := r.conns[connID]; !ok {
		r.mu.Unlock()
		return false
	}
	delete(r.conns, connID)
	delete(r.viewerProfiles, connID)
	n := int64(len(r.conns))
	r.mu.Unlock()
	r.viewers.Store(n)
	r.persistViewerCount(n)
	r.broadcastViewerList()
	return n == 0
}

// size is used for /debug/rooms top-N.
func (r *Room) size() int64 { return r.viewers.Load() }

// Size returns the current local connection count.
func (r *Room) Size() int64 { return r.size() }

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
			r.applyContribution(payload)
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

func (r *Room) updateViewer(connID string, profile ViewerProfile) {
	r.mu.Lock()
	if _, ok := r.conns[connID]; !ok {
		r.mu.Unlock()
		return
	}
	r.resetContributionIfNeededLocked()
	if profile.User == "" {
		profile.User = "Guest"
	}
	r.viewerProfiles[connID] = profile
	r.mu.Unlock()
	r.broadcastViewerList()
}

func (r *Room) applyContribution(payload []byte) {
	var ev struct {
		Type      string `json:"type"`
		UserID    string `json:"userId"`
		User      string `json:"user"`
		Avatar    string `json:"avatar"`
		Amount    string `json:"amount"`
		TotalCoin int64  `json:"totalCoin"`
		Coins     int64  `json:"coins"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return
	}
	if ev.Type != "gift" && ev.Type != "super_chat" {
		return
	}
	coins := ev.TotalCoin
	if coins <= 0 {
		coins = ev.Coins
	}
	if coins <= 0 {
		coins = parseCoinAmount(ev.Amount)
	}
	if coins <= 0 {
		return
	}
	key := contributionKey(ViewerProfile{UserID: ev.UserID, User: ev.User})
	if key == "" {
		return
	}

	r.mu.Lock()
	r.resetContributionIfNeededLocked()
	r.contributions[key] += coins
	for connID, profile := range r.viewerProfiles {
		if contributionKey(profile) != key {
			continue
		}
		if ev.User != "" {
			profile.User = ev.User
		}
		if ev.Avatar != "" {
			profile.Avatar = ev.Avatar
		}
		if ev.UserID != "" {
			profile.UserID = ev.UserID
		}
		r.viewerProfiles[connID] = profile
	}
	r.mu.Unlock()
	r.broadcastViewerList()
}

func (r *Room) broadcastViewerList() {
	total, viewers := r.viewerListSnapshot(100)
	r.fanout(EncodeViewerList(total, viewers))
}

func (r *Room) persistViewerCount(count int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := r.hub.broker.RecordViewerCount(ctx, r.id, count); err != nil {
		logger.L().Debug("record viewer count", zap.String("room", r.id), zap.Error(err))
	}
}

func (r *Room) viewerListSnapshot(limit int) (int, []ViewerListItem) {
	r.mu.Lock()
	r.resetContributionIfNeededLocked()
	total := len(r.viewerProfiles)
	items := make([]ViewerListItem, 0, total)
	for _, profile := range r.viewerProfiles {
		user := profile.User
		if user == "" {
			user = "Guest"
		}
		items = append(items, ViewerListItem{
			UserID:       profile.UserID,
			User:         user,
			Avatar:       profile.Avatar,
			Contribution: r.contributions[contributionKey(profile)],
		})
	}
	r.mu.Unlock()

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Contribution != items[j].Contribution {
			return items[i].Contribution > items[j].Contribution
		}
		return items[i].User < items[j].User
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return total, items
}

func (r *Room) resetContributionIfNeededLocked() {
	day := contributionBucketDay()
	if r.contributionDay == day {
		return
	}
	r.contributionDay = day
	r.contributions = make(map[string]int64)
}

func contributionBucketDay() string {
	return time.Now().Local().Format("2006-01-02")
}

func contributionKey(profile ViewerProfile) string {
	if profile.UserID != "" {
		return "id:" + profile.UserID
	}
	if profile.User != "" {
		return "user:" + profile.User
	}
	return ""
}

func parseCoinAmount(amount string) int64 {
	raw := make([]rune, 0, len(amount))
	for _, r := range amount {
		if r >= '0' && r <= '9' {
			raw = append(raw, r)
		}
	}
	if len(raw) == 0 {
		return 0
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
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
			size := r.size()
			r.persistViewerCount(size)
			r.fanout(encodeViewerCount(size))
			r.broadcastViewerList()
			metrics.MessagesSent.WithLabelValues("viewer_count").Inc()
		}
	}
}

func (r *Room) shutdown() {
	r.cancel()
	_ = r.sub.Close()
	metrics.RoomsActive.Dec()
}
