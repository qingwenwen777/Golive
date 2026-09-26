package repo_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// forEachReplayRepoDB runs fn against SQLite and, when GOLIVE_TEST_MYSQL_DSN
// names a server (root:root@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC),
// against a fresh MySQL database on it that is dropped afterwards.
func forEachReplayRepoDB(t *testing.T, fn func(t *testing.T, rooms *repo.RoomRepo)) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), cfg)
		require.NoError(t, err)
		rooms := repo.NewRoomRepo(db)
		require.NoError(t, rooms.AutoMigrate())
		fn(t, rooms)
	})
	t.Run("mysql", func(t *testing.T) {
		dsn := strings.TrimSpace(os.Getenv("GOLIVE_TEST_MYSQL_DSN"))
		if dsn == "" {
			t.Skip("GOLIVE_TEST_MYSQL_DSN not set; skipping MySQL-backed test")
		}
		slash := strings.LastIndex(dsn, "/")
		require.GreaterOrEqual(t, slash, 0)
		prefix, params := dsn[:slash+1], ""
		if q := strings.IndexByte(dsn[slash+1:], '?'); q >= 0 {
			params = dsn[slash+1+q:]
		}
		buf := make([]byte, 6)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		name := "golive_test_roomsvc_" + hex.EncodeToString(buf)
		admin, err := gorm.Open(mysql.Open(prefix+params), cfg)
		require.NoError(t, err)
		adminSQL, err := admin.DB()
		require.NoError(t, err)
		require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4").Error)
		t.Cleanup(func() {
			_ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error
			_ = adminSQL.Close()
		})
		db, err := gorm.Open(mysql.Open(prefix+name+params), cfg)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		rooms := repo.NewRoomRepo(db)
		require.NoError(t, rooms.AutoMigrate())
		fn(t, rooms)
	})
}

// replayRetryRooms stores a live room and an ended room for each replay
// upload state around now, each named after its state.
func replayRetryRooms(t *testing.T, rooms *repo.RoomRepo, now time.Time) {
	t.Helper()
	ended := now.Add(-48 * time.Hour)
	due, later := now.Add(-time.Minute), now.Add(time.Hour)
	recent, longAgo := now.Add(-24*time.Hour), now.Add(-10*24*time.Hour)
	for _, room := range []model.Room{
		{ID: "live", Status: model.StatusLive},
		{ID: "pending", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusPending},
		{ID: "uploading", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusUploading},
		{ID: "ready", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusReady},
		{ID: "none", ReplayStatus: model.ReplayStatusNone},
		// Failed before retries were counted.
		{ID: "failed-legacy", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed},
		{ID: "failed-due", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed,
			ReplayAttempts: 1, ReplayFailedAt: &recent, ReplayRetryAt: &due},
		{ID: "failed-later", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed,
			ReplayAttempts: 2, ReplayFailedAt: &recent, ReplayRetryAt: &later},
		{ID: "gave-up-recently", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed,
			ReplayAttempts: 7, ReplayFailedAt: &recent},
		{ID: "gave-up-long-ago", ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed,
			ReplayAttempts: 7, ReplayFailedAt: &longAgo},
		{ID: "failed-upload-off", ReplayStatus: model.ReplayStatusFailed},
		{ID: "deleted", ReplayStatus: model.ReplayStatusDeleted, ReplayDeletedAt: &recent},
	} {
		room.Title, room.OwnerID = room.ID, "owner-"+room.ID
		room.StartedAt = ended.Add(-time.Hour)
		if room.Status == "" {
			room.Status, room.EndedAt = model.StatusEnded, &ended
		}
		require.NoError(t, rooms.Upsert(context.Background(), &room))
	}
}

func roomIDs(rooms []model.Room) []string {
	ids := make([]string, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.ID)
	}
	return ids
}

