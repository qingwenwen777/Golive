package service_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/userlevel"
)

// newTestDB returns an isolated in-memory SQLite with the schema migrated
// and a `users` table holding one demo user with `balance` coins.
func newTestDB(t *testing.T, balance int64) *gorm.DB {
	t.Helper()
	// Each test gets its own database — `:memory:?cache=shared` would share
	// across goroutines but we want isolation, not sharing.
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_busy_timeout=5000&_pragma=foreign_keys=on"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	// Drop residue from prior tests in the shared cache.
	_ = db.Exec("DROP TABLE IF EXISTS users").Error
	_ = db.Exec("DROP TABLE IF EXISTS gifts").Error
	_ = db.Exec("DROP TABLE IF EXISTS gift_orders").Error
	_ = db.Exec("DROP TABLE IF EXISTS super_chat_orders").Error
	_ = db.Exec("DROP TABLE IF EXISTS coin_transactions").Error
	_ = db.Exec("DROP TABLE IF EXISTS local_messages").Error
	_ = db.Exec("DROP TABLE IF EXISTS fan_badges").Error

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(64) NOT NULL DEFAULT '',
		display_name VARCHAR(64) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		coin_balance INTEGER NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rooms (
		id VARCHAR(64) PRIMARY KEY,
		channel VARCHAR(128) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		owner_id VARCHAR(36) NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?)",
		"u-demo", "demo", "Xiahaobo", "", balance,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?)",
		"u-owner", "owner", "Streamer", "owner.png", int64(0),
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO rooms (id, channel, avatar, owner_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		"r", "Streamer", "room.png", "u-owner", "r1", "Streamer", "room.png", "u-owner",
	).Error)

	require.NoError(t, db.AutoMigrate(&model.Gift{}, &model.GiftOrder{}, &model.SuperChatOrder{}, &model.CoinTransaction{}, &model.LocalMessage{}, &model.FanBadge{}))
	return db
}

func balanceOf(t *testing.T, db *gorm.DB, uid string) int64 {
	t.Helper()
	var b int64
	require.NoError(t, db.Raw("SELECT coin_balance FROM users WHERE id = ?", uid).Scan(&b).Error)
	return b
}

func seedGift(t *testing.T, db *gorm.DB, id string, price int64) {
	t.Helper()
	require.NoError(t, db.Save(&model.Gift{
		ID: id, Name: id, Icon: "x", PriceCoin: price, Category: "basic",
	}).Error)
}

func seedTopupTotal(t *testing.T, db *gorm.DB, uid string, amount int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.CoinTransaction{
		ID:           "topup-" + uid,
		UserID:       uid,
		Type:         model.CoinTxTopup,
		Amount:       amount,
		BalanceAfter: amount,
		Title:        "topup",
	}).Error)
}

