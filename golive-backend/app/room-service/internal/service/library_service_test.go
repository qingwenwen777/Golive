package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func TestUserLibraryPersistsPerUserAndKeepsLatestSyncTime(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	svc := NewRoomService(rooms, "")
	svc.now = func() time.Time { return now }
	require.NoError(t, seedLibraryRoom(ctx, rooms, "room-1", "owner-1", model.StatusLive, now.Add(-time.Hour)))

	_, err = svc.SaveUserLibraryItem(ctx, "viewer-1", "watch_later", UserLibraryItemInput{RoomID: "room-1"})
	require.NoError(t, err)
	older := now.Add(-24 * time.Hour).Format(time.RFC3339)
	_, err = svc.SyncUserLibrary(ctx, "viewer-1", "watch_later", SyncUserLibraryReq{
		Items: []UserLibraryItemInput{{RoomID: "room-1", SavedAt: older}},
	})
	require.NoError(t, err)

	resp, err := svc.ListUserLibrary(ctx, "viewer-1", "watch-later")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "room-1", resp.Items[0].ID)
	require.Equal(t, now.Format(time.RFC3339), resp.Items[0].SavedAt)

	other, err := svc.ListUserLibrary(ctx, "viewer-2", "watch_later")
	require.NoError(t, err)
	require.Empty(t, other.Items)
}

func TestRecordWatchAddsHistoryLibraryItem(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	now := time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC)
	svc := NewRoomService(rooms, "")
	svc.now = func() time.Time { return now }
	require.NoError(t, seedLibraryRoom(ctx, rooms, "room-2", "owner-2", model.StatusLive, now.Add(-30*time.Minute)))

	require.NoError(t, svc.RecordWatch(ctx, "viewer-1", "room-2"))

	resp, err := svc.ListUserLibrary(ctx, "viewer-1", "history")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "room-2", resp.Items[0].ID)
	require.Equal(t, now.Format(time.RFC3339), resp.Items[0].WatchedAt)

	_, err = svc.RemoveUserLibraryItem(ctx, "viewer-1", "history", "room-2")
	require.NoError(t, err)
	rows, err := rooms.WatchCategoryRows(ctx, "viewer-1", now.Add(-time.Hour))
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestRecordWatchHeartbeatsAreRateLimitedAndDaily(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	now := time.Date(2026, 5, 7, 13, 0, 0, 0, time.UTC)
	svc := NewRoomService(rooms, "")
	svc.now = func() time.Time { return now }
	require.NoError(t, seedLibraryRoom(ctx, rooms, "room-heartbeat", "owner-2", model.StatusLive, now.Add(-30*time.Minute)))

	require.NoError(t, svc.RecordWatch(ctx, "viewer-1", "room-heartbeat"))
	now = now.Add(10 * time.Second)
	require.NoError(t, svc.RecordWatch(ctx, "viewer-1", "room-heartbeat"))
	now = now.Add(20 * time.Second)
	require.NoError(t, svc.RecordWatch(ctx, "viewer-1", "room-heartbeat"))

	var event model.RoomWatchEvent
	require.NoError(t, db.Where("user_id = ? AND room_id = ?", "viewer-1", "room-heartbeat").Take(&event).Error)
	require.Equal(t, int64(2), event.WatchCount)
	require.Equal(t, int64(2), event.DailyWatchCount)
	require.Equal(t, "2026-05-07", event.WatchDate)
}

func TestLikedLibraryStaysConsistentWithLikeState(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	mr := miniredis.RunT(t)
	socialRepo := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	now := time.Date(2026, 5, 7, 14, 0, 0, 0, time.UTC)
	roomSvc := NewRoomService(rooms, "", socialRepo)
	roomSvc.now = func() time.Time { return now }
	socialSvc := NewSocialService(socialRepo, rooms)
	require.NoError(t, seedLibraryRoom(ctx, rooms, "room-3", "owner-3", model.StatusLive, now.Add(-20*time.Minute)))

	_, err = socialSvc.Like(ctx, "viewer-1", "room-3")
	require.NoError(t, err)
	resp, err := roomSvc.ListUserLibrary(ctx, "viewer-1", "liked")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)

	_, err = socialSvc.Unlike(ctx, "viewer-1", "room-3")
	require.NoError(t, err)
	resp, err = roomSvc.ListUserLibrary(ctx, "viewer-1", "liked")
	require.NoError(t, err)
	require.Empty(t, resp.Items)

	_, err = roomSvc.SyncUserLibrary(ctx, "viewer-1", "liked", SyncUserLibraryReq{
		Items: []UserLibraryItemInput{{RoomID: "room-3", SavedAt: now.Format(time.RFC3339)}},
	})
	require.NoError(t, err)
	likeState, err := socialRepo.GetLike(ctx, "room-3", "viewer-1")
	require.NoError(t, err)
	require.True(t, likeState.Liked)

	_, err = roomSvc.ClearUserLibrary(ctx, "viewer-1", "liked")
	require.NoError(t, err)
	likeState, err = socialRepo.GetLike(ctx, "room-3", "viewer-1")
	require.NoError(t, err)
	require.False(t, likeState.Liked)
}

func seedLibraryRoom(ctx context.Context, rooms *repo.RoomRepo, id, ownerID, status string, startedAt time.Time) error {
	return rooms.Upsert(ctx, &model.Room{
		ID:           id,
		Title:        id,
		Channel:      "Creator " + ownerID,
		ChannelID:    "ch-" + ownerID,
		Category:     "Gaming",
		OwnerID:      ownerID,
		Status:       status,
		StartedAt:    startedAt,
		Viewers:      12,
		PeakViewers:  20,
		ReplayStatus: model.ReplayStatusNone,
	})
}
