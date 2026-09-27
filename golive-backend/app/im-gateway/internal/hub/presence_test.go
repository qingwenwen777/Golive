package hub_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
)

// The gateway kept a room:<id>:presence set for lucky-bag draws, which stopped
// reading it. Every join, profile update and viewer tick still wrote it (one
// Redis round trip per viewer per tick), and its "6h" expiry was 21.6µs:
// 6*60*60 passed as a time.Duration.
func TestRoom_WritesNoPresenceSet(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	broker := pubsub.NewRedis(rdb)
	t.Cleanup(func() { _ = broker.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := hub.New(ctx, broker, 10*time.Millisecond, fastFlush)

	_, err := joinRoom(h, "r1", newFakeSink("c1"), hub.ViewerProfile{UserID: "u1", User: "Luna"})
	require.NoError(t, err)
	h.UpdateViewer("r1", "c1", hub.ViewerProfile{UserID: "u1", User: "Luna 2"})
	// Let a few viewer ticks run; each persists the viewer count.
	require.Eventually(t, func() bool { return mr.Exists("roommetrics:r1") }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	require.Equal(t, []string{"roommetrics:r1"}, mr.Keys())
}
