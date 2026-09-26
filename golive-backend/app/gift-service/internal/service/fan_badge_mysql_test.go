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
