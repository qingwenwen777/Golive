package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/handler"
	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/server"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

const testInternalToken = "internal-test-token"

// newInternalGiftRouter serves the same DB as fx with the admin handler and
// the internal token wired in.
func newInternalGiftRouter(t *testing.T, db *gorm.DB, token string) *gin.Engine {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	orders := repo.NewOrderRepo(db)
	return server.NewRouter(server.Deps{
		JWTSecret:     "test-secret",
		SuperChat:     handler.NewSuperChatHandler(service.NewSuperChatService(orders), service.NewIdemCache(rdb, time.Minute)),
		Admin:         handler.NewAdminHandler(service.NewAdminService(repo.NewAdminRepo(db), orders)),
		InternalToken: token,
	})
}

func postInternal(router *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"operatorId":"admin-1"}`))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(internalauth.Header, token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type ledgerSnapshot struct {
	ViewerBalance int64
	OwnerBalance  int64
	TxCount       int64
	TxSum         int64
	TodaySCRev    int64
	ReportSCCoins int64
}

func snapshotLedger(t *testing.T, db *gorm.DB) ledgerSnapshot {
	t.Helper()
	ctx := context.Background()
	admin := repo.NewAdminRepo(db)
	var s ledgerSnapshot
	s.ViewerBalance = coinBalance(t, db, "u-demo")
	s.OwnerBalance = coinBalance(t, db, "u-owner")
	require.NoError(t, db.Model(&model.CoinTransaction{}).Count(&s.TxCount).Error)
	require.NoError(t, db.Model(&model.CoinTransaction{}).Select("COALESCE(SUM(amount), 0)").Row().Scan(&s.TxSum))
	summary, err := admin.EconomySummary(ctx)
	require.NoError(t, err)
	s.TodaySCRev = summary.TodaySuperChatRevenue
	rows, err := admin.RevenueReport(ctx, "day", 1)
	require.NoError(t, err)
	for _, row := range rows {
		s.ReportSCCoins += row.SuperChatCoins
	}
	return s
}

func TestInternalSuperChatModerationRequiresToken(t *testing.T) {
	fx := newGiftHTTPFixture(t, 5000)
	router := newInternalGiftRouter(t, fx.db, testInternalToken)
	for _, token := range []string{"", "wrong"} {
		rec := postInternal(router, "/internal/super-chats/sc-any/moderation", token)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	// A user's access token is not an internal credential.
	req := httptest.NewRequest(http.MethodPost, "/internal/super-chats/sc-any/moderation", nil)
	req.Header.Set("Authorization", "Bearer "+signGiftToken("u-demo"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	closed := newInternalGiftRouter(t, fx.db, "")
	rec = postInternal(closed, "/internal/super-chats/sc-any/moderation", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Moderating a paid super chat hides it but must not rewrite its financial
// outcome: status stays success, no refund, ledger and revenue unchanged.
func TestInternalSuperChatModerationKeepsLedgerAndRevenue(t *testing.T) {
	fx := newGiftHTTPFixture(t, 5000)
	router := newInternalGiftRouter(t, fx.db, testInternalToken)

	sent := postJSON(router, "/super-chats", "u-demo", `{"roomId":"room-1","amount":1000,"text":"hello","requestId":"sc-req-1"}`)
	require.Equal(t, http.StatusOK, sent.Code, sent.Body.String())
	var order model.SuperChatOrder
	require.NoError(t, json.Unmarshal(sent.Body.Bytes(), &order))
	require.Equal(t, model.StatusSuccess, order.Status)

	before := snapshotLedger(t, fx.db)
	require.EqualValues(t, 4000, before.ViewerBalance)
	require.EqualValues(t, 1000, before.OwnerBalance)
	require.EqualValues(t, 1000, before.TodaySCRev)
	require.EqualValues(t, 1000, before.ReportSCCoins)

	rec := postInternal(router, "/internal/super-chats/"+order.OrderID+"/moderation", testInternalToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp struct {
		OrderID     string     `json:"orderId"`
		Status      string     `json:"status"`
		ModeratedAt *time.Time `json:"moderatedAt"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, order.OrderID, resp.OrderID)
	require.Equal(t, model.StatusSuccess, resp.Status)
	require.NotNil(t, resp.ModeratedAt)

	var stored model.SuperChatOrder
	require.NoError(t, fx.db.Where("order_id = ?", order.OrderID).Take(&stored).Error)
	require.Equal(t, model.StatusSuccess, stored.Status)
	require.Empty(t, stored.FailReason)
	require.NotNil(t, stored.ModeratedAt)
	require.Equal(t, "admin-1", stored.ModeratedBy)
	require.Equal(t, before, snapshotLedger(t, fx.db))

	// Repeating the call is a no-op that keeps the first moderation.
	rec = postInternal(router, "/internal/super-chats/"+order.OrderID+"/moderation", testInternalToken)
	require.Equal(t, http.StatusOK, rec.Code)
	var again model.SuperChatOrder
	require.NoError(t, fx.db.Where("order_id = ?", order.OrderID).Take(&again).Error)
	require.True(t, again.ModeratedAt.Equal(*stored.ModeratedAt))
	require.Equal(t, before, snapshotLedger(t, fx.db))

	rec = postInternal(router, "/internal/super-chats/sc-missing/moderation", testInternalToken)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
