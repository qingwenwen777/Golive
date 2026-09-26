package service

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTLCacheExpiresEntries(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	cache := newTTLCache[int](10*time.Second, 8)
	cache.now = func() time.Time { return now }
	loads := 0
	load := func() (int, error) {
		loads++
		return loads, nil
	}

	v, err := cache.get("k", load)
	require.NoError(t, err)
	require.Equal(t, 1, v)
	now = now.Add(9 * time.Second)
	v, _ = cache.get("k", load)
	require.Equal(t, 1, v, "fresh entries are served from the cache")
	now = now.Add(time.Second)
	v, _ = cache.get("k", load)
	require.Equal(t, 2, v, "expired entries are reloaded")
	v, _ = cache.get("other", load)
	require.Equal(t, 3, v, "keys are independent")
}

func TestTTLCacheDoesNotCacheErrors(t *testing.T) {
	cache := newTTLCache[int](time.Minute, 8)
	_, err := cache.get("k", func() (int, error) { return 0, errors.New("boom") })
	require.Error(t, err)
	v, err := cache.get("k", func() (int, error) { return 7, nil })
	require.NoError(t, err)
	require.Equal(t, 7, v)
}

func TestTTLCacheDisabled(t *testing.T) {
	var nilCache *ttlCache[int]
	calls := 0
	load := func() (int, error) { calls++; return calls, nil }
	v, _ := nilCache.get("k", load)
	require.Equal(t, 1, v)
	zero := newTTLCache[int](0, 8)
	v, _ = zero.get("k", load)
	require.Equal(t, 2, v)
	v, _ = zero.get("k", load)
	require.Equal(t, 3, v)
}

func TestTTLCacheSharesConcurrentLoads(t *testing.T) {
	cache := newTTLCache[int](time.Minute, 8)
	release := make(chan struct{})
	var loads atomic.Int32
	var wg sync.WaitGroup
	results := make([]int, 20)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := cache.get("k", func() (int, error) {
				loads.Add(1)
				<-release
				return 42, nil
			})
			require.NoError(t, err)
			results[i] = v
		}(i)
	}
	// Let the goroutines queue up behind the first load.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	require.Equal(t, int32(1), loads.Load())
	for _, v := range results {
		require.Equal(t, 42, v)
	}
}

func TestTTLCacheBoundsEntries(t *testing.T) {
	cache := newTTLCache[int](time.Minute, 4)
	for i := 0; i < 50; i++ {
		_, err := cache.get(fmt.Sprintf("k%d", i), func() (int, error) { return i, nil })
		require.NoError(t, err)
	}
	require.LessOrEqual(t, len(cache.entries), 4)
}
