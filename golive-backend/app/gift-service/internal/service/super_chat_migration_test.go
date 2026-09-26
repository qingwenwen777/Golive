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

// Before moderated_at existed, moderation hid a paid super chat by marking it
// status=failed, fail_reason=moderated, which dropped it from revenue although
// its coins had moved. The startup migration must turn exactly those rows
// back into paid, hidden super chats, and running it again must change
// nothing.
func TestMigrateLegacyModeratedSuperChats(t *testing.T) {
	testMigrateLegacyModeratedSuperChats(t, newTestDB(t, 0))
}

func TestMySQLMigrateLegacyModeratedSuperChats(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	testMigrateLegacyModeratedSuperChats(t, db)
}

func testMigrateLegacyModeratedSuperChats(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	created := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	hiddenLater := created.Add(time.Hour)
	orders := []model.SuperChatOrder{
		{OrderID: "sc-legacy", Status: model.StatusFailed, FailReason: "moderated"},
		{OrderID: "sc-unpaid", Status: model.StatusFailed, FailReason: model.FailInsufficientCoin},
		{OrderID: "sc-hidden", Status: model.StatusSuccess, ModeratedAt: &hiddenLater},
		{OrderID: "sc-paid", Status: model.StatusSuccess},
	}
	for i := range orders {
		o := &orders[i]
		o.RequestID, o.UserID, o.RoomID = "req-"+o.OrderID, "u-demo", "r1"
		o.Amount, o.Tier, o.Text, o.CreatedAt = 3000, 2, "hello", created
		require.NoError(t, db.Create(o).Error)
	}

	orderRepo := repo.NewOrderRepo(db)
	n, err := orderRepo.MigrateLegacyModeratedSuperChats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	byID := func() map[string]model.SuperChatOrder {
		var rows []model.SuperChatOrder
		require.NoError(t, db.Order("order_id").Find(&rows).Error)
		out := make(map[string]model.SuperChatOrder, len(rows))
		for _, row := range rows {
			out[row.OrderID] = row
		}
		return out
	}
	rows := byID()
	legacy := rows["sc-legacy"]
	require.Equal(t, model.StatusSuccess, legacy.Status)
	require.Empty(t, legacy.FailReason)
	require.NotNil(t, legacy.ModeratedAt, "stays hidden from public history")
	require.True(t, legacy.ModeratedAt.Equal(created), "hidden since %v", legacy.ModeratedAt)

	require.Equal(t, model.StatusFailed, rows["sc-unpaid"].Status)
	require.Equal(t, model.FailInsufficientCoin, rows["sc-unpaid"].FailReason)
	require.Nil(t, rows["sc-unpaid"].ModeratedAt)
	require.Equal(t, model.StatusSuccess, rows["sc-hidden"].Status)
	require.True(t, rows["sc-hidden"].ModeratedAt.Equal(hiddenLater))
	require.Equal(t, model.StatusSuccess, rows["sc-paid"].Status)
	require.Nil(t, rows["sc-paid"].ModeratedAt)

	n, err = orderRepo.MigrateLegacyModeratedSuperChats(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, rows, byID())
}
