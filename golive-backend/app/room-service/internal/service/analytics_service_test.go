package service

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestHistoryByChannelHidesCreatorMetricsFromNonOwners(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, seedHotReplayMetricTables(db))

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	svc := NewRoomService(rooms, "")
	svc.SetReplayService(NewReplayService(rooms, nil, ReplayConfig{BunnyLibraryID: "lib"}))

	endedAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, seedHotReplayRoom(ctx, rooms, "history-public", endedAt, 50, model.PostVisibilityPublic))
	require.NoError(t, db.Exec(
		"INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES (?, ?, ?, ?, ?)",
		"history-public", "fan-1", 700, "success", endedAt.Add(-time.Minute),
	).Error)

	cases := []struct {
		name        string
		viewerID    string
		replaysOnly bool
	}{
		{"anonymous history", "", false},
		{"anonymous replays", "", true},
		{"other user history", "viewer-1", false},
		{"other user replays", "viewer-1", true},
	}
	for _, tc := range cases {
		resp, err := svc.HistoryByChannel(ctx, "owner-hot", tc.viewerID, 1, 10, tc.replaysOnly)
		require.NoError(t, err, tc.name)
		require.Len(t, resp.Items, 1, tc.name)
		item := resp.Items[0]
		require.Nil(t, item.RevenueCoin, tc.name)
		require.Nil(t, item.NewSubscribers, tc.name)
		require.Nil(t, item.TopFan, tc.name)

		raw, err := json.Marshal(resp)
		require.NoError(t, err)
		for _, leaked := range []string{"revenueCoin", "newSubscribers", "topFan", "fan-1"} {
			require.NotContains(t, string(raw), leaked, tc.name)
		}
	}

	ownerResp, err := svc.HistoryByChannel(ctx, "owner-hot", "owner-hot", 1, 10, false)
	require.NoError(t, err)
	require.Len(t, ownerResp.Items, 1)
	owned := ownerResp.Items[0]
	require.NotNil(t, owned.RevenueCoin)
	require.Equal(t, int64(700), *owned.RevenueCoin)
	require.NotNil(t, owned.TopFan)
	require.Equal(t, "fan-1", owned.TopFan.UserID)
	require.NotNil(t, owned.NewSubscribers)
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

	resp, err := svc.HotReplays(ctx, "", "", 3, 3)
	require.NoError(t, err)
	require.Equal(t, int64(4), resp.Total)
	require.Len(t, resp.Items, 3)
	require.Equal(t, []string{"replay-coin", "replay-chat", "replay-peak"}, []string{
		resp.Items[0].ID,
		resp.Items[1].ID,
		resp.Items[2].ID,
	})
	require.Equal(t, int64(900), resp.Items[0].RevenueCoin)
	body, err := json.Marshal(resp.Items[0])
	require.NoError(t, err)
	require.NotContains(t, string(body), "revenueCoin")
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