// 1) Concurrent same-requestId: only one charge, others see the same order.
func TestGiftSend_ConcurrentSameRequestID(t *testing.T) {
	db := newTestDB(t, 10000)
	seedGift(t, db, "rocket", 100)

	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	const N = 10
	var (
		wg         sync.WaitGroup
		successes  atomic.Int32
		replays    atomic.Int32
		ordersSeen sync.Map
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			order, replayed, err := svc.Send(context.Background(), service.SendGiftReq{
				UserID:    "u-demo",
				Username:  "demo",
				RoomID:    "r1",
				GiftID:    "rocket",
				Count:     2,
				RequestID: "req-x",
			})
			require.NoError(t, err)
			require.NotNil(t, order)
			ordersSeen.Store(order.OrderID, struct{}{})
			if replayed {
				replays.Add(1)
			} else {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	// All 10 calls returned the same orderId (one stored entry).
	count := 0
	ordersSeen.Range(func(_, _ any) bool { count++; return true })
	require.Equal(t, 1, count, "all callers must receive the same orderId")

	require.EqualValues(t, 1, successes.Load(), "exactly one caller charges the wallet")
	require.EqualValues(t, N-1, replays.Load(), "the other N-1 must replay")
	require.EqualValues(t, 10000-200, balanceOf(t, db, "u-demo"), "wallet decremented exactly once")
	require.EqualValues(t, 200, balanceOf(t, db, "u-owner"), "streamer credited exactly once")

	// And only one row landed in gift_orders.
	var n int64
	require.NoError(t, db.Model(&model.GiftOrder{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}

// 2) Balance boundary: 100 coins, send 50×2=100 succeeds; next send 402.
func TestGiftSend_BalanceBoundary(t *testing.T) {
	db := newTestDB(t, 100)
	seedGift(t, db, "donut", 50)

	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))
	ctx := context.Background()

	// First send drains exactly to zero.
	order, replayed, err := svc.Send(ctx, service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "donut", Count: 2, RequestID: "r1",
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, model.StatusSuccess, order.Status)
	require.EqualValues(t, 0, balanceOf(t, db, "u-demo"))

	// Second send must fail with insufficient_coin AND persist a failed
	// order so that retries with same requestId replay the same failure.
	order, _, err = svc.Send(ctx, service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "donut", Count: 1, RequestID: "r2",
	})
	require.ErrorIs(t, err, service.ErrInsufficientCoin)
	require.NotNil(t, order)
	require.Equal(t, model.StatusFailed, order.Status)
	require.Equal(t, model.FailInsufficientCoin, order.FailReason)
	require.EqualValues(t, 0, balanceOf(t, db, "u-demo"), "balance must NOT change on failure")
	require.EqualValues(t, 100, balanceOf(t, db, "u-owner"), "successful gift credits the streamer")

	// Replay the same failure: same requestId, same outcome, no extra balance change.
	order2, _, err := svc.Send(ctx, service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "donut", Count: 1, RequestID: "r2",
	})
	require.ErrorIs(t, err, service.ErrInsufficientCoin)
	require.Equal(t, order.OrderID, order2.OrderID, "second hit returns the persisted failed order")
}

// A count large enough to wrap price*count negative used to credit the sender.
func TestGiftSend_RejectsOverflowingCount(t *testing.T) {
	db := newTestDB(t, 100)
	seedGift(t, db, "flower", 10)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))
	ctx := context.Background()

	for i, count := range []int{0, -1, service.MaxGiftCount + 1, 1844674407270955161} {
		order, _, err := svc.Send(ctx, service.SendGiftReq{
			UserID: "u-demo", RoomID: "r", GiftID: "flower", Count: count, RequestID: "overflow-" + string(rune('a'+i)),
		})
		require.ErrorIs(t, err, service.ErrInvalidGiftCount, "count %d", count)
		require.Nil(t, order)
	}
	require.EqualValues(t, 100, balanceOf(t, db, "u-demo"))
	require.EqualValues(t, 0, balanceOf(t, db, "u-owner"))

	var orders int64
	require.NoError(t, db.Model(&model.GiftOrder{}).Count(&orders).Error)
	require.Zero(t, orders)
}

func TestGiftSend_UsesResolvedDisplayNameInOutbox(t *testing.T) {
	db := newTestDB(t, 1000)
	seedGift(t, db, "flower", 10)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	order, replayed, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID:    "u-demo",
		Username:  "u-demo",
		RoomID:    "r",
		GiftID:    "flower",
		Count:     1,
		RequestID: "rq-name",
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, model.StatusSuccess, order.Status)

	var msgs []model.LocalMessage
	require.NoError(t, db.Find(&msgs).Error)
	require.Len(t, msgs, 1)
	require.Equal(t, model.OutboxTopicGift, msgs[0].Topic)
	require.Contains(t, msgs[0].Payload, `"user":"Xiahaobo"`)
	require.NotContains(t, msgs[0].Payload, `"user":"u-demo"`)
}

// 3) Gift not found.
func TestGiftSend_GiftNotFound(t *testing.T) {
	db := newTestDB(t, 1000)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))
	_, _, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "ghost", Count: 1, RequestID: "rq",
	})
	require.True(t, errors.Is(err, service.ErrGiftNotFound))
}

func TestGiftSend_LevelLockedGift(t *testing.T) {
	db := newTestDB(t, 1000)
	require.NoError(t, db.Save(&model.Gift{
		ID: "aurora", Name: "Aurora", Icon: "x", PriceCoin: 10, Category: "premium", UnlockLevel: 12,
	}).Error)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	_, _, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "aurora", Count: 1, RequestID: "locked",
	})
	require.ErrorIs(t, err, service.ErrGiftLevelLocked)
	require.EqualValues(t, 1000, balanceOf(t, db, "u-demo"))

	seedTopupTotal(t, db, "u-demo", userlevel.RequiredCoinsForLevel(12))
	order, replayed, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "aurora", Count: 1, RequestID: "unlocked",
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, model.StatusSuccess, order.Status)
}

