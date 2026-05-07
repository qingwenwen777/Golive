package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/handler"
	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/server"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type giftHTTPFixture struct {
	router *gin.Engine
	db     *gorm.DB
}

func newGiftHTTPFixture(t *testing.T, viewerBalance int64) giftHTTPFixture {
	t.Helper()

	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf(
		"file:%s_%d?mode=memory&cache=shared&_busy_timeout=5000&_pragma=foreign_keys=on",
		dbName,
		time.Now().UnixNano(),
	)), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(64) NOT NULL DEFAULT '',
		display_name VARCHAR(64) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		coin_balance INTEGER NOT NULL,
		frozen_coins INTEGER NOT NULL DEFAULT 0,
		banned BOOLEAN NOT NULL DEFAULT false
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rooms (
		id VARCHAR(64) PRIMARY KEY,
		channel VARCHAR(128) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		owner_id VARCHAR(36) NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)",
		"u-demo", "demo", "Demo Viewer", "", viewerBalance,
		"u-owner", "owner", "Streamer", "owner.png", int64(0),
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO rooms (id, channel, avatar, owner_id) VALUES (?, ?, ?, ?)",
		"room-1", "Streamer", "room.png", "u-owner",
	).Error)
	require.NoError(t, db.AutoMigrate(
		&model.Gift{},
		&model.GiftOrder{},
		&model.SuperChatOrder{},
		&model.CoinTransaction{},
		&model.LocalMessage{},
		&model.FanBadge{},
		&model.BetRound{},
		&model.BetWager{},
	))

	redisSrv := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisSrv.Addr()})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	orders := repo.NewOrderRepo(db)
	idem := service.NewIdemCache(rdb, time.Minute)
	router := server.NewRouter(server.Deps{
		JWTSecret: "test-secret",
		Gift:      handler.NewGiftHandler(service.NewGiftService(repo.NewGiftRepo(db), orders), idem),
		SuperChat: handler.NewSuperChatHandler(service.NewSuperChatService(orders), idem),
		Bet:       handler.NewBetHandler(service.NewBetService(orders)),
	})

	return giftHTTPFixture{router: router, db: db}
}

func postJSON(router *gin.Engine, path, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+signGiftToken(userID))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getJSON(router *gin.Engine, path, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+signGiftToken(userID))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func signGiftToken(userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"typ": "access",
	})
	signed, err := tok.SignedString([]byte("test-secret"))
	if err != nil {
		panic(err)
	}
	return signed
}

func seedGift(t *testing.T, db *gorm.DB, id string, price int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.Gift{
		ID: id, Name: id, Icon: "x", PriceCoin: price, Category: "basic",
	}).Error)
}

func coinBalance(t *testing.T, db *gorm.DB, userID string) int64 {
	t.Helper()
	var balance int64
	require.NoError(t, db.Raw("SELECT coin_balance FROM users WHERE id = ?", userID).Scan(&balance).Error)
	return balance
}

func TestGiftSendHTTP_InsufficientCoinCachesReplayShape(t *testing.T) {
	fx := newGiftHTTPFixture(t, 50)
	seedGift(t, fx.db, "rocket", 100)

	body := `{"roomId":"room-1","giftId":"rocket","count":1,"requestId":"req-low"}`
	first := postJSON(fx.router, "/gifts/send", "u-demo", body)
	require.Equal(t, http.StatusPaymentRequired, first.Code)
	require.Empty(t, first.Header().Get("Idempotent-Replayed"))
	require.JSONEq(t, `{
		"giftId":"rocket",
		"count":1,
		"totalCoin":100,
		"status":"failed",
		"failReason":"insufficient_coin",
		"reason":"insufficient_coin",
		"message":"Insufficient coins"
	}`, stripVolatileGiftFields(t, first.Body.String()))
	require.EqualValues(t, 50, coinBalance(t, fx.db, "u-demo"))

	replay := postJSON(fx.router, "/gifts/send", "u-demo", body)
	require.Equal(t, http.StatusPaymentRequired, replay.Code)
	require.Equal(t, "true", replay.Header().Get("Idempotent-Replayed"))
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.EqualValues(t, 50, coinBalance(t, fx.db, "u-demo"))
}

