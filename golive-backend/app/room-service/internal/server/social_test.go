package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v9"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/server"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

const jwtSecret = "test-secret"

func init() {
	gin.SetMode(gin.TestMode)
}

type socialHTTPFixture struct {
	router *gin.Engine
	redis  *miniredis.Miniredis
}

func newSocialHTTPFixture(t *testing.T) socialHTTPFixture {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	social := service.NewSocialService(repo.NewSocialRepo(rdb))
	router := server.NewRouter(server.Deps{
		JWTSecret: jwtSecret,
		Social:    social,
	})
	return socialHTTPFixture{router: router, redis: mr}
}

func signedToken(t *testing.T, userID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	out, err := token.SignedString([]byte(jwtSecret))
	require.NoError(t, err)
	return out
}

func request(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSocialHTTP_FollowRequiresAuthButPublicStateAllowsGuests(t *testing.T) {
	fx := newSocialHTTPFixture(t)
	alice := signedToken(t, "alice")

	guest := request(fx.router, http.MethodGet, "/rooms/ch-luna/follow", "")
	require.Equal(t, http.StatusOK, guest.Code)
	require.JSONEq(t, `{"channelId":"ch-luna","following":false,"subscriberCount":0}`, guest.Body.String())

	unauthorized := request(fx.router, http.MethodPost, "/rooms/ch-luna/follow", "")
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	follow := request(fx.router, http.MethodPost, "/rooms/ch-luna/follow", alice)
	require.Equal(t, http.StatusOK, follow.Code)
	require.JSONEq(t, `{"channelId":"ch-luna","following":true,"subscriberCount":1}`, follow.Body.String())

	guestAfter := request(fx.router, http.MethodGet, "/rooms/ch-luna/follow", "")
	require.Equal(t, http.StatusOK, guestAfter.Code)
	require.JSONEq(t, `{"channelId":"ch-luna","following":false,"subscriberCount":1}`, guestAfter.Body.String())

	aliceAfter := request(fx.router, http.MethodGet, "/rooms/ch-luna/follow", alice)
	require.Equal(t, http.StatusOK, aliceAfter.Code)
	require.JSONEq(t, `{"channelId":"ch-luna","following":true,"subscriberCount":1}`, aliceAfter.Body.String())
}

func TestSocialHTTP_LikeDislikeAuthStateMachine(t *testing.T) {
	fx := newSocialHTTPFixture(t)
	token := signedToken(t, "viewer-1")
	require.NoError(t, fx.redis.Set("like:stream-1:count", "10"))

	// Anyone can read the count; only a signed-in viewer can change it.
	guest := request(fx.router, http.MethodGet, "/rooms/stream-1/like", "")
	require.Equal(t, http.StatusOK, guest.Code)
	require.JSONEq(t, `{"streamId":"stream-1","liked":false,"disliked":false,"likes":10}`, guest.Body.String())

	unauthorized := request(fx.router, http.MethodPost, "/rooms/stream-1/like", "")
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	like := request(fx.router, http.MethodPost, "/rooms/stream-1/like", token)
	require.Equal(t, http.StatusOK, like.Code)
	require.JSONEq(t, `{"streamId":"stream-1","liked":true,"disliked":false,"likes":11}`, like.Body.String())

	guestAfter := request(fx.router, http.MethodGet, "/rooms/stream-1/like", "")
	require.Equal(t, http.StatusOK, guestAfter.Code)
	require.JSONEq(t, `{"streamId":"stream-1","liked":false,"disliked":false,"likes":11}`, guestAfter.Body.String())

	dislike := request(fx.router, http.MethodPost, "/rooms/stream-1/dislike", token)
	require.Equal(t, http.StatusOK, dislike.Code)
	require.JSONEq(t, `{"streamId":"stream-1","liked":false,"disliked":true,"likes":10}`, dislike.Body.String())

	undislike := request(fx.router, http.MethodDelete, "/rooms/stream-1/dislike", token)
	require.Equal(t, http.StatusOK, undislike.Code)
	require.JSONEq(t, `{"streamId":"stream-1","liked":false,"disliked":false,"likes":10}`, undislike.Body.String())

	state := request(fx.router, http.MethodGet, "/rooms/stream-1/like", token)
	require.Equal(t, http.StatusOK, state.Code)
	var got struct {
		StreamID string `json:"streamId"`
		Liked    bool   `json:"liked"`
		Disliked bool   `json:"disliked"`
		Likes    int64  `json:"likes"`
	}
	require.NoError(t, json.Unmarshal(state.Body.Bytes(), &got))
	require.Equal(t, "stream-1", got.StreamID)
	require.False(t, got.Liked)
	require.False(t, got.Disliked)
	require.EqualValues(t, 10, got.Likes)
}