func TestLiveAnalysisRanksTopFansAcrossGiftsAndSuperChats(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	ownerID := f.owners[3]
	roomID := "replay-03"
	at := f.now.Add(-time.Hour)
	// The fixture already gives this room gifts of 40..80 from fan-0..fan-4
	// and a 103 super chat from fan-sc.
	require.NoError(t, f.db.Exec(`
INSERT INTO users (id, username, display_name, avatar) VALUES
	('whale', 'whale', 'Big Whale', '/whale.png'),
	('split', 'splitter', '', '');
INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES
	('replay-03', 'whale', 500, 'success', ?),
	('replay-03', 'whale', 500, 'success', ?),
	('replay-03', 'whale', 9000, 'failed', ?),
	('replay-03', 'split', 300, 'success', ?),
	('replay-03', '', 5000, 'success', ?),
	('replay-04', 'whale', 7000, 'success', ?);
INSERT INTO super_chat_orders (room_id, user_id, amount, status, created_at) VALUES
	('replay-03', 'split', 250, 'success', ?),
	('replay-03', 'fan-0', 40, 'success', ?);
`, at, at, at, at, at, at, at, at).Error)
	for i := 0; i < 6; i++ {
		require.NoError(t, f.db.Exec(
			"INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES (?, ?, 10, 'success', ?)",
			roomID, fmt.Sprintf("minnow-%d", i), at,
		).Error)
	}

	resp, err := f.svc.LiveAnalysis(ctx, "ch-"+ownerID, roomID, ownerID)
	require.NoError(t, err)
	require.Len(t, resp.TopFans, 8)
	require.Equal(t, FanContribution{UserID: "whale", Name: "Big Whale", Avatar: "/whale.png", Amount: 1000}, resp.TopFans[0])
	require.Equal(t, FanContribution{UserID: "split", Name: "splitter", Amount: 550}, resp.TopFans[1])
	require.Equal(t, FanContribution{UserID: "fan-sc", Name: "fan-sc", Amount: 103}, resp.TopFans[2])
	fans := make([]string, 0, len(resp.TopFans))
	for _, fan := range resp.TopFans[3:] {
		fans = append(fans, fmt.Sprintf("%s:%d", fan.UserID, fan.Amount))
	}
	// fan-0's gift and super chat add up to tie fan-4; ties go by user id.
	require.Equal(t, []string{"fan-0:80", "fan-4:80", "fan-3:70", "fan-2:60", "fan-1:50"}, fans)
	// Fixture gifts 40..80 plus 1000+300+5000+60 here; super chats 103+250+40.
	require.Equal(t, int64(300+1000+300+5000+60), resp.GiftRevenue)
	require.Equal(t, int64(103+250+40), resp.SuperChatRevenue)
	require.NotNil(t, resp.Record.RevenueCoin)
	require.Equal(t, resp.GiftRevenue+resp.SuperChatRevenue, *resp.Record.RevenueCoin)
	require.NotNil(t, resp.Record.TopFan)
	require.Equal(t, "whale", resp.Record.TopFan.UserID)

	history, err := f.svc.HistoryByChannel(ctx, "ch-"+ownerID, ownerID, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, history.Items, 1)
	require.Equal(t, "whale", history.Items[0].TopFan.UserID)
	require.Equal(t, int64(1000), history.Items[0].TopFan.Amount)
}

func TestLiveAnalysisWithoutRevenueReturnsEmptyTopFans(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	require.NoError(t, f.db.Exec("DELETE FROM gift_orders").Error)
	require.NoError(t, f.db.Exec("DELETE FROM super_chat_orders").Error)
	ownerID := f.owners[2]
	resp, err := f.svc.LiveAnalysis(ctx, "ch-"+ownerID, "replay-02", ownerID)
	require.NoError(t, err)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"topFans":[]`)
	require.Zero(t, resp.GiftRevenue)
	require.Nil(t, resp.Record.TopFan)
	require.Equal(t, int64(0), *resp.Record.RevenueCoin)
}

// BenchmarkLiveAnalysisTopFans measures LiveAnalysis for a room with 20k and
// 40k distinct fans. Ranking them with the previous O(n²) in-process sort
// took ~2.1s and ~8.8s per call on this fixture.
func BenchmarkLiveAnalysisTopFans(b *testing.B) {
	ctx := context.Background()
	for _, fans := range []int{20000, 40000} {
		b.Run(fmt.Sprintf("fans=%d", fans), func(b *testing.B) {
			f := newListPerfFixture(b)
			ownerID := f.owners[1]
			seedTopFanOrders(b, f.db, "replay-01", fans, f.now.Add(-time.Hour))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp, err := f.svc.LiveAnalysis(ctx, "ch-"+ownerID, "replay-01", ownerID)
				if err != nil {
					b.Fatal(err)
				}
				if len(resp.TopFans) != 8 {
					b.Fatalf("got %d top fans", len(resp.TopFans))
				}
			}
		})
	}
}

// seedTopFanOrders gives roomID gifts from fans distinct users.
func seedTopFanOrders(t testing.TB, db *gorm.DB, roomID string, fans int, at time.Time) {
	t.Helper()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < fans; i++ {
			if err := tx.Exec(
				"INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES (?, ?, ?, 'success', ?)",
				roomID, fmt.Sprintf("fan-%06d", i), (i*7919)%100000+1, at,
			).Error; err != nil {
				return err
			}
		}
		return nil
	}))
}
