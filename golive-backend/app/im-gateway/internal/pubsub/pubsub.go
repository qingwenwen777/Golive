// Package pubsub abstracts Redis Pub/Sub so the hub doesn't import redis
// directly. This is also the seam for the unit tests — they swap in a
// channel-backed in-memory broker.
package pubsub

import (
	"context"
	"errors"

	"github.com/go-redis/redis/v9"
)

// Broker is the contract the hub depends on.
type Broker interface {
	// Subscribe returns a *new* subscription. Each call opens its own
	// underlying redis subscription so per-room lifecycle stays isolated.
	Subscribe(ctx context.Context, channel string) (Subscription, error)
	// Publish is convenient for tests and for room-internal echoes.
	Publish(ctx context.Context, channel string, payload []byte) error
	// RecordViewerCount persists current and peak viewer counts for analytics.
	RecordViewerCount(ctx context.Context, roomID string, count int64) error
	// AddPresence marks an authenticated viewer as present in a room. Used by
	// gift-service to decide lucky-bag draw eligibility (must be in the room).
	AddPresence(ctx context.Context, roomID, userID string) error
	// RemovePresence clears a viewer's presence when they leave the room.
	RemovePresence(ctx context.Context, roomID, userID string) error
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

type RedisBroker struct {
	rdb *redis.Client
}

func NewRedis(rdb *redis.Client) *RedisBroker { return &RedisBroker{rdb: rdb} }

func (b *RedisBroker) Subscribe(ctx context.Context, channel string) (Subscription, error) {
	if b.rdb == nil {
		return nil, errors.New("redis client nil")
	}
	ps := b.rdb.Subscribe(ctx, channel)
	if _, err := ps.Receive(ctx); err != nil { // wait for SUBSCRIBE ack
		_ = ps.Close()
		return nil, err
	}
	out := make(chan []byte, 64)
	sub := &redisSub{ps: ps, ch: out}
	go sub.pump()
	return sub, nil
}

func (b *RedisBroker) Publish(ctx context.Context, channel string, payload []byte) error {
	return b.rdb.Publish(ctx, channel, payload).Err()
}

func (b *RedisBroker) RecordViewerCount(ctx context.Context, roomID string, count int64) error {
	return luaRecordViewerCount.Run(ctx, b.rdb, []string{"roommetrics:" + roomID}, count, 30*24*60*60).Err()
}

func presenceKey(roomID string) string { return "room:" + roomID + ":presence" }

func (b *RedisBroker) AddPresence(ctx context.Context, roomID, userID string) error {
	if b.rdb == nil || roomID == "" || userID == "" {
		return nil
	}
	pipe := b.rdb.Pipeline()
	pipe.SAdd(ctx, presenceKey(roomID), userID)
	pipe.Expire(ctx, presenceKey(roomID), 6*60*60)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) RemovePresence(ctx context.Context, roomID, userID string) error {
	if b.rdb == nil || roomID == "" || userID == "" {
		return nil
	}
	return b.rdb.SRem(ctx, presenceKey(roomID), userID).Err()
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

type redisSub struct {
	ps *redis.PubSub
	ch chan []byte
}

func (s *redisSub) pump() {
	defer close(s.ch)
	in := s.ps.Channel()
	for msg := range in {
		// Best-effort enqueue. Dropping here is fine: the hub-side fan-out
		// has its own buffer, and the broadcast path is "at most once".
		select {
		case s.ch <- []byte(msg.Payload):
		default:
		}
	}
}

func (s *redisSub) Channel() <-chan []byte { return s.ch }
func (s *redisSub) Close() error           { return s.ps.Close() }
