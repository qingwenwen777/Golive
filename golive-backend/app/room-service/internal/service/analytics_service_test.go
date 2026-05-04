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

func TestHistoryByChannelViewerModeReturnsOnlyWatchableReplays(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	svc := NewRoomService(rooms, "")
	svc.SetReplayService(NewReplayService(rooms, nil, ReplayConfig{BunnyLibraryID: "lib"}))

	ownerID := "00000000-0000-0000-0000-000000000101"
	startedAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Hour)
	uploadedAt := endedAt.Add(2 * time.Minute)

	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:           "plain-history",
		Title:        "Plain history",
		Channel:      "Creator",
		ChannelID:    "ch-" + ownerID,
		Category:     "Gaming",
		OwnerID:      ownerID,
		Status:       model.StatusEnded,
		StartedAt:    startedAt,
		EndedAt:      &endedAt,
		ReplayStatus: model.ReplayStatusNone,
	}))
	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:                   "private-replay",
		Title:                "Private replay",
		Channel:              "Creator",
		ChannelID:            "ch-" + ownerID,
		Category:             "Gaming",
		OwnerID:              ownerID,
		Status:               model.StatusEnded,
		StartedAt:            startedAt.Add(-time.Hour),
		EndedAt:              &endedAt,
		ReplayStatus:         model.ReplayStatusReady,
		ReplayVisibility:     model.PostVisibilityPrivate,
		ReplayBunnyVideoID:   "private-video",
		ReplayBunnyLibraryID: "lib",
		ReplayUploadedAt:     &uploadedAt,
	}))
	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:                   "public-replay",
		Title:                "Public replay",
		Channel:              "Creator",
		ChannelID:            "ch-" + ownerID,
		Category:             "Gaming",
		OwnerID:              ownerID,
		Status:               model.StatusEnded,
		StartedAt:            startedAt.Add(-2 * time.Hour),
		EndedAt:              &endedAt,
		ReplayStatus:         model.ReplayStatusReady,
		ReplayVisibility:     model.PostVisibilityPublic,
		ReplayBunnyVideoID:   "public-video",
		ReplayBunnyLibraryID: "lib",
		ReplayUploadedAt:     &uploadedAt,
	}))

	viewerResp, err := svc.HistoryByChannel(ctx, "ch-"+ownerID, "", 1, 10, true)
	require.NoError(t, err)
	require.Equal(t, int64(1), viewerResp.Total)
	require.Len(t, viewerResp.Items, 1)
	require.Equal(t, "public-replay", viewerResp.Items[0].ID)
	require.NotNil(t, viewerResp.Items[0].Replay)
	require.True(t, viewerResp.Items[0].Replay.CanWatch)

	ownerResp, err := svc.HistoryByChannel(ctx, "ch-"+ownerID, ownerID, 1, 10, false)
	require.NoError(t, err)
	require.Equal(t, int64(3), ownerResp.Total)
}

