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

func TestRecommendedLiveUsesPersonalSignalsAndCategory(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, seedRecommendationMetricTables(db))

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	mr := miniredis.RunT(t)
	social := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc := NewRoomService(rooms, "", social)
	svc.now = func() time.Time { return now }

	require.NoError(t, seedRecommendedRoom(ctx, rooms, "live-follow", "owner-a", "Gaming", model.StatusLive, now.Add(-20*time.Minute), 20))
	require.NoError(t, seedRecommendedRoom(ctx, rooms, "live-hot", "owner-b", "Music", model.StatusLive, now.Add(-10*time.Minute), 900))
	require.NoError(t, seedRecommendedRoom(ctx, rooms, "live-gaming", "owner-c", "Gaming", model.StatusLive, now.Add(-30*time.Minute), 80))
	require.NoError(t, seedRecommendedRoom(ctx, rooms, "old-follow", "owner-a", "Gaming", model.StatusEnded, now.Add(-48*time.Hour), 120))

	require.NoError(t, social.Follow(ctx, "viewer-1", "ch-owner-a"))
	require.NoError(t, social.Follow(ctx, "fan-1", "ch-owner-b"))
	require.NoError(t, social.Follow(ctx, "fan-2", "ch-owner-b"))
	require.NoError(t, seedRecommendationMetrics(db))
	require.NoError(t, svc.RecordWatch(ctx, "viewer-1", "old-follow"))

	resp, err := svc.RecommendedLive(ctx, "viewer-1", "", 12)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Items)
	require.Equal(t, "live-follow", resp.Items[0].ID)

	musicResp, err := svc.RecommendedLive(ctx, "viewer-1", "Music", 12)
	require.NoError(t, err)
	require.Len(t, musicResp.Items, 1)
	require.Equal(t, "live-hot", musicResp.Items[0].ID)

	gamingResp, err := svc.RecommendedLive(ctx, "viewer-1", "Gaming", 12)
	require.NoError(t, err)
	require.Len(t, gamingResp.Items, 2)
	for _, item := range gamingResp.Items {
		require.Equal(t, "Gaming", item.Category)
	}
}

func seedRecommendationMetricTables(db *gorm.DB) error {
	return db.Exec(`
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
`).Error
}

func seedRecommendedRoom(ctx context.Context, rooms *repo.RoomRepo, id, ownerID, category, status string, startedAt time.Time, viewers int64) error {
	var endedAt *time.Time
	if status == model.StatusEnded {
		ended := startedAt.Add(time.Hour)
		endedAt = &ended
	}
	return rooms.Upsert(ctx, &model.Room{
		ID:           id,
		Title:        id,
		Channel:      "Creator " + ownerID,
		ChannelID:    "ch-" + ownerID,
		Category:     category,
		OwnerID:      ownerID,
		Status:       status,
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Viewers:      viewers,
		PeakViewers:  viewers,
		ReplayStatus: model.ReplayStatusNone,
	})
}

func seedRecommendationMetrics(db *gorm.DB) error {
	if err := db.Exec(`
INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES
	('old-follow', 'viewer-1', 800, 'success', '2026-05-04 10:00:00'),
	('live-hot', 'fan-1', 2000, 'success', '2026-05-05 09:55:00');
INSERT INTO super_chat_orders (room_id, user_id, amount, status, created_at) VALUES
	('live-hot', 'fan-2', 1200, 'success', '2026-05-05 09:56:00');
`).Error; err != nil {
		return err
	}
	for i := 0; i < 80; i++ {
		if err := db.Exec("INSERT INTO danmus_0 (room_id) VALUES (?)", "live-hot").Error; err != nil {
			return err
		}
	}
	return nil
}
