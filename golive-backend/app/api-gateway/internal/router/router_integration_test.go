package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/api-gateway/internal/config"
	"github.com/qingwenwen777/golive/app/api-gateway/internal/router"
)

func init() { gin.SetMode(gin.TestMode) }

// upstreamSpy returns a test server + the most recent request it saw.
type upstreamSpy struct {
	srv  *httptest.Server
	last *http.Request
	body string
}

func newUpstreamSpy(t *testing.T, status int, body string) *upstreamSpy {
	t.Helper()
	s := &upstreamSpy{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		s.body = string(buf)
		s.last = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func newGateway(t *testing.T, cfg *config.Config) *gin.Engine {
	t.Helper()
	r, err := router.New(cfg)
	require.NoError(t, err)
	return r
}

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
	ch chan bool
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return r.ch
}

func newRecorder() *closeNotifyRecorder {
	return &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), ch: make(chan bool)}
}

func sign(t *testing.T, secret, uid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uid, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func baseCfg(user, room, gift string) *config.Config {
	return &config.Config{
		Upstreams: config.UpstreamsCfg{
			UserService: user,
			RoomService: room,
			GiftService: gift,
			ChatService: gift,
		},
		Proxy:     config.ProxyCfg{Timeout: 2 * time.Second, MaxIdleConns: 10, MaxIdleConnsPerHost: 5},
		JWT:       config.JWTCfg{Secret: "secret"},
		CORS:      config.CORSCfg{AllowedOrigins: []string{"http://localhost:5173"}, MaxAge: 600},
		RateLimit: config.RateLimitCfg{Enabled: false},
	}
}

func TestRoute_StripsPrefixAndForwards(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{"id":"u1"}`)
	room := newUpstreamSpy(t, 200, `{"items":[]}`)
	gift := newUpstreamSpy(t, 200, `[]`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	// /api/rooms — public, no auth needed.
	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/rooms?page=1", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "/rooms", room.last.URL.Path, "prefix /api should be stripped")
	require.Equal(t, "page=1", room.last.URL.RawQuery)
}

func TestRoute_UploadAvatarsForwardToUserService(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/uploads/avatars/u.png", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "/uploads/avatars/u.png", user.last.URL.Path)
	require.Nil(t, room.last)
}

func TestRoute_UploadCoversForwardToUserService(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/uploads/covers/banner.webp", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "/uploads/covers/banner.webp", user.last.URL.Path)
	require.Nil(t, room.last)
}

func TestRoute_UploadCoversFallbackToRoomService(t *testing.T) {
	user := newUpstreamSpy(t, 404, `not found`)
	room := newUpstreamSpy(t, 200, `room-cover`)
	gift := newUpstreamSpy(t, 200, `{}`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/uploads/covers/live.webp", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "room-cover", w.Body.String())
	require.Equal(t, "/uploads/covers/live.webp", user.last.URL.Path)
	require.Equal(t, "/uploads/covers/live.webp", room.last.URL.Path)
}

func TestRoute_PublicChatHistoryNoAuth(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	chat := newUpstreamSpy(t, 200, `{"items":[]}`)
	cfg := baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL)
	cfg.Upstreams.ChatService = chat.srv.URL

	r := newGateway(t, cfg)

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/chat/rooms/r1/danmus?limit=12", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "/chat/rooms/r1/danmus", chat.last.URL.Path)
	require.Equal(t, "limit=12", chat.last.URL.RawQuery)
	require.Nil(t, room.last)
}

func TestRoute_InjectsUserIDFromJWT(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{"id":"u1"}`)
	room := newUpstreamSpy(t, 200, `{"channelId":"c","following":true}`)
	gift := newUpstreamSpy(t, 200, `[]`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	tok := sign(t, "secret", "user-99")
	req := httptest.NewRequest("POST", "/api/rooms/abc/follow", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-User-Id", "spoofed") // gateway must strip
	w := newRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "user-99", room.last.Header.Get("X-User-Id"))
	require.NotEmpty(t, w.Header().Get("X-Request-Id"), "gateway echoes request id")
}

func TestRoute_RequestIDPassedThrough(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)

	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))
	tok := sign(t, "secret", "u1")

	req := httptest.NewRequest("POST", "/api/gifts/send", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Request-Id", "client-id-123")
	w := newRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, "client-id-123", gift.last.Header.Get("X-Request-Id"),
		"gift-service must see the same request id (idempotency dependency)")
	require.Equal(t, "client-id-123", w.Header().Get("X-Request-Id"))
}

func TestRoute_RejectsUnauthenticatedNonPublic(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/rooms/x/follow", nil))
	require.Equal(t, 401, w.Code)
	require.Contains(t, w.Body.String(), "Unauthorized")
	require.Nil(t, room.last, "request must NOT reach upstream")
}

func TestRoute_PublicLoginNoAuth(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{"token":"t","refreshToken":"r","user":{}}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"demo","password":"demo"}`)))
	require.Equal(t, 200, w.Code)
}

func TestRoute_PublicUserProfileNoAuth(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{"id":"u1"}`)
	room := newUpstreamSpy(t, 200, `{}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/users/profile/u1", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "/users/profile/u1", user.last.URL.Path)
}

func TestRoute_Wraps5xxFromUpstream(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	// Upstream returns a 500 with a non-JSON body.
	room := newUpstreamSpy(t, 500, `boom`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/rooms", nil))
	require.Equal(t, 500, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "Internal error", body["message"])
	require.Equal(t, "upstream_unavailable", body["reason"])
}

func TestRoute_Passes4xxThroughUntouched(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	// 402 with reason — must NOT be rewritten.
	room := newUpstreamSpy(t, 402, `{"message":"insufficient coin","reason":"insufficient_coin"}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	tok := sign(t, "secret", "u1")
	req := httptest.NewRequest("POST", "/api/rooms/x/follow", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := newRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, 402, w.Code)
	require.Contains(t, w.Body.String(), `"insufficient_coin"`)
}

func TestRoute_5xxWithUnifiedBodyKept(t *testing.T) {
	user := newUpstreamSpy(t, 200, `{}`)
	// Upstream already speaks our shape — keep it.
	room := newUpstreamSpy(t, 503, `{"message":"db down","reason":"db_unavailable"}`)
	gift := newUpstreamSpy(t, 200, `{}`)
	r := newGateway(t, baseCfg(user.srv.URL, room.srv.URL, gift.srv.URL))

	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/rooms", nil))
	require.Equal(t, 503, w.Code)
	require.Contains(t, w.Body.String(), `"db down"`)
	require.Contains(t, w.Body.String(), `"db_unavailable"`)
}

func TestRoute_UpstreamDownReturnsBadGateway(t *testing.T) {
	r := newGateway(t, baseCfg("http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1"))
	w := newRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/rooms", nil))
	require.True(t, w.Code == http.StatusBadGateway || w.Code == http.StatusGatewayTimeout)
	require.Contains(t, w.Body.String(), "upstream_unavailable")
}
