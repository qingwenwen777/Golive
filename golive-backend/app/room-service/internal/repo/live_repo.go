package repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/go-redis/redis/v9"

	"github.com/qingwenwen777/golive/pkg/miclink"
)

var ErrStreamKeyNotFound = errors.New("stream key not found")

const streamKeyPrefix = "streamkey:"
const streamSessionPrefix = "streamsession:"
const roomChannelPrefix = "room:"
const liveLockPrefix = "livelock:"

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

func (r *LiveRepo) SavePublishSession(ctx context.Context, key, clientID string, ttl time.Duration) error {
	if key == "" || clientID == "" {
		return nil
	}
	return r.rdb.Set(ctx, streamSessionPrefix+key, clientID, ttl).Err()
}

func (r *LiveRepo) PublishSession(ctx context.Context, key string) (string, error) {
	v, err := r.rdb.Get(ctx, streamSessionPrefix+key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrStreamKeyNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (r *LiveRepo) DeletePublishSession(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, streamSessionPrefix+key).Err()
}

// AcquireLock takes the named lock for ttl. It returns a token for
// ReleaseLock, or ok=false when someone else holds the lock.
func (r *LiveRepo) AcquireLock(ctx context.Context, name string, ttl time.Duration) (token string, ok bool, err error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	token = hex.EncodeToString(buf)
	ok, err = r.rdb.SetNX(ctx, liveLockPrefix+name, token, ttl).Result()
	if err != nil || !ok {
		return "", false, err
	}
	return token, true, nil
}

var luaReleaseLock = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// ReleaseLock releases the named lock if token still holds it, so a holder
// whose lock expired cannot release someone else's.
func (r *LiveRepo) ReleaseLock(ctx context.Context, name, token string) error {
	return luaReleaseLock.Run(ctx, r.rdb, []string{liveLockPrefix + name}, token).Err()
}

// MicLinkPublishToken returns the token gift-service issued for a mic-link
// guest stream, or ErrStreamKeyNotFound when none is active.
func (r *LiveRepo) MicLinkPublishToken(ctx context.Context, stream string) (string, error) {
	v, err := r.rdb.Get(ctx, miclink.TokenKey(stream)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrStreamKeyNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (r *LiveRepo) PublishRoomEvent(ctx context.Context, roomID string, payload []byte) error {
	return r.rdb.Publish(ctx, roomChannelPrefix+roomID, payload).Err()
}

type ViewerMetrics struct {
	Viewers int64
	Peak    int64
}

func (r *LiveRepo) ViewerMetrics(ctx context.Context, roomID string) (*ViewerMetrics, error) {
	values, err := r.rdb.HMGet(ctx, "roommetrics:"+roomID, "viewers", "peak").Result()
	if err != nil {
		return nil, err
	}
	if len(values) >= 2 && values[0] == nil && values[1] == nil {
		return nil, nil
	}
	viewers := parseRedisInt(values[0])
	peak := parseRedisInt(values[1])
	if peak < viewers {
		peak = viewers
	}
	return &ViewerMetrics{Viewers: viewers, Peak: peak}, nil
}

func parseRedisInt(value any) int64 {
	switch v := value.(type) {
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case int64:
		return v
	}
	return 0
}
