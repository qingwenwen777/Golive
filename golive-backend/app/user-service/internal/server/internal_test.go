package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

const testInternalToken = "internal-test-token"

func newInternalTestRouter(t *testing.T, token string) (*gin.Engine, *repo.UserRepo, *service.AuthService, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	users := repo.NewUserRepo(db).WithRedis(rdb)
	require.NoError(t, users.AutoMigrate())
	auth := service.NewAuthService(users, repo.NewTokenRepo(rdb), service.Options{
		JWTSecret:  "test-secret",
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})
	return NewRouter(Deps{Auth: auth, Users: users, InternalToken: token}), users, auth, db, mr
}

func serveInternal(router *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set(internalauth.Header, token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestInternalEndpointsRequireToken(t *testing.T) {
	router, _, auth, _, _ := newInternalTestRouter(t, testInternalToken)
	target, err := auth.Register(context.Background(), "target", "secret123", "Target")
	require.NoError(t, err)

	restriction := "/internal/users/" + target.User.ID + "/restriction"
	permission := "/internal/users/" + target.User.ID + "/permission"
	for _, token := range []string{"", "wrong"} {
		rec := serveInternal(router, http.MethodPost, restriction, token, `{"action":"ban"}`)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		rec = serveInternal(router, http.MethodGet, permission, token, "")
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	// A user's own access token is not an internal credential.
	req := httptest.NewRequest(http.MethodPost, restriction, bytes.NewBufferString(`{"action":"ban"}`))
	req.Header.Set("Authorization", "Bearer "+target.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = serveInternal(router, http.MethodGet, permission, testInternalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// Without a configured token the internal API is closed.
	closed, _, closedAuth, _, _ := newInternalTestRouter(t, "")
	other, err := closedAuth.Register(context.Background(), "other", "secret123", "Other")
	require.NoError(t, err)
	rec = serveInternal(closed, http.MethodGet, "/internal/users/"+other.User.ID+"/permission", "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInternalBanRevokesSessionsAndUnbanClears(t *testing.T) {
	router, users, auth, db, mr := newInternalTestRouter(t, testInternalToken)
	ctx := context.Background()
	target, err := auth.Register(ctx, "target", "secret123", "Target")
	require.NoError(t, err)
	path := "/internal/users/" + target.User.ID + "/restriction"

	rec := serveInternal(router, http.MethodPost, path, testInternalToken, `{"action":"ban","reason":"spam","operatorId":"admin-1"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	persisted, err := users.FindByID(ctx, target.User.ID)
	require.NoError(t, err)
	require.True(t, persisted.Banned)
	require.Equal(t, "spam", persisted.BanReason)
	var state model.UserModerationState
	require.NoError(t, db.Where("user_id = ?", target.User.ID).Take(&state).Error)
	require.True(t, state.Banned)
	require.Equal(t, "admin-1", state.UpdatedBy)
	require.True(t, mr.Exists(contentpolicy.RedisSiteBanPrefix+target.User.ID))
	// Same path as the admin ban: refresh tokens issued before are revoked.
	_, err = auth.Refresh(ctx, target.RefreshToken)
	require.Error(t, err)

	rec = serveInternal(router, http.MethodPost, path, testInternalToken, `{"action":"unban","operatorId":"admin-1"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	persisted, err = users.FindByID(ctx, target.User.ID)
	require.NoError(t, err)
	require.False(t, persisted.Banned)
	require.False(t, mr.Exists(contentpolicy.RedisSiteBanPrefix+target.User.ID))
}

func TestInternalMuteSetsStateAndCache(t *testing.T) {
	router, _, auth, db, mr := newInternalTestRouter(t, testInternalToken)
	ctx := context.Background()
	target, err := auth.Register(ctx, "target", "secret123", "Target")
	require.NoError(t, err)
	path := "/internal/users/" + target.User.ID + "/restriction"

	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	body := fmt.Sprintf(`{"action":"mute","reason":"flood","mutedUntil":%q,"operatorId":"admin-1"}`, until.Format(time.RFC3339))
	rec := serveInternal(router, http.MethodPost, path, testInternalToken, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var state model.UserModerationState
	require.NoError(t, db.Where("user_id = ?", target.User.ID).Take(&state).Error)
	require.NotNil(t, state.MutedUntil)
	require.True(t, state.MutedUntil.Equal(until))
	require.Equal(t, "flood", state.MuteReason)
	require.False(t, state.Banned)
	require.True(t, mr.Exists(contentpolicy.RedisSiteMutePrefix+target.User.ID))
	require.Greater(t, mr.TTL(contentpolicy.RedisSiteMutePrefix+target.User.ID), time.Hour)

	rec = serveInternal(router, http.MethodPost, path, testInternalToken, `{"action":"unmute"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var cleared model.UserModerationState
	require.NoError(t, db.Where("user_id = ?", target.User.ID).Take(&cleared).Error)
	require.Nil(t, cleared.MutedUntil)
	require.False(t, mr.Exists(contentpolicy.RedisSiteMutePrefix+target.User.ID))
}

func TestInternalRestrictionRejectsBadInput(t *testing.T) {
	router, _, auth, _, _ := newInternalTestRouter(t, testInternalToken)
	target, err := auth.Register(context.Background(), "target", "secret123", "Target")
	require.NoError(t, err)
	path := "/internal/users/" + target.User.ID + "/restriction"

	rec := serveInternal(router, http.MethodPost, path, testInternalToken, `{"action":"delete"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	rec = serveInternal(router, http.MethodPost, path, testInternalToken, `{"action":"mute"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	rec = serveInternal(router, http.MethodPost, path, testInternalToken, fmt.Sprintf(`{"action":"mute","mutedUntil":%q}`, past))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	for _, action := range []string{"ban", "unmute"} {
		rec = serveInternal(router, http.MethodPost, "/internal/users/nobody/restriction", testInternalToken, `{"action":"`+action+`"}`)
		require.Equal(t, http.StatusNotFound, rec.Code, action)
	}
}
