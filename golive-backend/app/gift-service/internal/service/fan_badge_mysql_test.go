package service_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

// pauseAfterRoomLookup parks the next gift transaction right after its
// rooms SELECT, which is where MySQL's REPEATABLE READ snapshot is taken.
// It returns a channel closed once a transaction is parked and a release func.
func pauseAfterRoomLookup(t *testing.T, db *gorm.DB) (<-chan struct{}, func()) {
	t.Helper()
	var armed atomic.Bool
	armed.Store(true)
	reached := make(chan struct{})
	release := make(chan struct{})
	unblock := closeOnce(release)
	t.Cleanup(unblock)
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:pause_room_lookup", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "FROM rooms") && armed.CompareAndSwap(true, false) {
			close(reached)
			<-release
		}
	}))
	return reached, unblock
}

func placeMySQLGift(ctx context.Context, orders *repo.OrderRepo, userID, giftID string, coin int64, mode repo.FanBadgeContributionMode) error {
	_, _, err := orders.PlaceGiftOrder(ctx, &model.GiftOrder{
		OrderID:   "gift-" + uuid.NewString(),
		RequestID: uuid.NewString(),
		UserID:    userID,
		RoomID:    "r1",
		GiftID:    giftID,
		Count:     1,
		TotalCoin: coin,
		Status:    model.StatusSuccess,
	}, []byte("{}"), mode)
	return err
}

// pauseBeforeGiftOrderInsert parks the next transaction that inserts a
// gift_orders row, i.e. after it has debited the buyer, and so while it holds
// the buyer's users row lock. It returns a channel closed once a transaction
// is parked and a release func.
func pauseBeforeGiftOrderInsert(t *testing.T, db *gorm.DB) (<-chan struct{}, func()) {
	t.Helper()
	var armed atomic.Bool
	armed.Store(true)
	reached := make(chan struct{})
	release := make(chan struct{})
	unblock := closeOnce(release)
	t.Cleanup(unblock)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:pause_gift_order", func(tx *gorm.DB) {
		if tx.Statement.Table == "gift_orders" && armed.CompareAndSwap(true, false) {
			close(reached)
			<-release
		}
	}))
	return reached, unblock
}

func joinMySQLFanClub(ctx context.Context, orders *repo.OrderRepo, userID, creatorID string) error {
	_, _, err := orders.PlaceFanClubJoinOrder(ctx, &model.GiftOrder{
		OrderID:   "gift-" + uuid.NewString(),
		RequestID: uuid.NewString(),
		UserID:    userID,
		GiftID:    "fan_light",
		Count:     1,
		TotalCoin: 1000,
		Status:    model.StatusSuccess,
	}, creatorID)
	return err
}

func fanBadgeOf(t *testing.T, db *gorm.DB, userID, creatorID string) model.FanBadge {
	t.Helper()
	var badge model.FanBadge
	require.NoError(t, db.Where("user_id = ? AND creator_id = ?", userID, creatorID).Take(&badge).Error)
	return badge
}

// A gift whose snapshot predates a concurrent gift's commit must still add
// its coins on top of that gift's, not overwrite the badge total.
func TestMySQLFanBadge_ConcurrentGiftsDoNotLoseContribution(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-fan", 10000)
	require.NoError(t, db.Create(&model.FanBadge{
		UserID: "u-fan", CreatorID: "u-owner", CreatorName: "owner",
		TotalContribution: 1000, Level: 1,
	}).Error)
	orders := repo.NewOrderRepo(db)
	ctx := context.Background()

	reached, unblock := pauseAfterRoomLookup(t, db)
	firstErr := make(chan error, 1)
	go func() { firstErr <- placeMySQLGift(ctx, orders, "u-fan", "rocket", 500, repo.FanBadgeIfExists) }()
	<-reached
	require.NoError(t, placeMySQLGift(ctx, orders, "u-fan", "rocket", 500, repo.FanBadgeIfExists))
	unblock()
	require.NoError(t, <-firstErr)

	require.Equal(t, int64(9000), balanceOf(t, db, "u-fan"))
	badge := fanBadgeOf(t, db, "u-fan", "u-owner")
	require.Equal(t, int64(2000), badge.TotalContribution)
	require.Equal(t, 2, badge.Level)
}

