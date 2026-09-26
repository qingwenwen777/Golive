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
	ctx    context.Context
	cancel context.CancelFunc

	// ready is closed once start has finished; err is its result and is
	// only read after ready.
	ready chan struct{}
	err   error

	mu              sync.RWMutex
	conns           map[string]Sink
	viewerProfiles  map[string]ViewerProfile
	contributions   map[string]int64
	contributionDay string
	// appliedEvents holds the most recent gift-service outbox event ids
	// counted into contributions. The outbox publishes at least once, so a
	// redelivered gift must not be counted twice.
	appliedEvents recentEventIDs
	// closed is set (under mu) when the hub reaps the empty room; add then
	// refuses so a joiner retries with a fresh room instead of being
	// orphaned in one without a subscription.
	closed bool

	viewers atomic.Int64
	// viewersDirty (cap 1) signals pumpViewers that count/list changed.
	viewersDirty chan struct{}

	sub pubsub.Subscription
}

func newRoom(parent context.Context, h *Hub, id string) *Room {
	ctx, cancel := context.WithCancel(parent)
	return &Room{
		id:              id,
		hub:             h,
		ctx:             ctx,
		cancel:          cancel,
		ready:           make(chan struct{}),
		viewersDirty:    make(chan struct{}, 1),
		conns:           make(map[string]Sink),
		viewerProfiles:  make(map[string]ViewerProfile),
		contributions:   make(map[string]int64),
		contributionDay: contributionBucketDay(),
	}
}

// start subscribes to the room channel and starts the pumps. Called once,
// without the hub lock held.
func (r *Room) start() {
	defer close(r.ready)
	ctx, cancel := context.WithTimeout(r.ctx, subscribeTimeout)
	sub, err := r.hub.broker.Subscribe(ctx, pubsub.RoomChannel(r.id))
	cancel()
	if err != nil {
		r.err = err
		r.cancel()
		return
	}
	r.sub = sub
	go r.pumpFromBroker(r.ctx)
	go r.pumpViewers(r.ctx, r.hub.viewerPushInterval, r.hub.viewerFlushInterval)
	metrics.RoomsActive.Inc()
}

// add attaches c. It returns false if the room has already been reaped.
func (r *Room) add(c Sink, profile ViewerProfile) bool {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return false
	}
	r.conns[c.ID()] = c
	r.resetContributionIfNeededLocked()
	if profile.User == "" {
		profile.User = "Guest"
	}
	r.viewerProfiles[c.ID()] = profile
	n := r.uniqueViewerCountLocked()
	r.mu.Unlock()
	r.viewers.Store(n)
	r.markViewersDirty()
	return true
}

// remove detaches connID. removed reports whether it was a member; empty
// whether the room now has no connections and should be reaped.
func (r *Room) remove(connID string) (removed, empty bool) {
	r.mu.Lock()
	if _, ok := r.conns[connID]; !ok {
		r.mu.Unlock()
		return false, false
	}
	delete(r.conns, connID)
	delete(r.viewerProfiles, connID)
	n := r.uniqueViewerCountLocked()
	empty = len(r.conns) == 0
	r.mu.Unlock()
	r.viewers.Store(n)
	r.markViewersDirty()
	return true, empty
}

// closeIfEmpty marks the room closed when it has no connections (owners
// included) and reports whether it did.
func (r *Room) closeIfEmpty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.conns) > 0 {
		return false
	}
	r.closed = true
	return true
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
	if r.viewerProfiles[connID] == profile {
		r.mu.Unlock()
		return
	}
	r.viewerProfiles[connID] = profile
	n := r.uniqueViewerCountLocked()
	r.mu.Unlock()
	r.viewers.Store(n)
	r.markViewersDirty()
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
		EventID   uint64 `json:"eventId"`
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
	if ev.EventID != 0 && !r.appliedEvents.add(ev.EventID) {
		r.mu.Unlock()
		return
	}
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
	r.markViewersDirty()
}

// maxRecentEventIDs bounds recentEventIDs. A redelivery follows the original
// by about one outbox claim hold (30s), far fewer events than this per room.
const maxRecentEventIDs = 1024

// recentEventIDs is a FIFO-bounded set of event ids. The zero value is ready
// to use; it is not safe for concurrent use.
type recentEventIDs struct {
	seen map[uint64]struct{}
	ring []uint64
	next int
}

