package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

// newBetTestDB extends newTestDB with the bet tables and two more viewers.
func newBetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDB(t, 1000)
	_ = db.Exec("DROP TABLE IF EXISTS bet_rounds").Error
	_ = db.Exec("DROP TABLE IF EXISTS bet_wagers").Error
	require.NoError(t, db.AutoMigrate(&model.BetRound{}, &model.BetWager{}))
	for _, id := range []string{"u-a", "u-b"} {
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, username, coin_balance) VALUES (?, ?, ?)", id, id, int64(1000),
		).Error)
	}
	return db
}

func TestBetRefundOrphanedWagers(t *testing.T) {
	db := newBetTestDB(t)
	orders := repo.NewOrderRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, r := range []model.BetRound{
		{ID: "bet-settled", RoomID: "r1", OwnerID: "u-owner", Question: "q", Amount: 100, Status: model.BetRoundSettled, CloseAt: now},
		{ID: "bet-open", RoomID: "r", OwnerID: "u-owner", Question: "q", Amount: 100, Status: model.BetRoundOpen, CloseAt: now.Add(time.Minute)},
	} {
		require.NoError(t, db.Create(&r).Error)
	}
	// u-a's stake slipped in after settle (the old race); u-b's belongs to a
	// round that is still open and must be left alone.
	require.NoError(t, db.Exec("UPDATE users SET coin_balance = coin_balance - 100 WHERE id IN ('u-a', 'u-b')").Error)
	require.NoError(t, db.Create(&model.BetWager{ID: "bw-orphan", RoundID: "bet-settled", RoomID: "r1", UserID: "u-a", Option: model.BetOptionLose, Amount: 100, Status: model.StatusLocked}).Error)
	require.NoError(t, db.Create(&model.BetWager{ID: "bw-live", RoundID: "bet-open", RoomID: "r", UserID: "u-b", Option: model.BetOptionWin, Amount: 100, Status: model.StatusLocked}).Error)

	n, err := orders.RefundOrphanedBetWagers(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, int64(1000), balanceOf(t, db, "u-a"))
	require.Equal(t, int64(900), balanceOf(t, db, "u-b"))

	var orphan, live model.BetWager
	require.NoError(t, db.Where("id = ?", "bw-orphan").Take(&orphan).Error)
	require.Equal(t, model.StatusRefunded, orphan.Status)
	require.Equal(t, int64(100), orphan.Payout)
	require.NoError(t, db.Where("id = ?", "bw-live").Take(&live).Error)
	require.Equal(t, model.StatusLocked, live.Status)

	var refunds int64
	require.NoError(t, db.Model(&model.CoinTransaction{}).
		Where("user_id = ? AND type = ?", "u-a", model.CoinTxBetRefund).Count(&refunds).Error)
	require.Equal(t, int64(1), refunds)

	// Idempotent: a second pass finds nothing to refund.
	n, err = orders.RefundOrphanedBetWagers(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, int64(1000), balanceOf(t, db, "u-a"))
}
