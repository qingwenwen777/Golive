// Package pubsub abstracts Redis Pub/Sub so the hub doesn't import redis
// directly. This is also the seam for the unit tests — they swap in a
// channel-backed in-memory broker.
package pubsub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
)

// Broker is the contract the hub depends on.
type Broker interface {
	// Subscribe returns a *new* subscription whose Close affects only that
	// subscription. Implementations may share one underlying connection;
	// ctx bounds the subscribe call, not the subscription's lifetime.
	Subscribe(ctx context.Context, channel string) (Subscription, error)
	// Publish is convenient for tests and for room-internal echoes.
	Publish(ctx context.Context, channel string, payload []byte) error
	// RecordViewerCount persists current and peak viewer counts for analytics.
	RecordViewerCount(ctx context.Context, roomID string, count int64) error
}

// Subscription is the receive side. Channel() yields raw payload bytes.
// Close() releases the underlying redis subscription.
type Subscription interface {
	Channel() <-chan []byte
	Close() error
}

// RoomChannel returns the redis pub/sub channel name for a room.
func RoomChannel(roomID string) string { return "room:" + roomID }

// --- Redis implementation ---------------------------------------------

// RedisBroker multiplexes every room subscription over ONE shared Redis
// PubSub connection. A dedicated connection per room let a single client
// open one Redis connection per distinct roomId and exhaust the server's
// maxclients, which every service shares. go-redis re-subscribes all
// channels on the shared connection after a reconnect.
type RedisBroker struct {
	rdb *redis.Client

	// opMu serialises SUBSCRIBE/UNSUBSCRIBE decisions and writes so a room
	// being torn down and a new room for the same id can't reorder them.
	opMu sync.Mutex
	ps   *redis.PubSub // created lazily under opMu

	// mu guards subs; the dispatcher only needs it briefly per message.
	mu   sync.RWMutex
	subs map[string]map[*redisSub]struct{}
}

func NewRedis(rdb *redis.Client) *RedisBroker {
	return &RedisBroker{rdb: rdb, subs: make(map[string]map[*redisSub]struct{})}
}

// Subscribe registers a local subscription and, if it is the first for the
// channel, sends SUBSCRIBE on the shared connection. ctx bounds that write
// only; the subscription lives until Close.
func (b *RedisBroker) Subscribe(ctx context.Context, channel string) (Subscription, error) {
	if b.rdb == nil {
		return nil, errors.New("redis client nil")
	}
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if b.ps == nil {
		b.ps = b.rdb.Subscribe(context.Background())
		go b.dispatch(b.ps.Channel(redis.WithChannelSize(1024)))
	}

	sub := &redisSub{broker: b, channel: channel, ch: make(chan []byte, 64)}
	b.mu.Lock()
	set := b.subs[channel]
	first := len(set) == 0
	if set == nil {
		set = make(map[*redisSub]struct{})
		b.subs[channel] = set
	}
	set[sub] = struct{}{}
	b.mu.Unlock()

	if first {
		if err := b.ps.Subscribe(ctx, channel); err != nil {
			// go-redis remembers the channel even when the write fails;
			// drop it again so a reconnect doesn't resurrect it.
			b.removeLocked(sub)
			return nil, err
		}
	}
	return sub, nil
}

// removeLocked drops sub and unsubscribes the channel when it was the last
// local subscriber. Caller holds opMu.
func (b *RedisBroker) removeLocked(sub *redisSub) {
	b.mu.Lock()
	set := b.subs[sub.channel]
	delete(set, sub)
	last := len(set) == 0
	if last {
		delete(b.subs, sub.channel)
	}
	b.mu.Unlock()
	// No dispatcher can reach sub any more, so closing its channel is safe.
	close(sub.ch)
	if last {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = b.ps.Unsubscribe(ctx, sub.channel)
	}
}

// dispatch routes messages from the shared connection to local subscribers.
// Delivery is best-effort: a full subscriber buffer drops the message rather
// than stalling every other room.
func (b *RedisBroker) dispatch(in <-chan *redis.Message) {
	for msg := range in {
		payload := []byte(msg.Payload)
		b.mu.RLock()
		for sub := range b.subs[msg.Channel] {
			select {
			case sub.ch <- payload:
			default:
			}
		}
		b.mu.RUnlock()
	}
}

// Close releases the shared PubSub connection.
func (b *RedisBroker) Close() error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if b.ps == nil {
		return nil
	}
	return b.ps.Close()
}

func (b *RedisBroker) Publish(ctx context.Context, channel string, payload []byte) error {
	return b.rdb.Publish(ctx, channel, payload).Err()
}

func (b *RedisBroker) RecordViewerCount(ctx context.Context, roomID string, count int64) error {
	return luaRecordViewerCount.Run(ctx, b.rdb, []string{"roommetrics:" + roomID}, count, 30*24*60*60).Err()
}

var luaRecordViewerCount = redis.NewScript(`
local count = tonumber(ARGV[1]) or 0
redis.call('HSET', KEYS[1], 'viewers', count)
local peak = tonumber(redis.call('HGET', KEYS[1], 'peak') or '0')
if count > peak then
  redis.call('HSET', KEYS[1], 'peak', count)
end
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]) or 2592000)
return 1
`)

// redisSub is one local subscriber on the shared connection.
type redisSub struct {
	broker  *RedisBroker
	channel string
	ch      chan []byte
	once    sync.Once
}

func (s *redisSub) Channel() <-chan []byte { return s.ch }

// Close is idempotent. The channel returned by Channel is closed.
func (s *redisSub) Close() error {
	s.once.Do(func() {
		s.broker.opMu.Lock()
		defer s.broker.opMu.Unlock()
		s.broker.removeLocked(s)
	})
	return nil
}