// 4) Outbox row gets written alongside successful order.
func TestGiftSend_OutboxRowWritten(t *testing.T) {
	db := newTestDB(t, 1000)
	seedGift(t, db, "flower", 10)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	_, _, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "flower", Count: 3, RequestID: "rq",
	})
	require.NoError(t, err)

	var msgs []model.LocalMessage
	require.NoError(t, db.Find(&msgs).Error)
	require.Len(t, msgs, 1)
	require.Equal(t, "r", msgs[0].RoomID)
	require.Equal(t, model.OutboxTopicGift, msgs[0].Topic)
	require.Equal(t, model.OutboxStatusPending, msgs[0].Status)
	require.Contains(t, msgs[0].Payload, `"type":"gift"`)
}

// 5) Failed orders do NOT write to the outbox (no broadcast for failures).
func TestGiftSend_FailureDoesNotEnqueue(t *testing.T) {
	db := newTestDB(t, 5)
	seedGift(t, db, "flower", 10)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	_, _, err := svc.Send(context.Background(), service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "flower", Count: 1, RequestID: "rq",
	})
	require.ErrorIs(t, err, service.ErrInsufficientCoin)

	var n int64
	require.NoError(t, db.Model(&model.LocalMessage{}).Count(&n).Error)
	require.EqualValues(t, 0, n)
}

// 6) AmountToTier mapping (mirrors src/types/gift.ts).
func TestAmountToTier(t *testing.T) {
	cases := map[int64]int{
		0: 0, 199: 0, 200: 1, 999: 1, 1000: 2, 1999: 2,
		2000: 3, 4999: 3, 5000: 4, 9999: 4, 10000: 5, 100000: 5,
	}
	for amount, want := range cases {
		require.Equalf(t, want, service.AmountToTier(amount), "amount=%d", amount)
	}
}

func TestFanBadgeLevelThresholds(t *testing.T) {
	cases := map[int64]int{
		0: 1, 999: 1, 1000: 1, 1999: 1, 2000: 2, 5000: 5, 99000: 99, 120000: 99,
	}
	for contribution, want := range cases {
		require.Equalf(t, want, repo.FanBadgeLevel(contribution), "contribution=%d", contribution)
	}
}

func TestListFanBadgesRecomputesLevels(t *testing.T) {
	db := newTestDB(t, 0)
	now := time.Now()
	require.NoError(t, db.Create(&[]model.FanBadge{
		{
			UserID:            "u-demo",
			CreatorID:         "creator-low",
			CreatorName:       "Low",
			TotalContribution: 999,
			Level:             99,
			UpdatedAt:         now.Add(time.Minute),
		},
		{
			UserID:            "u-demo",
			CreatorID:         "creator-high",
			CreatorName:       "High",
			TotalContribution: 2500,
			Level:             1,
			UpdatedAt:         now,
		},
	}).Error)

	badges, err := repo.NewOrderRepo(db).ListFanBadges(context.Background(), "u-demo")
	require.NoError(t, err)
	require.Len(t, badges, 2)
	require.Equal(t, "creator-high", badges[0].CreatorID)
	require.Equal(t, 2, badges[0].Level)
	require.Equal(t, "creator-low", badges[1].CreatorID)
	require.Equal(t, 1, badges[1].Level)
}

