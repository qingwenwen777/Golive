package service

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
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
