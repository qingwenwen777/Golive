package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func newCoinsTestRouter(t *testing.T) (*gin.Engine, *repo.UserRepo, *service.AuthService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	users := repo.NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	auth := service.NewAuthService(users, repo.NewTokenRepo(rdb), service.Options{
		JWTSecret:  "test-secret",
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})

	return NewRouter(Deps{Auth: auth, Users: users}), users, auth
}

func TestTopupCoinsReturnsUpdatedUser(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	login, err := auth.Register(ctx, "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup", bytes.NewBufferString(`{"amount":1000}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(2200), got.CoinBalance)

	persisted, err := users.FindByID(ctx, login.User.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2200), persisted.CoinBalance)
}

func TestTopupCoinsRejectsInvalidAmount(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup", bytes.NewBufferString(`{"amount":0}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