func TestListFanClubMembersRanksByContribution(t *testing.T) {
	db := newTestDB(t, 0)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)",
		"fan-1", "fan1", "Fan One", "fan1.png", int64(0),
		"fan-2", "fan2", "Fan Two", "fan2.png", int64(0),
		"fan-3", "fan3", "Fan Three", "fan3.png", int64(0),
	).Error)
	now := time.Now()
	require.NoError(t, db.Create(&[]model.FanBadge{
		{
			UserID:            "fan-1",
			CreatorID:         "u-owner",
			CreatorName:       "Streamer",
			TotalContribution: 2000,
			Level:             90,
			UpdatedAt:         now.Add(-time.Minute),
		},
		{
			UserID:            "fan-2",
			CreatorID:         "u-owner",
			CreatorName:       "Streamer",
			TotalContribution: 5000,
			Level:             1,
			UpdatedAt:         now,
		},
		{
			UserID:            "fan-3",
			CreatorID:         "u-owner",
			CreatorName:       "Streamer",
			TotalContribution: 1000,
			Level:             1,
			UpdatedAt:         now.Add(-2 * time.Minute),
		},
		{
			UserID:            "u-owner",
			CreatorID:         "u-owner",
			CreatorName:       "Streamer",
			TotalContribution: 99000,
			Level:             99,
			UpdatedAt:         now,
		},
	}).Error)

	members, total, err := repo.NewOrderRepo(db).ListFanClubMembers(context.Background(), "u-owner", 2)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, members, 2)
	require.Equal(t, "fan-2", members[0].UserID)
	require.Equal(t, "Fan Two", members[0].Name)
	require.Equal(t, "fan2.png", members[0].Avatar)
	require.Equal(t, 5, members[0].Level)
	require.Equal(t, "fan-1", members[1].UserID)
	require.Equal(t, 2, members[1].Level)

	empty, emptyTotal, err := repo.NewOrderRepo(db).ListFanClubMembers(context.Background(), "missing", 5)
	require.NoError(t, err)
	require.EqualValues(t, 0, emptyTotal)
	require.NotNil(t, empty)
	require.Len(t, empty, 0)
}

func TestListFanBadgesUsesCurrentCreatorProfile(t *testing.T) {
	db := newTestDB(t, 0)
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?)",
		"creator-fresh", "fresh", "Fresh Name", "fresh.png", int64(0),
	).Error)
	require.NoError(t, db.Create(&model.FanBadge{
		UserID:            "u-demo",
		CreatorID:         "creator-fresh",
		CreatorName:       "Old Name",
		CreatorAvatar:     "old.png",
		TotalContribution: 30,
		Level:             1,
	}).Error)

	badges, err := repo.NewOrderRepo(db).ListFanBadges(context.Background(), "u-demo")
	require.NoError(t, err)
	require.Len(t, badges, 1)
	require.Equal(t, "Fresh Name", badges[0].CreatorName)
	require.Equal(t, "fresh.png", badges[0].CreatorAvatar)
}

func TestJoinFanClubWithoutLiveCreatesBadgeAndIncomeLedger(t *testing.T) {
	db := newTestDB(t, 2000)
	seedGift(t, db, "fan_light", 1000)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))

	order, replayed, err := svc.JoinFanClub(context.Background(), service.JoinFanClubReq{
		UserID:    "u-demo",
		CreatorID: "u-owner",
		RequestID: "join-fan-1",
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, model.StatusSuccess, order.Status)
	require.Equal(t, "", order.RoomID)
	require.EqualValues(t, 1000, balanceOf(t, db, "u-demo"))
	require.EqualValues(t, 1000, balanceOf(t, db, "u-owner"))

	var badge model.FanBadge
	require.NoError(t, db.Where("user_id = ? AND creator_id = ?", "u-demo", "u-owner").Take(&badge).Error)
	require.Equal(t, "Streamer", badge.CreatorName)
	require.Equal(t, "owner.png", badge.CreatorAvatar)

	var title string
	require.NoError(t, db.Raw(
		"SELECT title FROM coin_transactions WHERE user_id = ? AND type = ?",
		"u-owner",
		model.CoinTxCreatorGiftIncome,
	).Scan(&title).Error)
	require.Equal(t, "加入粉丝团收入", title)
}

