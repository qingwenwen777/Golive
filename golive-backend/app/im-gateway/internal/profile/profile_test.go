package profile

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/internalauth"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newResolver(t *testing.T, users, chat http.HandlerFunc) (*HTTPResolver, *clock) {
	t.Helper()
	cfg := Config{InternalToken: "internal-tok"}
	if users != nil {
		srv := httptest.NewServer(users)
		t.Cleanup(srv.Close)
		cfg.UserServiceURL = srv.URL
	}
	if chat != nil {
		srv := httptest.NewServer(chat)
		t.Cleanup(srv.Close)
		cfg.ChatServiceURL = srv.URL
	}
	r := NewHTTPResolver(cfg)
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	r.now = c.now
	return r, c
}

func TestProfile_ResolvedFromUserService(t *testing.T) {
	var calls atomic.Int32
	r, clk := newResolver(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		require.Equal(t, "/users/profile/u-1", req.URL.Path)
		require.Empty(t, req.Header.Get(internalauth.Header), "public profile calls carry no internal token")
		_, _ = w.Write([]byte(`{"id":"u-1","username":"luna","displayName":"Luna","avatar":"/a.png","levelInfo":{"level":7}}`))
	}, nil)

	p, ok := r.Profile(context.Background(), "u-1")
	require.True(t, ok)
	require.Equal(t, Profile{UserID: "u-1", Name: "Luna", Avatar: "/a.png", Level: 7}, p)

	_, _ = r.Profile(context.Background(), "u-1")
	require.EqualValues(t, 1, calls.Load(), "cached within TTL")

	clk.t = clk.t.Add(time.Minute)
	_, _ = r.Profile(context.Background(), "u-1")
	require.EqualValues(t, 2, calls.Load(), "refetched after TTL")
}

func TestProfile_FallbacksNeverUseClientData(t *testing.T) {
	var fail atomic.Bool
	r, clk := newResolver(t, func(w http.ResponseWriter, req *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		switch req.URL.Path {
		case "/users/profile/u-1":
			_, _ = w.Write([]byte(`{"id":"u-1","displayName":"Luna","levelInfo":{"level":3}}`))
		case "/users/profile/by-name":
			// user-service also resolves usernames; a different id must not count.
			_, _ = w.Write([]byte(`{"id":"someone-else","displayName":"Streamer","levelInfo":{"level":99}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}, nil)
	lookup := func(userID string) (Profile, bool) { return r.Profile(context.Background(), userID) }

	// user-service's answer, even when it has no such profile.
	p, ok := lookup("by-name")
	require.Equal(t, Profile{UserID: "by-name", Name: "Creator by-name"}, p)
	require.True(t, ok)
	p, ok = lookup("missing-user")
	require.Equal(t, "Creator missing-", p.Name)
	require.True(t, ok)

	p, _ = lookup("u-1")
	require.Equal(t, "Luna", p.Name)
	fail.Store(true)
	clk.t = clk.t.Add(time.Minute)
	p, ok = lookup("u-1")
	require.Equal(t, "Luna", p.Name, "stale value served while user-service is down")
	require.True(t, ok)
	p, ok = lookup("u-9")
	require.Equal(t, Profile{UserID: "u-9", Name: "Creator u-9"}, p)
	require.False(t, ok, "a fallback served because of an error says so")
}

// A lookup that failed was cached as the fallback for ErrorTTL and served
// like any other profile, so a connection that resolved during a
// user-service restart kept "Creator <id>" for its whole life.
func TestProfile_FailedLookupIsRetriedAfterBackoff(t *testing.T) {
	var fail atomic.Bool
	var calls atomic.Int32
	fail.Store(true)
	r, clk := newResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"id":"u-1","displayName":"Luna","levelInfo":{"level":3}}`))
	}, nil)

	p, ok := r.Profile(context.Background(), "u-1")
	require.Equal(t, "Creator u-1", p.Name)
	require.False(t, ok)

	fail.Store(false)
	_, ok = r.Refresh(context.Background(), "u-1")
	require.False(t, ok, "backing off: neither call hammers the service")
	require.EqualValues(t, 1, calls.Load())

	clk.t = clk.t.Add(5 * time.Second)
	p, ok = r.Profile(context.Background(), "u-1")
	require.Equal(t, Profile{UserID: "u-1", Name: "Luna", Level: 3}, p)
	require.True(t, ok)
	require.EqualValues(t, 2, calls.Load())
}

// viewer_profile (sent after the client edits its profile) went through the
// 30s cache, so a rename didn't show until the entry expired.
func TestRefresh_RefetchesAtMostEveryMinRefresh(t *testing.T) {
	var name atomic.Value
	name.Store("Luna")
	var calls atomic.Int32
	r, clk := newResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintf(w, `{"id":"u-1","displayName":%q}`, name.Load())
	}, nil)
	current := func(p Profile, _ bool) string { return p.Name }

	require.Equal(t, "Luna", current(r.Profile(context.Background(), "u-1")))
	name.Store("Luna Moon")
	clk.t = clk.t.Add(2 * time.Second)
	require.Equal(t, "Luna", current(r.Refresh(context.Background(), "u-1")), "rate limited")
	require.EqualValues(t, 1, calls.Load())

	clk.t = clk.t.Add(3 * time.Second)
	require.Equal(t, "Luna Moon", current(r.Refresh(context.Background(), "u-1")))
	require.Equal(t, "Luna Moon", current(r.Profile(context.Background(), "u-1")), "Profile serves the refreshed entry")
	for i := 0; i < 20; i++ {
		_, _ = r.Refresh(context.Background(), "u-1")
	}
	require.EqualValues(t, 2, calls.Load())
}

