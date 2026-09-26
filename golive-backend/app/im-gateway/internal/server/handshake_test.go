package server

import (
	"context"
	"encoding/json"
	"io"
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
	"github.com/qingwenwen777/golive/pkg/jwtauth"
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
	return newTestServerWithVerifier(t, d, cfg, tokenIsUser{})
}

func newTestServerWithVerifier(t *testing.T, d Deps, cfg WSConfig, v auth.Verifier) *httptest.Server {
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
	srv := httptest.NewServer(NewWSHandler(d, v, cfg, "welcome"))
	t.Cleanup(srv.Close)
	return srv
}

// dial connects like the web client: the token is offered as the
// "auth.<token>" subprotocol next to golive.v1, never put in the URL. An
// empty token dials as a guest.
func dial(t *testing.T, srv *httptest.Server, query url.Values, token string) (*websocket.Conn, int) {
	t.Helper()
	protocols := []string{subprotocol}
	if token != "" {
		protocols = append(protocols, authProtocolPrefix+token)
	}
	ws, resp := dialProtocols(t, srv, query, protocols...)
	return ws, resp.StatusCode
}

// dialProtocols offers exactly the given subprotocols. The connection is nil
// when the handshake fails; the response is always set.
func dialProtocols(t *testing.T, srv *httptest.Server, query url.Values, protocols ...string) (*websocket.Conn, *http.Response) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?" + query.Encode()
	d := *websocket.DefaultDialer
	d.Subprotocols = protocols
	ws, resp, err := d.Dial(u, nil)
	if err != nil {
		require.NotNil(t, resp, err)
		return nil, resp
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws, resp
}

// accessToken signs a real access token, exactly as user-service does.
func accessToken(t *testing.T, secret, userID string) string {
	t.Helper()
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	require.NoError(t, err)
	token, err := keys.SignAccess(userID, time.Now(), time.Hour)
	require.NoError(t, err)
	return token
}

// requireTokenNotEchoed fails if the handshake response carries the token.
func requireTokenNotEchoed(t *testing.T, resp *http.Response, token string) {
	t.Helper()
	for name, values := range resp.Header {
		for _, v := range values {
			require.NotContains(t, v, token, "response header %s", name)
		}
	}
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), token)
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
		_, code := dial(t, srv, q("roomId", id), "u1")
		require.Equal(t, http.StatusBadRequest, code, id)
	}
}

// The access token used to ride in the URL (?token=), which nginx and any
// other proxy write to their access logs. It now travels as the
// "auth.<token>" subprotocol.
func TestServeHTTP_AuthenticatesWithProtocolToken(t *testing.T) {
	srv := newTestServerWithVerifier(t, Deps{}, WSConfig{MaxConnsPerUser: 1}, auth.NewHMACVerifier("secret"))
	token := accessToken(t, "secret", "u1")

	ws, resp := dialProtocols(t, srv, q("roomId", "r1"), subprotocol, authProtocolPrefix+token)
	require.NotNil(t, ws, "status %d", resp.StatusCode)
	require.Equal(t, subprotocol, ws.Subprotocol())
	requireTokenNotEchoed(t, resp, token)

	// The socket belongs to the token's user: u1's second one hits the cap.
	_, code := dial(t, srv, q("roomId", "r2"), accessToken(t, "secret", "u1"))
	require.Equal(t, http.StatusTooManyRequests, code)
	_, code = dial(t, srv, q("roomId", "r2"), accessToken(t, "secret", "u2"))
	require.Equal(t, http.StatusSwitchingProtocols, code)
}

func TestServeHTTP_RejectsBadProtocolToken(t *testing.T) {
	srv := newTestServerWithVerifier(t, Deps{}, WSConfig{}, auth.NewHMACVerifier("secret"))
	for name, token := range map[string]string{
		"not a jwt":    "not-a-jwt",
		"wrong secret": accessToken(t, "other-secret", "u1"),
	} {
		ws, resp := dialProtocols(t, srv, q("roomId", "r1"), subprotocol, authProtocolPrefix+token)
		require.Nil(t, ws, name)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, name)
		requireTokenNotEchoed(t, resp, token)
	}
}

