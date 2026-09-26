package repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

func TestAdminAdjustCoinsBalancesAndLedger(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	require.NoError(t, db.Create(&model.User{
		ID: "u1", Username: "u1", PasswordHash: "hash", CoinBalance: 100, Banned: true,
	}).Error)

	steps := []struct {
		action       string
		amount       int64
		wantErr      error
		wantBalance  int64
		wantFrozen   int64
		wantType     string
		wantAmount   int64
		wantTitle    string
		wantBalAfter int64
	}{
		{"add", 50, nil, 150, 0, model.CoinTxAdminAdjust, 50, "Admin coin adjustment", 150},
		{"freeze", 151, ErrInsufficientCoins, 150, 0, "", 0, "", 0},
		{"freeze", 120, nil, 150, 120, model.CoinTxAdminFreeze, 0, "Admin coin freeze", 150},
		// Deduct ignores the ban and the frozen coins; only the total counts.
		{"deduct", 140, nil, 10, 120, model.CoinTxAdminAdjust, -140, "Admin coin adjustment", 10},
		{"deduct", 11, ErrInsufficientCoins, 10, 120, "", 0, "", 0},
		{"unfreeze", 20, nil, 10, 100, model.CoinTxAdminUnfreeze, 0, "Admin coin unfreeze", 10},
		{"unfreeze", 500, nil, 10, 0, model.CoinTxAdminUnfreeze, 0, "Admin coin unfreeze", 10},
		{"deduct", 0, ErrInsufficientCoins, 10, 0, "", 0, "", 0},
	}
	ledgerRows := 0
	for _, step := range steps {
		u, tx, err := users.AdminAdjustCoins(ctx, "u1", step.action, step.amount, "  note  ", "op-1")
		if step.wantErr != nil {
			require.ErrorIs(t, err, step.wantErr, "%s %d", step.action, step.amount)
		} else {
			require.NoError(t, err, "%s %d", step.action, step.amount)
			ledgerRows++
			require.Equal(t, step.wantBalance, u.CoinBalance)
			require.Equal(t, step.wantFrozen, u.FrozenCoins)
			require.NotEmpty(t, tx.ID)
			require.Equal(t, "u1", tx.UserID)
			require.Equal(t, step.wantType, tx.Type)
			require.Equal(t, step.wantAmount, tx.Amount)
			require.Equal(t, step.wantTitle, tx.Title)
			require.Equal(t, step.wantBalAfter, tx.BalanceAfter)
			require.Equal(t, "note", tx.Description)
			require.Equal(t, "admin", tx.SourceType)
			require.Equal(t, "op-1", tx.SourceID)
			require.Equal(t, "op-1", tx.CounterpartyID)
		}
		var row model.User
		require.NoError(t, db.Where("id = ?", "u1").Take(&row).Error)
		require.Equal(t, step.wantBalance, row.CoinBalance, "%s %d", step.action, step.amount)
		require.Equal(t, step.wantFrozen, row.FrozenCoins, "%s %d", step.action, step.amount)
		var count int64
		require.NoError(t, db.Model(&model.CoinTransaction{}).Where("user_id = ?", "u1").Count(&count).Error)
		require.EqualValues(t, ledgerRows, count)
	}

	_, _, err = users.AdminAdjustCoins(ctx, "missing", "add", 5, "", "op-1")
	require.ErrorIs(t, err, ErrUserNotFound)
	_, _, err = users.AdminAdjustCoins(ctx, "u1", "burn", 5, "", "op-1")
	require.EqualError(t, err, "invalid coin action")
}
