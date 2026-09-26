package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/rooms"
)

// tokenIsUser treats the token as the user id.
type tokenIsUser struct{}

func (tokenIsUser) Verify(token string) (auth.Identity, error) {
	if token == "" {
		return auth.Identity{}, auth.ErrMissingToken
	}
	return auth.Identity{UserID: token}, nil
}

// memBroker is a minimal in-memory pubsub.Broker.
type memBroker struct{}

type memSub struct {
	ch   chan []byte
	once sync.Once
}

func (s *memSub) Channel() <-chan []byte { return s.ch }
func (s *memSub) Close() error           { s.once.Do(func() { close(s.ch) }); return nil }

func (memBroker) Subscribe(context.Context, string) (pubsub.Subscription, error) {
	return &memSub{ch: make(chan []byte, 1)}, nil
}
func (memBroker) Publish(context.Context, string, []byte) error          { return nil }
func (memBroker) RecordViewerCount(context.Context, string, int64) error { return nil }

// fakeRooms knows a fixed set of rooms and their owners.
type fakeRooms map[string]string

func (f fakeRooms) Lookup(_ context.Context, roomID string) (rooms.Info, bool, error) {
	owner, ok := f[roomID]
	return rooms.Info{OwnerID: owner}, ok, nil
}

func newTestServer(t *testing.T, d Deps, cfg WSConfig) *httptest.Server {
	t.Helper()
	d.Hub = hub.New(context.Background(), memBroker{}, 0)
	if cfg.SendBuffer == 0 {
		cfg.SendBuffer = 16
	}
	if cfg.ReadIdleTimeout == 0 {
		cfg.ReadIdleTimeout = time.Minute
	}
	if cfg.WriteDeadline == 0 {
		cfg.WriteDeadline = time.Second
	}
	if cfg.ReadLimitBytes == 0 {
		cfg.ReadLimitBytes = 4096
	}
	srv := httptest.NewServer(NewWSHandler(d, tokenIsUser{}, cfg, "welcome"))
	t.Cleanup(srv.Close)
	return srv
}

func dial(t *testing.T, srv *httptest.Server, query url.Values) (*websocket.Conn, int) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?" + query.Encode()
	ws, resp, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		require.NotNil(t, resp, err)
		return nil, resp.StatusCode
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws, resp.StatusCode
}

func q(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

// readViewerCount reads frames until the initial viewer_count.
func readViewerCount(t *testing.T, ws *websocket.Conn) int64 {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, raw, err := ws.ReadMessage()
		require.NoError(t, err)
		var msg struct {
			Type  string `json:"type"`
			Count int64  `json:"count"`
		}
		require.NoError(t, json.Unmarshal(raw, &msg))
		if msg.Type == "viewer_count" {
			return msg.Count
		}
	}
}

func TestServeHTTP_RejectsMalformedRoomID(t *testing.T) {
	srv := newTestServer(t, Deps{}, WSConfig{})
	for _, id := range []string{"a b", "../x", strings.Repeat("r", 65), "room:1"} {
		_, code := dial(t, srv, q("roomId", id, "token", "u1"))
		require.Equal(t, http.StatusBadRequest, code, id)
	}
}

func TestServeHTTP_RejectsUnknownRoom(t *testing.T) {
	srv := newTestServer(t, Deps{Rooms: fakeRooms{"live-1": "owner-1"}}, WSConfig{RequireKnownRoom: true})
	_, code := dial(t, srv, q("roomId", "live-404", "token", "u1"))
	require.Equal(t, http.StatusNotFound, code)
	_, code = dial(t, srv, q("roomId", "live-1", "token", "u1"))
	require.Equal(t, http.StatusSwitchingProtocols, code)
}

// ownerId used to be taken from the query string, so any viewer could claim
// to be the room owner (and drop out of viewer counts and presence).
func TestServeHTTP_OwnerComesFromRoomDirectory(t *testing.T) {
	srv := newTestServer(t, Deps{Rooms: fakeRooms{"live-1": "owner-1"}}, WSConfig{RequireKnownRoom: true})

	impostor, _ := dial(t, srv, q("roomId", "live-1", "token", "u1", "ownerId", "u1"))
	require.EqualValues(t, 1, readViewerCount(t, impostor), "impostor is counted as a viewer")

	owner, _ := dial(t, srv, q("roomId", "live-1", "token", "owner-1"))
	require.EqualValues(t, 1, readViewerCount(t, owner), "the real owner is not counted")
}

func TestServeHTTP_CapsConnectionsPerUser(t *testing.T) {
	srv := newTestServer(t, Deps{}, WSConfig{MaxConnsPerUser: 2})
	a, code := dial(t, srv, q("roomId", "r1", "token", "u1"))
	require.Equal(t, http.StatusSwitchingProtocols, code)
	_, code = dial(t, srv, q("roomId", "r2", "token", "u1"))
	require.Equal(t, http.StatusSwitchingProtocols, code)
	_, code = dial(t, srv, q("roomId", "r3", "token", "u1"))
	require.Equal(t, http.StatusTooManyRequests, code)
	_, code = dial(t, srv, q("roomId", "r3", "token", "u2"))
	require.Equal(t, http.StatusSwitchingProtocols, code, "other users are unaffected")

	// Closing a connection frees its slot.
	require.NoError(t, a.Close())
	require.Eventually(t, func() bool {
		ws, code := dial(t, srv, q("roomId", "r3", "token", "u1"))
		if ws != nil {
			_ = ws.Close()
		}
		return code == http.StatusSwitchingProtocols
	}, 2*time.Second, 20*time.Millisecond)
}

func TestServeHTTP_CapsConnectionsPerIP(t *testing.T) {
	srv := newTestServer(t, Deps{}, WSConfig{MaxConnsPerIP: 2})
	for _, user := range []string{"u1", "u2"} {
		_, code := dial(t, srv, q("roomId", "r1", "token", user))
		require.Equal(t, http.StatusSwitchingProtocols, code)
	}
	_, code := dial(t, srv, q("roomId", "r1", "token", "u3"))
	require.Equal(t, http.StatusTooManyRequests, code)
}

func TestClientIP_OnlyTrustsConfiguredProxies(t *testing.T) {
	_, proxyNet, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)
	trusted := []*net.IPNet{proxyNet}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.RemoteAddr = "203.0.113.9:4000"
	req.Header.Set("X-Real-IP", "198.51.100.1")
	require.Equal(t, "203.0.113.9", clientIP(req, trusted), "untrusted peer can't pick its IP")

	req.RemoteAddr = "10.1.2.3:4000"
	require.Equal(t, "198.51.100.1", clientIP(req, trusted))

	req.Header.Del("X-Real-IP")
	req.Header.Set("X-Forwarded-For", "198.51.100.2, 10.1.2.3")
	require.Equal(t, "198.51.100.2", clientIP(req, trusted))
}
