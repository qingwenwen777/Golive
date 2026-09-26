package repo

import (
	"context"
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
	return viewerMetricsFromValues(values), nil
}

// ViewerMetricsByRooms is ViewerMetrics for many rooms in one pipeline. Rooms
// without metrics are absent from the result.
func (r *LiveRepo) ViewerMetricsByRooms(ctx context.Context, roomIDs []string) (map[string]ViewerMetrics, error) {
	out := make(map[string]ViewerMetrics, len(roomIDs))
	if len(roomIDs) == 0 {
		return out, nil
	}
	pipe := r.rdb.Pipeline()
	cmds := make(map[string]*redis.SliceCmd, len(roomIDs))
	for _, roomID := range roomIDs {
		if _, ok := cmds[roomID]; ok {
			continue
		}
		cmds[roomID] = pipe.HMGet(ctx, "roommetrics:"+roomID, "viewers", "peak")
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	for roomID, cmd := range cmds {
		if metrics := viewerMetricsFromValues(cmd.Val()); metrics != nil {
			out[roomID] = *metrics
		}
	}
	return out, nil
}

func viewerMetricsFromValues(values []any) *ViewerMetrics {
	if len(values) < 2 || values[0] == nil && values[1] == nil {
		return nil
	}
	viewers := parseRedisInt(values[0])
	peak := parseRedisInt(values[1])
	if peak < viewers {
		peak = viewers
	}
	return &ViewerMetrics{Viewers: viewers, Peak: peak}
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
