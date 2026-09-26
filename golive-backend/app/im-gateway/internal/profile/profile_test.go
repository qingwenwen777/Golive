package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newResolver(t *testing.T, users, chat http.HandlerFunc) (*HTTPResolver, *clock) {
	t.Helper()
	cfg := Config{}
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
		_, _ = w.Write([]byte(`{"id":"u-1","username":"luna","displayName":"Luna","avatar":"/a.png","levelInfo":{"level":7}}`))
	}, nil)

	p := r.Profile(context.Background(), "u-1")
	require.Equal(t, Profile{UserID: "u-1", Name: "Luna", Avatar: "/a.png", Level: 7}, p)

	_ = r.Profile(context.Background(), "u-1")
	require.EqualValues(t, 1, calls.Load(), "cached within TTL")

	clk.t = clk.t.Add(time.Minute)
	_ = r.Profile(context.Background(), "u-1")
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

	require.Equal(t, "Creator by-name", r.Profile(context.Background(), "by-name").Name)
	require.Zero(t, r.Profile(context.Background(), "by-name").Level)
	require.Equal(t, "Creator missing-", r.Profile(context.Background(), "missing-user").Name)

	require.Equal(t, "Luna", r.Profile(context.Background(), "u-1").Name)
	fail.Store(true)
	clk.t = clk.t.Add(time.Minute)
	require.Equal(t, "Luna", r.Profile(context.Background(), "u-1").Name, "stale value served while user-service is down")
	require.Equal(t, Profile{UserID: "u-9", Name: "Creator u-9"}, r.Profile(context.Background(), "u-9"))
}

func TestFanBadge_ResolvedFromChatService(t *testing.T) {
	var calls atomic.Int32
	r, _ := newResolver(t, nil, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
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
