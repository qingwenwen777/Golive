package repo

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v9"
)

var ErrStreamKeyNotFound = errors.New("stream key not found")

const streamKeyPrefix = "streamkey:"
const roomChannelPrefix = "room:"

type LiveRepo struct {
	rdb *redis.Client
}

func NewLiveRepo(rdb *redis.Client) *LiveRepo { return &LiveRepo{rdb: rdb} }

// Save binds a streamKey to a roomId for ttl. Used by POST /rooms/live.
func (r *LiveRepo) Save(ctx context.Context, key, roomID string, ttl time.Duration) error {
	return r.rdb.Set(ctx, streamKeyPrefix+key, roomID, ttl).Err()
}

// Resolve returns the roomId for a streamKey, or ErrStreamKeyNotFound. Used
// by SRS on_publish webhook.
func (r *LiveRepo) Resolve(ctx context.Context, key string) (string, error) {
	v, err := r.rdb.Get(ctx, streamKeyPrefix+key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrStreamKeyNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (r *LiveRepo) Delete(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, streamKeyPrefix+key).Err()
}

func (r *LiveRepo) PublishRoomEvent(ctx context.Context, roomID string, payload []byte) error {
	return r.rdb.Publish(ctx, roomChannelPrefix+roomID, payload).Err()
}