// Joining again (the client sends a new requestId per click) used to charge
// the Fan Light price a second time and add it to the badge.
func TestJoinFanClub_ExistingMemberIsRefusedWithoutCharge(t *testing.T) {
	db := newTestDB(t, 5000)
	seedGift(t, db, "fan_light", 1000)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))
	ctx := context.Background()
	join := func(requestID string) (*model.GiftOrder, bool, error) {
		return svc.JoinFanClub(ctx, service.JoinFanClubReq{
			UserID: "u-demo", CreatorID: "u-owner", RequestID: requestID,
		})
	}

	first, _, err := join("join-1")
	require.NoError(t, err)

	order, replayed, err := join("join-2")
	require.ErrorIs(t, err, service.ErrAlreadyFanClubMember)
	require.Nil(t, order)
	require.False(t, replayed)

	// A retry of the join that succeeded replays it instead of being refused.
	order, replayed, err = join("join-1")
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.OrderID, order.OrderID)

	require.EqualValues(t, 4000, balanceOf(t, db, "u-demo"))
	require.EqualValues(t, 1000, balanceOf(t, db, "u-owner"))
	var badge model.FanBadge
	require.NoError(t, db.Where("user_id = ? AND creator_id = ?", "u-demo", "u-owner").Take(&badge).Error)
	require.EqualValues(t, 1000, badge.TotalContribution)
	var orders, ledger int64
	require.NoError(t, db.Model(&model.GiftOrder{}).Count(&orders).Error)
	require.EqualValues(t, 1, orders)
	require.NoError(t, db.Model(&model.CoinTransaction{}).Where("user_id = ?", "u-demo").Count(&ledger).Error)
	require.EqualValues(t, 1, ledger)
}

// A badge from the Fan Light gift is the same membership.
func TestJoinFanClub_FanLightGiftMemberIsRefused(t *testing.T) {
	db := newTestDB(t, 5000)
	seedGift(t, db, "fan_light", 1000)
	svc := service.NewGiftService(repo.NewGiftRepo(db), repo.NewOrderRepo(db))
	ctx := context.Background()

	_, _, err := svc.Send(ctx, service.SendGiftReq{
		UserID: "u-demo", RoomID: "r", GiftID: "fan_light", Count: 1, RequestID: "gift-1",
	})
	require.NoError(t, err)

	_, _, err = svc.JoinFanClub(ctx, service.JoinFanClubReq{
		UserID: "u-demo", CreatorID: "u-owner", RequestID: "join-1",
	})
	require.ErrorIs(t, err, service.ErrAlreadyFanClubMember)
	require.EqualValues(t, 4000, balanceOf(t, db, "u-demo"))
	require.EqualValues(t, 1000, balanceOf(t, db, "u-owner"))
}

// Repeat Fan Light gifts take the upsert's conflict path and other gifts only
// add to an existing badge; both keep the stored level in step with the total.
func TestFanBadgeContributionAccumulates(t *testing.T) {
	db := newTestDB(t, 5000)
	orders := repo.NewOrderRepo(db)
	place := func(requestID, giftID string, coin int64, mode repo.FanBadgeContributionMode) {
		t.Helper()
		_, _, err := orders.PlaceGiftOrder(context.Background(), &model.GiftOrder{
			OrderID: "gift-" + requestID, RequestID: requestID, UserID: "u-demo", RoomID: "r1",
			GiftID: giftID, Count: 1, TotalCoin: coin, Status: model.StatusSuccess,
		}, []byte("{}"), mode)
		require.NoError(t, err)
	}

	place("rq-1", "rocket", 500, repo.FanBadgeIfExists)
	var n int64
	require.NoError(t, db.Model(&model.FanBadge{}).Count(&n).Error)
	require.Zero(t, n, "a non-Fan-Light gift must not create a badge")

	place("rq-2", "fan_light", 1000, repo.FanBadgeCreate)
	place("rq-3", "fan_light", 1000, repo.FanBadgeCreate)
	place("rq-4", "rocket", 500, repo.FanBadgeIfExists)

	var badge model.FanBadge
	require.NoError(t, db.Where("user_id = ? AND creator_id = ?", "u-demo", "u-owner").Take(&badge).Error)
	require.EqualValues(t, 2500, badge.TotalContribution)
	require.Equal(t, 2, badge.Level)
	require.Equal(t, "Streamer", badge.CreatorName)
}

// 7) SC tier 0 rejected.
func TestSuperChat_TierZeroRejected(t *testing.T) {
	db := newTestDB(t, 1000)
	svc := service.NewSuperChatService(repo.NewOrderRepo(db))
	_, _, err := svc.Send(context.Background(), service.SendSuperChatReq{
		UserID: "u-demo", RoomID: "r", Amount: 50, Text: "hi", RequestID: "rq",
	})
	require.ErrorIs(t, err, service.ErrInvalidAmount)
	require.EqualValues(t, 1000, balanceOf(t, db, "u-demo"), "no charge on rejected SC")
}

