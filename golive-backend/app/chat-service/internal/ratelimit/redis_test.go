package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/chat-service/internal/ratelimit"
)

func newLimiter(t *testing.T, limit int, window time.Duration) (*ratelimit.Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return ratelimit.New(rdb, limit, window), mr
}

func TestLimiter_AllowsUnderLimit(t *testing.T) {
	l, _ := newLimiter(t, 3, time.Second)
	for i := 0; i < 3; i++ {
		ok, err := l.Allow(context.Background(), "u1")
		require.NoError(t, err)
		require.Truef(t, ok, "request %d should be allowed", i+1)
	}
}

func TestLimiter_DeniesOverLimit(t *testing.T) {
	l, _ := newLimiter(t, 3, time.Second)
	for i := 0; i < 3; i++ {
		_, _ = l.Allow(context.Background(), "u1")
	}
	ok, err := l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLimiter_PerUserIsolation(t *testing.T) {
	l, _ := newLimiter(t, 1, time.Second)
	ok1, _ := l.Allow(context.Background(), "u1")
	ok2, _ := l.Allow(context.Background(), "u2")
	require.True(t, ok1)
	require.True(t, ok2, "different users have separate buckets")
}

// A nanosecond window (what `bucket_seconds: 1` used to decode to) put every
// call in its own bucket, so nothing was ever limited.
func TestLimiter_SubMillisecondWindowStillLimits(t *testing.T) {
	l, _ := newLimiter(t, 1, time.Nanosecond)
	ok, err := l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLimiter_ResetsAcrossWindow(t *testing.T) {
	l, mr := newLimiter(t, 1, 100*time.Millisecond)
	ok, _ := l.Allow(context.Background(), "u1")
	require.True(t, ok)
	ok, _ = l.Allow(context.Background(), "u1")
	require.False(t, ok)

	// fast-forward miniredis past the window
	mr.FastForward(150 * time.Millisecond)
	ok, _ = l.Allow(context.Background(), "u1")
	require.True(t, ok)
}