// Two first-time Fan Light gifts racing to create the badge must both
// succeed and both count, instead of one failing on the duplicate key.
func TestMySQLFanBadge_ConcurrentFirstFanLightBothSucceed(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-fan", 10000)
	orders := repo.NewOrderRepo(db)
	ctx := context.Background()

	reached, unblock := pauseAfterRoomLookup(t, db)
	firstErr := make(chan error, 1)
	go func() { firstErr <- placeMySQLGift(ctx, orders, "u-fan", "fan_light", 1000, repo.FanBadgeCreate) }()
	<-reached
	require.NoError(t, placeMySQLGift(ctx, orders, "u-fan", "fan_light", 1000, repo.FanBadgeCreate))
	unblock()
	require.NoError(t, <-firstErr)

	require.Equal(t, int64(8000), balanceOf(t, db, "u-fan"))
	badge := fanBadgeOf(t, db, "u-fan", "u-owner")
	require.Equal(t, int64(2000), badge.TotalContribution)
	require.Equal(t, 2, badge.Level)
	require.Equal(t, "u-owner", badge.CreatorName)
}

// Two joins by the same fan (a double click sends two requestIds) used to
// both debit the Fan Light price. The second must wait for the first and
// then be refused.
func TestMySQLFanClub_ConcurrentJoinsChargeOnce(t *testing.T) {
	db, dbName := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-fan", 5000)
	orders := repo.NewOrderRepo(db)
	ctx := context.Background()

	reached, unblock := pauseBeforeGiftOrderInsert(t, db)
	firstErr := make(chan error, 1)
	go func() { firstErr <- joinMySQLFanClub(ctx, orders, "u-fan", "u-owner") }()
	<-reached

	secondDone := make(chan struct{})
	var secondErr error
	go func() {
		defer close(secondDone)
		secondErr = joinMySQLFanClub(ctx, orders, "u-fan", "u-owner")
	}()
	waitDoneOrLockWait(t, db, dbName, secondDone)
	unblock()
	require.NoError(t, <-firstErr)
	<-secondDone
	require.ErrorIs(t, secondErr, repo.ErrAlreadyFanClubMember)

	require.Equal(t, int64(4000), balanceOf(t, db, "u-fan"))
	require.Equal(t, int64(1000), balanceOf(t, db, "u-owner"))
	require.Equal(t, int64(1000), fanBadgeOf(t, db, "u-fan", "u-owner").TotalContribution)
	var joins int64
	require.NoError(t, db.Model(&model.GiftOrder{}).Where("user_id = ?", "u-fan").Count(&joins).Error)
	require.Equal(t, int64(1), joins)
}

// Joins by different fans must not wait on each other's membership check: a
// locking read of the missing badge would take a gap lock that deadlocks
// their badge inserts.
func TestMySQLFanClub_ConcurrentJoinsByDifferentFansSucceed(t *testing.T) {
	db, dbName := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-fan-a", 5000)
	insertMySQLUser(t, db, "u-fan-b", 5000)
	orders := repo.NewOrderRepo(db)
	ctx := context.Background()

	reached, unblock := pauseBeforeGiftOrderInsert(t, db)
	firstErr := make(chan error, 1)
	go func() { firstErr <- joinMySQLFanClub(ctx, orders, "u-fan-a", "u-owner") }()
	<-reached

	secondDone := make(chan struct{})
	var secondErr error
	go func() {
		defer close(secondDone)
		secondErr = joinMySQLFanClub(ctx, orders, "u-fan-b", "u-owner")
	}()
	waitDoneOrLockWait(t, db, dbName, secondDone)
	unblock()
	require.NoError(t, <-firstErr)
	<-secondDone
	require.NoError(t, secondErr)

	require.Equal(t, int64(4000), balanceOf(t, db, "u-fan-a"))
	require.Equal(t, int64(4000), balanceOf(t, db, "u-fan-b"))
	require.Equal(t, int64(2000), balanceOf(t, db, "u-owner"))
	require.Equal(t, int64(1000), fanBadgeOf(t, db, "u-fan-a", "u-owner").TotalContribution)
	require.Equal(t, int64(1000), fanBadgeOf(t, db, "u-fan-b", "u-owner").TotalContribution)
}