// The per-tier text limits used to exist only in the web client, so longer
// text reached the database (and failed there with a 500).
func TestSuperChat_TextLimitPerTier(t *testing.T) {
	db := newTestDB(t, 20000)
	svc := service.NewSuperChatService(repo.NewOrderRepo(db))
	ctx := context.Background()

	for _, tc := range []struct {
		amount   int64
		maxRunes int
	}{
		{200, 50}, {1000, 100}, {2000, 150}, {5000, 200}, {10000, 200},
	} {
		tier := service.AmountToTier(tc.amount)
		require.Equal(t, tc.maxRunes, service.SuperChatMaxText(tier), "tier %d", tier)
		// Runes, not bytes ("赢" is three bytes); surrounding whitespace is trimmed.
		atLimit := strings.Repeat("赢", tc.maxRunes)
		order, _, err := svc.Send(ctx, service.SendSuperChatReq{
			UserID: "u-demo", RoomID: "r", Amount: tc.amount, Text: " \n" + atLimit + " ",
			RequestID: fmt.Sprintf("sc-ok-%d", tier),
		})
		require.NoError(t, err, "tier %d", tier)
		require.Equal(t, atLimit, order.Text)

		before := balanceOf(t, db, "u-demo")
		order, _, err = svc.Send(ctx, service.SendSuperChatReq{
			UserID: "u-demo", RoomID: "r", Amount: tc.amount, Text: atLimit + "!",
			RequestID: fmt.Sprintf("sc-long-%d", tier),
		})
		require.ErrorIs(t, err, service.ErrSuperChatTextTooLong, "tier %d", tier)
		require.Nil(t, order)
		require.Equal(t, before, balanceOf(t, db, "u-demo"), "tier %d: no charge", tier)
	}
	var orders int64
	require.NoError(t, db.Model(&model.SuperChatOrder{}).Count(&orders).Error)
	require.EqualValues(t, 5, orders, "a rejected super chat leaves no order row")
}

// Every tier's text must fit each column that stores it (MySQL counts
// varchar lengths in characters): the order row and the ledger description.
func TestSuperChatMaxTextFitsStorage(t *testing.T) {
	orderText := varcharLen(t, model.SuperChatOrder{}, "Text")
	ledgerDescription := varcharLen(t, model.CoinTransaction{}, "Description")
	for tier := 0; tier <= service.AmountToTier(math.MaxInt64); tier++ {
		require.LessOrEqual(t, service.SuperChatMaxText(tier), orderText, "tier %d", tier)
		require.LessOrEqual(t, service.SuperChatMaxText(tier), ledgerDescription, "tier %d", tier)
	}
}

// varcharLen returns N from the field's gorm:"type:varchar(N)" tag.
func varcharLen(t *testing.T, v any, field string) int {
	t.Helper()
	f, ok := reflect.TypeOf(v).FieldByName(field)
	require.True(t, ok, field)
	m := regexp.MustCompile(`varchar\((\d+)\)`).FindStringSubmatch(f.Tag.Get("gorm"))
	require.NotNil(t, m, "%s has no varchar size", field)
	n, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	return n
}

func TestSuperChat_UsesResolvedDisplayNameInOutbox(t *testing.T) {
	db := newTestDB(t, 1000)
	svc := service.NewSuperChatService(repo.NewOrderRepo(db))

	order, replayed, err := svc.Send(context.Background(), service.SendSuperChatReq{
		UserID:    "u-demo",
		Username:  "u-demo",
		RoomID:    "r",
		Amount:    200,
		Text:      "hello",
		RequestID: "rq",
	})
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, model.StatusSuccess, order.Status)

	var msgs []model.LocalMessage
	require.NoError(t, db.Find(&msgs).Error)
	require.Len(t, msgs, 1)
	require.Equal(t, model.OutboxTopicSuperChat, msgs[0].Topic)
	require.Contains(t, msgs[0].Payload, `"user":"Xiahaobo"`)
	require.NotContains(t, msgs[0].Payload, `"user":"u-demo"`)
	require.EqualValues(t, 800, balanceOf(t, db, "u-demo"))
	require.EqualValues(t, 200, balanceOf(t, db, "u-owner"))
}
