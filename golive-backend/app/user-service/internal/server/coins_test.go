package server

import (
	"bytes"
	"context"
	"encoding/json"
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
)

type fakeStripeAPI struct {
	nextSession int
	sessions    map[string]*service.StripeCheckoutSession
}

type coinWatchEvent struct {
	ID              string    `gorm:"primaryKey;type:varchar(64)"`
	UserID          string    `gorm:"type:varchar(36);not null"`
	RoomID          string    `gorm:"type:varchar(64);not null"`
	WatchDate       string    `gorm:"type:varchar(10);not null"`
	DailyWatchCount int64     `gorm:"not null;default:1"`
	LastWatchedAt   time.Time `gorm:"not null"`
}

func (coinWatchEvent) TableName() string { return "room_watch_events" }

func newFakeStripeAPI() *fakeStripeAPI {
	return &fakeStripeAPI{
		sessions: make(map[string]*service.StripeCheckoutSession),
	}
}

func (f *fakeStripeAPI) CreateCheckoutSession(_ context.Context, req service.StripeCheckoutCreateRequest) (*service.StripeCheckoutSession, error) {
	f.nextSession++
	id := fmt.Sprintf("cs_test_%d", f.nextSession)
	sess := &service.StripeCheckoutSession{
		ID:                id,
		URL:               "https://checkout.stripe.test/" + id,
		PaymentStatus:     "paid",
		ClientReferenceID: req.UserID,
		AmountTotal:       req.AmountMinor,
		Currency:          req.Currency,
		Metadata:          req.Metadata,
	}
	f.sessions[id] = sess
	return sess, nil
}

func (f *fakeStripeAPI) RetrieveCheckoutSession(_ context.Context, id string) (*service.StripeCheckoutSession, error) {
	return f.sessions[id], nil
}

func newCoinsTestRouter(t *testing.T) (*gin.Engine, *repo.UserRepo, *service.AuthService) {
	router, users, auth, _ := newCoinsTestRouterWithDB(t)
	return router, users, auth
}

func newCoinsTestRouterWithDB(t *testing.T) (*gin.Engine, *repo.UserRepo, *service.AuthService, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	users := repo.NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	require.NoError(t, db.AutoMigrate(&coinWatchEvent{}))

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

	stripeAPI := newFakeStripeAPI()
	stripeSvc := service.NewStripeServiceWithAPI(service.StripeOptions{
		PublishableKey:       "pk_test",
		SecretKey:            "sk_test_fake",
		Currency:             "usd",
		CoinsPerCurrencyUnit: 10,
	}, stripeAPI)

	return NewRouter(Deps{Auth: auth, Users: users, Stripe: stripeSvc}), users, auth, db
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
	var checkout struct {
		CheckoutURL string `json:"checkoutUrl"`
		SessionID   string `json:"sessionId"`
		Amount      int64  `json:"amount"`
		Currency    string `json:"currency"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &checkout))
	require.NotEmpty(t, checkout.CheckoutURL)
	require.NotEmpty(t, checkout.SessionID)
	require.Equal(t, int64(10), checkout.Amount)

	confirmReq := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup/confirm", bytes.NewBufferString(fmt.Sprintf(`{"sessionId":%q}`, checkout.SessionID)))
	confirmReq.Header.Set("Authorization", "Bearer "+login.Token)
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmRec := httptest.NewRecorder()
	router.ServeHTTP(confirmRec, confirmReq)

	require.Equal(t, http.StatusOK, confirmRec.Code)
	var confirmed struct {
		User        model.PublicUser      `json:"user"`
		Transaction model.CoinTransaction `json:"transaction"`
		Credited    bool                  `json:"credited"`
	}
	require.NoError(t, json.Unmarshal(confirmRec.Body.Bytes(), &confirmed))
	require.True(t, confirmed.Credited)
	got := confirmed.User
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

	replayReq := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup/confirm", bytes.NewBufferString(fmt.Sprintf(`{"sessionId":%q}`, checkout.SessionID)))
	replayReq.Header.Set("Authorization", "Bearer "+login.Token)
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	router.ServeHTTP(replayRec, replayReq)

	require.Equal(t, http.StatusOK, replayRec.Code)
	var replay struct {
		User     model.PublicUser `json:"user"`
		Credited bool             `json:"credited"`
	}
	require.NoError(t, json.Unmarshal(replayRec.Body.Bytes(), &replay))
	require.False(t, replay.Credited)
	require.Equal(t, int64(1210), replay.User.CoinBalance)
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

func TestTopupCoinsRejectsAmountAboveCap(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	// 10+2^62 used to wrap coins*100 to a $1 charge while crediting ~4.6e18 coins.
	for _, amount := range []int64{service.MaxTopupCoins + 1, 4611686018427387914} {
		req := httptest.NewRequest(http.MethodPost, "/users/me/coins/topup", bytes.NewBufferString(fmt.Sprintf(`{"amount":%d}`, amount)))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code, "amount %d", amount)
	}
}

func TestWithdrawCoinsIsNotImplemented(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	login, err := auth.Register(ctx, "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/withdrawals", bytes.NewBufferString(`{"amount":100}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	var got struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "withdrawal_not_implemented", got.Reason)
	require.NotEmpty(t, got.Message)

	persisted, err := users.FindByID(ctx, login.User.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1200), persisted.CoinBalance)
	require.Equal(t, int64(0), persisted.FrozenCoins)
}

func TestClaimDailyLoginTaskIsOncePerDay(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/daily-login-lottery/claim", nil)
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
	require.GreaterOrEqual(t, first.Transaction.Amount, int64(6))
	require.LessOrEqual(t, first.Transaction.Amount, int64(18))
	require.Equal(t, int64(1200)+first.Transaction.Amount, first.User.CoinBalance)

	req2 := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/daily-login-lottery/claim", nil)
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
	require.Equal(t, first.User.CoinBalance, second.User.CoinBalance)
	require.Equal(t, first.Transaction.ID, second.Transaction.ID)
}

func TestClaimWatchDailyTaskRequiresServerProgress(t *testing.T) {
	router, _, auth, db := newCoinsTestRouterWithDB(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/watch-3-lives/claim", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "daily_task_incomplete")

	today := time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02")
	for i := 1; i <= 3; i++ {
		require.NoError(t, db.Create(&coinWatchEvent{
			ID:              fmt.Sprintf("watch-%d", i),
			UserID:          login.User.ID,
			RoomID:          fmt.Sprintf("room-%d", i),
			WatchDate:       today,
			DailyWatchCount: 1,
			LastWatchedAt:   time.Now().UTC(),
		}).Error)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/users/me/coins/daily-tasks/watch-3-lives/claim", nil)
	req2.Header.Set("Authorization", "Bearer "+login.Token)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
	var claimed struct {
		Transaction model.CoinTransaction `json:"transaction"`
		Created     bool                  `json:"created"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &claimed))
	require.True(t, claimed.Created)
	require.Equal(t, int64(18), claimed.Transaction.Amount)
}