// Every lookup fetched on its own, so a reconnect storm (or one user's many
// tabs) sent user-service one request per connection.
func TestProfile_ConcurrentLookupsShareOneFetch(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	r, _ := newResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		_, _ = w.Write([]byte(`{"id":"u-1","displayName":"Luna"}`))
	}, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, ok := r.Profile(context.Background(), "u-1")
			assert.Equal(t, "Luna", p.Name)
			assert.True(t, ok)
		}()
	}
	require.Eventually(t, func() bool { return calls.Load() > 0 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // let the other lookups queue up behind it
	close(release)
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
}

// The default transport kept 2 idle connections per host, so each burst of
// lookups opened a TCP connection per lookup and closed most of them again.
func TestProfile_BurstsReuseConnections(t *testing.T) {
	const burstSize = 64
	var newConns atomic.Int32
	// The server holds each request until the whole burst has arrived, so
	// both bursts need burstSize connections at once however the goroutines
	// are scheduled. Otherwise a slow first burst reuses its own connections,
	// leaves fewer idle ones, and the second burst has to open the rest.
	var (
		mu      sync.Mutex
		arrived int
		gate    = make(chan struct{})
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		burstDone := gate
		if arrived++; arrived == burstSize {
			close(gate)
			gate, arrived = make(chan struct{}), 0
		}
		mu.Unlock()
		select {
		case <-burstDone:
		case <-time.After(5 * time.Second):
		}
		id := strings.TrimPrefix(req.URL.Path, "/users/profile/")
		_, _ = fmt.Fprintf(w, `{"id":%q,"displayName":"N-%s"}`, id, id)
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	r := NewHTTPResolver(Config{UserServiceURL: srv.URL})

	burst := func(round int) {
		var wg sync.WaitGroup
		for i := 0; i < burstSize; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, ok := r.Profile(context.Background(), fmt.Sprintf("r%d-u%d", round, i))
				assert.True(t, ok)
			}(i)
		}
		wg.Wait()
	}
	burst(1)
	// The transport hands connections back to its pool asynchronously.
	time.Sleep(100 * time.Millisecond)
	first := newConns.Load()
	require.EqualValues(t, burstSize, first, "the first burst runs every lookup at once")
	burst(2)
	// With 2 idle connections kept, the second burst opened ~62 new ones.
	require.LessOrEqual(t, newConns.Load()-first, int32(16), "the second burst reuses the first one's connections")
}

func TestFanBadge_ResolvedFromChatService(t *testing.T) {
	var calls atomic.Int32
	r, _ := newResolver(t, nil, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Header.Get(internalauth.Header) != "internal-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch req.URL.Path {
		case "/internal/rooms/live-1/fan-badges/u-1":
			_, _ = w.Write([]byte(`{"fanBadge":{"creatorId":"owner-1","level":4}}`))
		default:
			_, _ = w.Write([]byte(`{"fanBadge":null}`))
		}
	})
	require.Equal(t, &FanBadge{CreatorID: "owner-1", Level: 4}, r.FanBadge(context.Background(), "live-1", "u-1"))
	require.Nil(t, r.FanBadge(context.Background(), "live-1", "u-2"))
	_ = r.FanBadge(context.Background(), "live-1", "u-1")
	_ = r.FanBadge(context.Background(), "live-1", "u-2")
	require.EqualValues(t, 2, calls.Load(), "hits and misses are cached")
}

func TestDisplayName(t *testing.T) {
	require.Equal(t, "Luna", DisplayName("u", " Luna ", "luna"))
	require.Equal(t, "luna", DisplayName("u", "", "luna"))
	require.Equal(t, "Creator 3f2a0c1e", DisplayName("3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f", "", "3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f"))
	require.Equal(t, "ab", DisplayName("u", "a\nb", ""))
}
