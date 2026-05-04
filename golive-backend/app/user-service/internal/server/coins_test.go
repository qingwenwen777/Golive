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
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
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

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup", bytes.NewBufferString(`{"amount":10}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(1210), got.CoinBalance)
	require.Equal(t, 1, got.LevelInfo.Level)
	require.Equal(t, int64(10), got.LevelInfo.TotalTopupCoins)
	require.Greater(t, got.LevelInfo.CoinsToNextLevel, int64(0))

	persisted, err := users.FindByID(ctx, login.User.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1210), persisted.CoinBalance)

	txReq := httptest.NewRequest(http.MethodGet, "/users/me/coins/transactions", nil)
	txReq.Header.Set("Authorization", "Bearer "+login.Token)
	txRec := httptest.NewRecorder()
	router.ServeHTTP(txRec, txReq)

	require.Equal(t, http.StatusOK, txRec.Code)
	var ledger struct {
		Items []model.CoinTransaction `json:"items"`
	}
	require.NoError(t, json.Unmarshal(txRec.Body.Bytes(), &ledger))
	require.Len(t, ledger.Items, 1)
	require.Equal(t, model.CoinTxTopup, ledger.Items[0].Type)
	require.Equal(t, int64(10), ledger.Items[0].Amount)
	require.Equal(t, int64(1210), ledger.Items[0].BalanceAfter)
}

func TestTopupCoinsRejectsInvalidAmount(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup", bytes.NewBufferString(`{"amount":9}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestClaimDailyTaskIsOncePerDay(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/watch-3-lives/claim", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var first struct {
		User           model.PublicUser      `json:"user"`
		Transaction    model.CoinTransaction `json:"transaction"`
		Created        bool                  `json:"created"`
		AlreadyClaimed bool                  `json:"alreadyClaimed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
	require.True(t, first.Created)
	require.False(t, first.AlreadyClaimed)
	require.Equal(t, int64(18), first.Transaction.Amount)
	require.Equal(t, int64(1218), first.User.CoinBalance)

	req2 := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/watch-3-lives/claim", nil)
	req2.Header.Set("Authorization", "Bearer "+login.Token)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
	var second struct {
		User           model.PublicUser      `json:"user"`
		Transaction    model.CoinTransaction `json:"transaction"`
		Created        bool                  `json:"created"`
		AlreadyClaimed bool                  `json:"alreadyClaimed"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &second))
	require.False(t, second.Created)
	require.True(t, second.AlreadyClaimed)
	require.Equal(t, int64(1218), second.User.CoinBalance)
	require.Equal(t, first.Transaction.ID, second.Transaction.ID)
}
