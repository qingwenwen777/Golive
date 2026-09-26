package rooms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
)

func TestValidID(t *testing.T) {
	for _, id := range []string{"live-3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f-lx9k2m3n4o", "appt-u1-abc", "R1", "room_1"} {
		require.True(t, ValidID(id), id)
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	for _, id := range []string{"", "-lead", "a b", "a/b", "a:b", "房间", "x\n", string(long)} {
		require.False(t, ValidID(id), id)
	}
}

func newDirectory(t *testing.T, handler http.HandlerFunc) (*RedisDirectory, *miniredis.Miniredis, *atomic.Int32) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	var calls atomic.Int32
	url := ""
	if handler != nil {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			handler(w, r)
		}))
		t.Cleanup(srv.Close)
		url = srv.URL
	}
	d := NewRedisDirectory(redis.NewClient(&redis.Options{Addr: mr.Addr()}), Config{RoomServiceURL: url})
	return d, mr, &calls
}

func TestLookup_RedisOwnerKey(t *testing.T) {
	d, mr, calls := newDirectory(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	require.NoError(t, mr.Set("room:owner:live-1", "owner-1"))

	info, found, err := d.Lookup(context.Background(), "live-1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "owner-1", info.OwnerID)
	require.Zero(t, calls.Load(), "Redis hit needs no room-service call")
}

func TestLookup_FallsBackToRoomServiceAndCaches(t *testing.T) {
	d, _, calls := newDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rooms/appt-1":
			_, _ = w.Write([]byte(`{"id":"appt-1","ownerId":"owner-2","status":"scheduled"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	for i := 0; i < 3; i++ {
		info, found, err := d.Lookup(context.Background(), "appt-1")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "owner-2", info.OwnerID)
	}
	for i := 0; i < 3; i++ {
		_, found, err := d.Lookup(context.Background(), "nope")
		require.NoError(t, err)
		require.False(t, found)
	}
	require.EqualValues(t, 2, calls.Load(), "positive and negative results are cached")
}

func TestLookup_RoomServiceErrorIsUnavailable(t *testing.T) {
	d, _, _ := newDirectory(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	_, _, err := d.Lookup(context.Background(), "live-x")
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestLookup_WithoutFallbackUnknownIsNotFound(t *testing.T) {
	d, _, _ := newDirectory(t, nil)
	_, found, err := d.Lookup(context.Background(), "live-x")
	require.NoError(t, err)
	require.False(t, found)
}

func TestLookup_FallbackIsRateLimited(t *testing.T) {
	d, _, calls := newDirectory(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	var unavailable int
	for i := 0; i < 200; i++ {
		_, _, err := d.Lookup(context.Background(), "junk-"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		if err != nil {
			require.ErrorIs(t, err, ErrUnavailable)
			unavailable++
		}
	}
	require.Positive(t, unavailable)
	require.Less(t, int(calls.Load()), 200)
}

func TestLookup_InvalidIDNeverHitsBackends(t *testing.T) {
	d, _, calls := newDirectory(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_, found, err := d.Lookup(context.Background(), "../admin")
	require.NoError(t, err)
	require.False(t, found)
	require.Zero(t, calls.Load())
}
