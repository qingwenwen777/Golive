package chatlimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/chatlimit"
)

// windowStart is a time at the start of every window the tests use, so calls
// made "at the same time" share a window however long they take.
var windowStart = time.Unix(1_700_000_000, 0)

// newLimiter returns a limiter whose clock reads *now, starting at
// windowStart; tests advance it to move to a later window.
func newLimiter(t *testing.T, limit int, window time.Duration) (*chatlimit.Limiter, *miniredis.Miniredis, *time.Time) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := chatlimit.New(rdb, "rl:test:", limit, window)
	now := windowStart
	l.SetNow(func() time.Time { return now })
	return l, mr, &now
}

func TestLimiter_AllowsUnderLimit(t *testing.T) {
	l, _, _ := newLimiter(t, 3, time.Second)
	for i := 0; i < 3; i++ {
		ok, err := l.Allow(context.Background(), "u1")
		require.NoError(t, err)
		require.Truef(t, ok, "request %d should be allowed", i+1)
	}
}

func TestLimiter_DeniesOverLimit(t *testing.T) {
	l, _, _ := newLimiter(t, 3, time.Second)
	for i := 0; i < 3; i++ {
		_, _ = l.Allow(context.Background(), "u1")
	}
	ok, err := l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLimiter_PerUserIsolation(t *testing.T) {
	l, _, _ := newLimiter(t, 1, time.Second)
	ok1, _ := l.Allow(context.Background(), "u1")
	ok2, _ := l.Allow(context.Background(), "u2")
	require.True(t, ok1)
	require.True(t, ok2, "different users have separate buckets")
}

func TestLimiter_PrefixesAreIndependent(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	a := chatlimit.New(rdb, "rl:a:", 1, time.Second)
	b := chatlimit.New(rdb, "rl:b:", 1, time.Second)
	a.SetNow(func() time.Time { return windowStart })
	b.SetNow(func() time.Time { return windowStart })
	ok, _ := a.Allow(context.Background(), "u1")
	require.True(t, ok)
	ok, _ = b.Allow(context.Background(), "u1")
	require.True(t, ok, "a different prefix has its own budget")
}

// A nanosecond window (what `bucket_seconds: 1` used to decode to) put every
// call in its own bucket, so nothing was ever limited.
func TestLimiter_SubMillisecondWindowStillLimits(t *testing.T) {
	l, _, _ := newLimiter(t, 1, time.Nanosecond)
	ok, err := l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = l.Allow(context.Background(), "u1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLimiter_ResetsAcrossWindow(t *testing.T) {
	l, mr, now := newLimiter(t, 1, 100*time.Millisecond)
	ok, _ := l.Allow(context.Background(), "u1")
	require.True(t, ok)
	ok, _ = l.Allow(context.Background(), "u1")
	require.False(t, ok)

	// Move the limiter's clock and miniredis past the window.
	*now = now.Add(150 * time.Millisecond)
	mr.FastForward(150 * time.Millisecond)
	ok, _ = l.Allow(context.Background(), "u1")
	require.True(t, ok)
}