func TestHotReplaysRanksWatchableRecentReplays(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, seedHotReplayMetricTables(db))

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	mr := miniredis.RunT(t)
	social := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	svc := NewRoomService(rooms, "", social)
	svc.now = func() time.Time { return now }
	replays := NewReplayService(rooms, social, ReplayConfig{BunnyLibraryID: "lib"})
	svc.SetReplayService(replays)

	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-low", now.Add(-1*time.Hour), 20, model.PostVisibilityPublic))
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-coin", now.Add(-2*time.Hour), 60, model.PostVisibilityPublic))
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-chat", now.Add(-3*time.Hour), 30, model.PostVisibilityPublic))
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-peak", now.Add(-4*time.Hour), 300, model.PostVisibilityPublic))
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-old", now.Add(-96*time.Hour), 500, model.PostVisibilityPublic))
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "replay-private", now.Add(-30*time.Minute), 900, model.PostVisibilityPrivate))

	require.NoError(t, social.SeedLikeCount(ctx, "replay-low", 1))
	require.NoError(t, social.SeedLikeCount(ctx, "replay-coin", 3))
	require.NoError(t, social.SeedLikeCount(ctx, "replay-chat", 2))
	require.NoError(t, social.SeedLikeCount(ctx, "replay-peak", 2))
	require.NoError(t, social.SeedLikeCount(ctx, "replay-old", 100))
	require.NoError(t, social.SeedLikeCount(ctx, "replay-private", 100))
	require.NoError(t, seedHotReplayMetrics(db))

	resp, err := svc.HotReplays(ctx, "", 3, 3)
	require.NoError(t, err)
	require.Equal(t, int64(4), resp.Total)
	require.Len(t, resp.Items, 3)
	require.Equal(t, []string{"replay-coin", "replay-chat", "replay-peak"}, []string{
		resp.Items[0].ID,
		resp.Items[1].ID,
		resp.Items[2].ID,
	})
	require.Equal(t, int64(900), resp.Items[0].RevenueCoin)
	require.Equal(t, int64(40), resp.Items[1].CommentCount)
	require.Equal(t, int64(300), resp.Items[2].PeakViewers)
}

func seedHotReplayMetricTables(db *gorm.DB) error {
	return db.Exec(`
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	username TEXT,
	display_name TEXT,
	avatar TEXT
);
CREATE TABLE gift_orders (
	room_id TEXT,
	user_id TEXT,
	total_coin INTEGER,
	status TEXT,
	created_at DATETIME
);
CREATE TABLE super_chat_orders (
	room_id TEXT,
	user_id TEXT,
	amount INTEGER,
	status TEXT,
	created_at DATETIME
);
CREATE TABLE danmus_0 (
	room_id TEXT
);
INSERT INTO users (id, username, display_name, avatar) VALUES ('fan-1', 'fan1', 'Fan One', '');
`).Error
}

func seedHotReplayRoom(ctx context.Context, rooms *repo.RoomRepo, id string, endedAt time.Time, peak int64, visibility string) error {
	startedAt := endedAt.Add(-1 * time.Hour)
	uploadedAt := endedAt.Add(2 * time.Minute)
	return rooms.Upsert(ctx, &model.Room{
		ID:                   id,
		Title:                id,
		Channel:              "Creator",
		ChannelID:            "ch-hot",
		Category:             "Gaming",
		OwnerID:              "owner-hot",
		Status:               model.StatusEnded,
		StartedAt:            startedAt,
		EndedAt:              &endedAt,
		PeakViewers:          peak,
		ReplayStatus:         model.ReplayStatusReady,
		ReplayVisibility:     visibility,
		ReplayBunnyVideoID:   id + "-video",
		ReplayBunnyLibraryID: "lib",
		ReplayUploadedAt:     &uploadedAt,
	})
}

func seedHotReplayMetrics(db *gorm.DB) error {
	if err := db.Exec(`
INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES
	('replay-low', 'fan-1', 10, 'success', '2026-05-04 11:00:00'),
	('replay-coin', 'fan-1', 900, 'success', '2026-05-04 10:00:00'),
	('replay-chat', 'fan-1', 20, 'success', '2026-05-04 09:00:00');
`).Error; err != nil {
		return err
	}
	for i := 0; i < 1; i++ {
		if err := db.Exec("INSERT INTO danmus_0 (room_id) VALUES (?)", "replay-low").Error; err != nil {
			return err
		}
	}
	for i := 0; i < 2; i++ {
		if err := db.Exec("INSERT INTO danmus_0 (room_id) VALUES (?)", "replay-coin").Error; err != nil {
			return err
		}
	}
	for i := 0; i < 40; i++ {
		if err := db.Exec("INSERT INTO danmus_0 (room_id) VALUES (?)", "replay-chat").Error; err != nil {
			return err
		}
	}
	for i := 0; i < 1; i++ {
		if err := db.Exec("INSERT INTO danmus_0 (room_id) VALUES (?)", "replay-peak").Error; err != nil {
			return err
		}
	}
	return nil
}