// add records id and reports whether it was new.
func (s *recentEventIDs) add(id uint64) bool {
	if _, ok := s.seen[id]; ok {
		return false
	}
	if s.seen == nil {
		s.seen = make(map[uint64]struct{})
	}
	if len(s.ring) < maxRecentEventIDs {
		s.ring = append(s.ring, id)
	} else {
		delete(s.seen, s.ring[s.next])
		s.ring[s.next] = id
		s.next = (s.next + 1) % maxRecentEventIDs
	}
	s.seen[id] = struct{}{}
	return true
}

// markViewersDirty schedules a viewer_count + viewer_list push. Pushes are
// O(room size) and go to every member, so they are coalesced by
// pumpViewers to at most one per viewerFlushInterval instead of one per
// join, leave, profile update or gift — a burst of those used to fill every
// viewer's send queue and evict the whole room.
func (r *Room) markViewersDirty() {
	select {
	case r.viewersDirty <- struct{}{}:
	default:
	}
}

// flushViewers persists the viewer count and pushes count + list to the room.
func (r *Room) flushViewers() {
	n := r.size()
	r.persistViewerCount(n)
	r.fanout(encodeViewerCount(n))
	r.broadcastViewerList()
	metrics.MessagesSent.WithLabelValues("viewer_count").Inc()
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
	byViewer := make(map[string]ViewerListItem, len(r.viewerProfiles))
	for connID, profile := range r.viewerProfiles {
		key := viewerIdentityKey(connID, profile)
		if key == "" {
			continue
		}
		user := profile.User
		if user == "" {
			user = "Guest"
		}
		item := byViewer[key]
		if item.User == "" {
			item = ViewerListItem{
				UserID:    profile.UserID,
				User:      user,
				Avatar:    profile.Avatar,
				UserLevel: profile.UserLevel,
			}
		} else {
			if item.UserID == "" && profile.UserID != "" {
				item.UserID = profile.UserID
			}
			if item.User == "Guest" && user != "" {
				item.User = user
			}
			if item.Avatar == "" && profile.Avatar != "" {
				item.Avatar = profile.Avatar
			}
			if item.UserLevel == 0 && profile.UserLevel > 0 {
				item.UserLevel = profile.UserLevel
			}
		}
		item.Contribution = r.contributions[contributionKey(profile)]
		byViewer[key] = item
	}
	items := make([]ViewerListItem, 0, len(byViewer))
	for _, item := range byViewer {
		items = append(items, item)
	}
	total := len(items)
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

func (r *Room) uniqueViewerCountLocked() int64 {
	seen := make(map[string]struct{}, len(r.viewerProfiles))
	for connID, profile := range r.viewerProfiles {
		key := viewerIdentityKey(connID, profile)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	return int64(len(seen))
}

func viewerIdentityKey(connID string, profile ViewerProfile) string {
	if profile.IsOwner {
		return ""
	}
	if profile.UserID != "" {
		return "id:" + profile.UserID
	}
	if connID != "" {
		return "conn:" + connID
	}
	return ""
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

// pumpViewers pushes viewer_count + viewer_list to all sinks in this room:
// on change (markViewersDirty), at most once per minGap — the first change
// after a quiet period goes out immediately, later ones are folded into one
// trailing push — and additionally every interval (if > 0). In a
// multi-instance deployment the count should read from a Redis HINCRBY
// counter rather than the local count; MVP uses local.
func (r *Room) pumpViewers(ctx context.Context, interval, minGap time.Duration) {
	var tick <-chan time.Time
	if interval > 0 {
		t := time.NewTicker(interval)
		defer t.Stop()
		tick = t.C
	}
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.viewersDirty:
			if wait := minGap - time.Since(last); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				// Changes made while waiting are covered by this flush.
				select {
				case <-r.viewersDirty:
				default:
				}
			}
			r.flushViewers()
			last = time.Now()
		case <-tick:
			r.flushViewers()
			last = time.Now()
		}
	}
}

func (r *Room) shutdown() {
	r.cancel()
	if r.sub == nil {
		return
	}
	_ = r.sub.Close()
	metrics.RoomsActive.Dec()
}