func TestReplayRetryableUploads(t *testing.T) {
	forEachReplayRepoDB(t, func(t *testing.T, rooms *repo.RoomRepo) {
		ctx := context.Background()
		now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		replayRetryRooms(t, rooms, now)

		all, err := rooms.ReplayRetryableUploads(ctx, time.Time{}, 0)
		require.NoError(t, err)
		require.Equal(t, []string{"failed-legacy", "failed-due", "failed-later"}, roomIDs(all))

		due, err := rooms.ReplayRetryableUploads(ctx, now, 0)
		require.NoError(t, err)
		require.Equal(t, []string{"failed-legacy", "failed-due"}, roomIDs(due))

		first, err := rooms.ReplayRetryableUploads(ctx, time.Time{}, 1)
		require.NoError(t, err)
		require.Equal(t, []string{"failed-legacy"}, roomIDs(first))
	})
}

func TestRecordingRoomsKeepsFailedUploadsWithRetriesOrWithinRetention(t *testing.T) {
	forEachReplayRepoDB(t, func(t *testing.T, rooms *repo.RoomRepo) {
		now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		replayRetryRooms(t, rooms, now)

		got, err := rooms.RecordingRooms(context.Background(), now.Add(-7*24*time.Hour))
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"live", "pending", "uploading", "failed-legacy", "failed-due", "failed-later", "gave-up-recently",
		}, roomIDs(got))
	})
}

func TestSetReplayFailedThenClaimReplayRetry(t *testing.T) {
	forEachReplayRepoDB(t, func(t *testing.T, rooms *repo.RoomRepo) {
		ctx := context.Background()
		now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		replayRetryRooms(t, rooms, now)
		retryAt := now.Add(15 * time.Minute)

		updated, err := rooms.SetReplayFailed(ctx, "uploading", "upload failed (attempt 1 of 7)", 1, now, &retryAt)
		require.NoError(t, err)
		require.True(t, updated)
		got, err := rooms.GetByID(ctx, "uploading")
		require.NoError(t, err)
		require.Equal(t, model.ReplayStatusFailed, got.ReplayStatus)
		require.Equal(t, "upload failed (attempt 1 of 7)", got.ReplayError)
		require.Equal(t, 1, got.ReplayAttempts)
		require.True(t, got.ReplayFailedAt.Equal(now), got.ReplayFailedAt)
		require.True(t, got.ReplayRetryAt.Equal(retryAt), got.ReplayRetryAt)

		claimed, err := rooms.ClaimReplayRetry(ctx, "uploading", 0)
		require.NoError(t, err)
		require.False(t, claimed, "claimed with stale attempts")
		claimed, err = rooms.ClaimReplayRetry(ctx, "uploading", 1)
		require.NoError(t, err)
		require.True(t, claimed)
		got, err = rooms.GetByID(ctx, "uploading")
		require.NoError(t, err)
		require.Equal(t, model.ReplayStatusPending, got.ReplayStatus)
		require.Empty(t, got.ReplayError)
		require.Nil(t, got.ReplayRetryAt)
		require.Equal(t, 1, got.ReplayAttempts)
		claimed, err = rooms.ClaimReplayRetry(ctx, "uploading", 1)
		require.NoError(t, err)
		require.False(t, claimed, "claimed twice")

		updated, err = rooms.SetReplayFailed(ctx, "uploading", "gave up", 7, now, nil)
		require.NoError(t, err)
		require.True(t, updated)
		got, err = rooms.GetByID(ctx, "uploading")
		require.NoError(t, err)
		require.Nil(t, got.ReplayRetryAt)

		for _, id := range []string{"deleted", "failed-upload-off"} {
			claimed, err := rooms.ClaimReplayRetry(ctx, id, 0)
			require.NoError(t, err)
			require.False(t, claimed, id)
		}
		updated, err = rooms.SetReplayFailed(ctx, "deleted", "upload failed", 1, now, &retryAt)
		require.NoError(t, err)
		require.False(t, updated, "a deleted replay stays deleted")
		got, err = rooms.GetByID(ctx, "deleted")
		require.NoError(t, err)
		require.Equal(t, model.ReplayStatusDeleted, got.ReplayStatus)
	})
}
