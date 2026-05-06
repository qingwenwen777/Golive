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

func newFakeStripeAPI() *fakeStripeAPI {
	return &fakeStripeAPI{sessions: make(map[string]*service.StripeCheckoutSession)}
}

func (f *fakeStripeAPI) CreateCheckoutSession(_ context.Context, req service.StripeCheckoutCreateRequest) (*service.StripeCheckoutSession, error) {
	f.nextSession++
	id := fmt.Sprintf("cs_test_%d", f.nextSession)
	sess := &service.StripeCheckoutSession{
		ID:                id,
		URL:               "https://checkout.stripe.test/" + id,
		PaymentStatus:     "paid",
		ClientReferenceID: req.UserID,
		Metadata:          req.Metadata,
	}
	f.sessions[id] = sess
	return sess, nil
}

func (f *fakeStripeAPI) RetrieveCheckoutSession(_ context.Context, id string) (*service.StripeCheckoutSession, error) {
	return f.sessions[id], nil
}

func (f *fakeStripeAPI) CreateExpressAccount(_ context.Context, _ service.StripeAccountCreateRequest) (*service.StripeAccount, error) {
	return &service.StripeAccount{ID: "acct_test", PayoutsEnabled: true, DetailsSubmitted: true}, nil
}

func (f *fakeStripeAPI) RetrieveAccount(_ context.Context, id string) (*service.StripeAccount, error) {
	return &service.StripeAccount{ID: id, PayoutsEnabled: true, DetailsSubmitted: true}, nil
}

func (f *fakeStripeAPI) CreateAccountLink(_ context.Context, req service.StripeAccountLinkCreateRequest) (string, error) {
	return "https://connect.stripe.test/" + req.AccountID, nil
}

func (f *fakeStripeAPI) CreateTransfer(_ context.Context, _ service.StripeTransferCreateRequest) (*service.StripeTransfer, error) {
	return &service.StripeTransfer{ID: "tr_test"}, nil
}

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

	stripeSvc := service.NewStripeServiceWithAPI(service.StripeOptions{
		PublishableKey:       "pk_test",
		SecretKey:            "sk_test",
		Currency:             "usd",
		CoinsPerCurrencyUnit: 10,
		ConnectCountry:       "US",
	}, newFakeStripeAPI())

	return NewRouter(Deps{Auth: auth, Users: users, Stripe: stripeSvc}), users, auth
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

func TestWithdrawCoinsCreatesStripeTransferAndLedger(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	login, err := auth.Register(ctx, "demo", "demo", "Demo")
	require.NoError(t, err)
	_, err = users.SetStripeAccountID(ctx, login.User.ID, "acct_test")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/users/me/coins/withdrawals", bytes.NewBufferString(`{"amount":100}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		User        model.PublicUser      `json:"user"`
		Transaction model.CoinTransaction `json:"transaction"`
		Amount      int64                 `json:"amount"`
		Fee         int64                 `json:"fee"`
		NetCoins    int64                 `json:"netCoins"`
		TransferID  string                `json:"transferId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(1100), got.User.CoinBalance)
	require.Equal(t, int64(0), got.User.FrozenCoins)
	require.Equal(t, int64(100), got.Amount)
	require.Equal(t, int64(35), got.Fee)
	require.Equal(t, int64(65), got.NetCoins)
	require.Equal(t, "tr_test", got.TransferID)
	require.Equal(t, model.CoinTxWithdrawal, got.Transaction.Type)
	require.Equal(t, int64(-100), got.Transaction.Amount)
	require.Equal(t, int64(1100), got.Transaction.BalanceAfter)

	persisted, err := users.FindByID(ctx, login.User.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1100), persisted.CoinBalance)
	require.Equal(t, int64(0), persisted.FrozenCoins)
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