func TestBetHTTP_OpenWagerDuplicateAndLatestViewerState(t *testing.T) {
	fx := newGiftHTTPFixture(t, 1000)

	open := postJSON(
		fx.router,
		"/bets",
		"u-owner",
		`{"roomId":"room-1","amount":100,"question":"Will blue team win?"}`,
	)
	require.Equal(t, http.StatusOK, open.Code)

	var opened struct {
		Round struct {
			ID       string `json:"id"`
			Question string `json:"question"`
			Amount   int64  `json:"amount"`
			Status   string `json:"status"`
		} `json:"round"`
	}
	require.NoError(t, json.Unmarshal(open.Body.Bytes(), &opened))
	require.NotEmpty(t, opened.Round.ID)
	require.Equal(t, "Will blue team win?", opened.Round.Question)
	require.EqualValues(t, 100, opened.Round.Amount)
	require.Equal(t, model.BetRoundOpen, opened.Round.Status)

	wager := postJSON(
		fx.router,
		"/bets/"+opened.Round.ID+"/wagers",
		"u-demo",
		`{"roomId":"room-1","option":"win"}`,
	)
	require.Equal(t, http.StatusOK, wager.Code)
	require.EqualValues(t, 900, coinBalance(t, fx.db, "u-demo"))

	duplicate := postJSON(
		fx.router,
		"/bets/"+opened.Round.ID+"/wagers",
		"u-demo",
		`{"roomId":"room-1","option":"lose"}`,
	)
	require.Equal(t, http.StatusConflict, duplicate.Code)
	require.JSONEq(t, `{"message":"Bet already placed","reason":"bet_already_placed"}`, duplicate.Body.String())
	require.EqualValues(t, 900, coinBalance(t, fx.db, "u-demo"), "duplicate wager must not charge twice")

	latest := getJSON(fx.router, "/bets/latest?roomId=room-1", "u-demo")
	require.Equal(t, http.StatusOK, latest.Code)
	var view struct {
		Summary []repo.BetOptionSummary `json:"summary"`
		MyWager *struct {
			Option string `json:"option"`
			Amount int64  `json:"amount"`
			Status string `json:"status"`
		} `json:"myWager"`
	}
	require.NoError(t, json.Unmarshal(latest.Body.Bytes(), &view))
	require.NotNil(t, view.MyWager)
	require.Equal(t, model.BetOptionWin, view.MyWager.Option)
	require.Equal(t, model.StatusLocked, view.MyWager.Status)
	require.EqualValues(t, 100, view.MyWager.Amount)
	require.Equal(t, []repo.BetOptionSummary{
		{Option: model.BetOptionWin, Count: 1, Total: 100},
		{Option: model.BetOptionLose, Count: 0, Total: 0},
	}, view.Summary)
}

func TestBetHTTP_RejectsUnauthorizedAndBadOpenPayloads(t *testing.T) {
	fx := newGiftHTTPFixture(t, 1000)

	unauthorized := postJSON(
		fx.router,
		"/bets",
		"",
		`{"roomId":"room-1","amount":100,"question":"Will blue team win?"}`,
	)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	tooLongQuestion := strings.Repeat("赢", service.MaxBetQuestionRunes+1)
	bad := postJSON(
		fx.router,
		"/bets",
		"u-owner",
		fmt.Sprintf(`{"roomId":"room-1","amount":100,"question":%q}`, tooLongQuestion),
	)
	require.Equal(t, http.StatusBadRequest, bad.Code)
}

func TestGiftHTTP_RejectsSpoofedUserHeader(t *testing.T) {
	fx := newGiftHTTPFixture(t, 1000)

	req := httptest.NewRequest(http.MethodPost, "/bets", strings.NewReader(`{"roomId":"room-1","amount":100,"question":"Will blue team win?"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "u-owner")
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func stripVolatileGiftFields(t *testing.T, body string) string {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	delete(payload, "orderId")
	delete(payload, "requestId")
	delete(payload, "createdAt")
	out, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(out)
}
