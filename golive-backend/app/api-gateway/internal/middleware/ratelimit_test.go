package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimitStoreCleanupRemovesIdleBuckets(t *testing.T) {
	store := newRateLimitStore(1, 1, time.Minute, 10)
	now := time.Unix(100, 0)

	_ = store.get("192.0.2.1", now)
	_ = store.get("192.0.2.2", now.Add(30*time.Second))

	store.cleanup(now.Add(80 * time.Second))

	store.mu.Lock()
	defer store.mu.Unlock()
	require.NotContains(t, store.buckets, "192.0.2.1")
	require.Contains(t, store.buckets, "192.0.2.2")
}

func TestRateLimitStoreEvictsOldestWhenFull(t *testing.T) {
	store := newRateLimitStore(1, 1, time.Hour, 2)
	now := time.Unix(100, 0)

	_ = store.get("192.0.2.1", now)
	_ = store.get("192.0.2.2", now.Add(time.Second))
	_ = store.get("192.0.2.3", now.Add(2*time.Second))

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.buckets, 2)
	require.NotContains(t, store.buckets, "192.0.2.1")
	require.Contains(t, store.buckets, "192.0.2.2")
	require.Contains(t, store.buckets, "192.0.2.3")
}
