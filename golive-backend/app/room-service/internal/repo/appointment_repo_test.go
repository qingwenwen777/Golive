package repo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

func newAppointmentTestRepo(t *testing.T) (*AppointmentRepo, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	repo := NewAppointmentRepo(db)
	require.NoError(t, repo.AutoMigrate())
	return repo, db
}

// Each notification row binds 13 placeholders; one INSERT for a popular
// appointment's audience exceeded MySQL's 65,535 limit.
func TestCreateNotificationsInsertsInBoundedBatches(t *testing.T) {
	repo, db := newAppointmentTestRepo(t)
	ctx := context.Background()
	var batchRows []int
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:batch_rows", func(tx *gorm.DB) {
		if tx.Statement.Table == "notifications" {
			batchRows = append(batchRows, tx.Statement.ReflectValue.Len())
		}
	}))

	now := time.Now()
	notifications := make([]model.Notification, 0, 1234)
	for i := 0; i < 1234; i++ {
		notifications = append(notifications, model.Notification{
			ID:        fmt.Sprintf("n-%04d", i),
			UserID:    fmt.Sprintf("user-%04d", i),
			Type:      "appointment_reminder",
			Title:     "Reminder",
			CreatedAt: now,
		})
	}
	require.NoError(t, repo.CreateNotifications(ctx, notifications[:10]))
	// Re-sending already delivered rows is ignored in every batch.
	require.NoError(t, repo.CreateNotifications(ctx, notifications))

	var count int64
	require.NoError(t, db.Model(&model.Notification{}).Count(&count).Error)
	require.EqualValues(t, 1234, count)
	require.Equal(t, []int{10, notificationInsertBatch, notificationInsertBatch, 1234 - 2*notificationInsertBatch}, batchRows)
}

func TestWatcherIDsAfterPagesInUserIDOrder(t *testing.T) {
	repo, _ := newAppointmentTestRepo(t)
	ctx := context.Background()
	for _, userID := range []string{"u-3", "u-1", "u-5", "u-2", "u-4"} {
		require.NoError(t, repo.Reserve(ctx, "appt-1", userID))
	}
	require.NoError(t, repo.Reserve(ctx, "appt-2", "u-0"))

	page, err := repo.WatcherIDsAfter(ctx, "appt-1", "", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"u-1", "u-2"}, page)
	page, err = repo.WatcherIDsAfter(ctx, "appt-1", page[len(page)-1], 2)
	require.NoError(t, err)
	require.Equal(t, []string{"u-3", "u-4"}, page)
	page, err = repo.WatcherIDsAfter(ctx, "appt-1", page[len(page)-1], 2)
	require.NoError(t, err)
	require.Equal(t, []string{"u-5"}, page)
}