// A token in the URL is refused, even next to a valid protocol token:
// accepting it would keep putting tokens into access logs.
func TestServeHTTP_RefusesTokenInURL(t *testing.T) {
	srv := newTestServerWithVerifier(t, Deps{}, WSConfig{}, auth.NewHMACVerifier("secret"))
	token := accessToken(t, "secret", "u1")

	ws, resp := dialProtocols(t, srv, q("roomId", "r1", "token", token))
	require.Nil(t, ws)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Sec-WebSocket-Protocol")

	ws, resp = dialProtocols(t, srv, q("roomId", "r1", "token", token), subprotocol, authProtocolPrefix+token)
	require.Nil(t, ws)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Only golive.v1 is ever selected, so the token never comes back, whatever
// order the client offers the protocols in.
func TestServeHTTP_NeverSelectsTheTokenProtocol(t *testing.T) {
	srv := newTestServerWithVerifier(t, Deps{}, WSConfig{}, auth.NewHMACVerifier("secret"))
	token := accessToken(t, "secret", "u1")

	ws, resp := dialProtocols(t, srv, q("roomId", "r1"), authProtocolPrefix+token, subprotocol)
	require.NotNil(t, ws, "status %d", resp.StatusCode)
	require.Equal(t, subprotocol, ws.Subprotocol())
	requireTokenNotEchoed(t, resp, token)

	// Offered alone, the token still authenticates, and no protocol is
	// selected (browsers only send it next to golive.v1).
	ws, resp = dialProtocols(t, srv, q("roomId", "r2"), authProtocolPrefix+token)
	require.NotNil(t, ws, "status %d", resp.StatusCode)
	require.Empty(t, ws.Subprotocol())
	requireTokenNotEchoed(t, resp, token)
}

// Guests (no token) are refused at the handshake, as they were before.
func TestServeHTTP_GuestsStillNeedAToken(t *testing.T) {
	srv := newTestServerWithVerifier(t, Deps{}, WSConfig{}, auth.NewHMACVerifier("secret"))
	for _, protocols := range [][]string{nil, {subprotocol}, {subprotocol, authProtocolPrefix}} {
		ws, resp := dialProtocols(t, srv, q("roomId", "r1"), protocols...)
		require.Nil(t, ws, protocols)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, protocols)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "missing token", protocols)
	}
}

func TestServeHTTP_RejectsUnknownRoom(t *testing.T) {
	srv := newTestServer(t, Deps{Rooms: fakeRooms{"live-1": "owner-1"}}, WSConfig{RequireKnownRoom: true})
	_, code := dial(t, srv, q("roomId", "live-404"), "u1")
	require.Equal(t, http.StatusNotFound, code)
	_, code = dial(t, srv, q("roomId", "live-1"), "u1")
	require.Equal(t, http.StatusSwitchingProtocols, code)
}

// ownerId used to be taken from the query string, so any viewer could claim
// to be the room owner (and drop out of viewer counts and presence).
func TestServeHTTP_OwnerComesFromRoomDirectory(t *testing.T) {
	srv := newTestServer(t, Deps{Rooms: fakeRooms{"live-1": "owner-1"}}, WSConfig{RequireKnownRoom: true})

	impostor, _ := dial(t, srv, q("roomId", "live-1", "ownerId", "u1"), "u1")
	require.EqualValues(t, 1, readViewerCount(t, impostor), "impostor is counted as a viewer")

	owner, _ := dial(t, srv, q("roomId", "live-1"), "owner-1")
	require.EqualValues(t, 1, readViewerCount(t, owner), "the real owner is not counted")
}

func TestServeHTTP_CapsConnectionsPerUser(t *testing.T) {
	srv := newTestServer(t, Deps{}, WSConfig{MaxConnsPerUser: 2})
	a, code := dial(t, srv, q("roomId", "r1"), "u1")
	require.Equal(t, http.StatusSwitchingProtocols, code)
	_, code = dial(t, srv, q("roomId", "r2"), "u1")
	require.Equal(t, http.StatusSwitchingProtocols, code)
	_, code = dial(t, srv, q("roomId", "r3"), "u1")
	require.Equal(t, http.StatusTooManyRequests, code)
	_, code = dial(t, srv, q("roomId", "r3"), "u2")
	require.Equal(t, http.StatusSwitchingProtocols, code, "other users are unaffected")

	// Closing a connection frees its slot.
	require.NoError(t, a.Close())
	require.Eventually(t, func() bool {
		ws, code := dial(t, srv, q("roomId", "r3"), "u1")
		if ws != nil {
			_ = ws.Close()
		}
		return code == http.StatusSwitchingProtocols
	}, 2*time.Second, 20*time.Millisecond)
}

func TestServeHTTP_CapsConnectionsPerIP(t *testing.T) {
	srv := newTestServer(t, Deps{}, WSConfig{MaxConnsPerIP: 2})
	for _, user := range []string{"u1", "u2"} {
		_, code := dial(t, srv, q("roomId", "r1"), user)
		require.Equal(t, http.StatusSwitchingProtocols, code)
	}
	_, code := dial(t, srv, q("roomId", "r1"), "u3")
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
