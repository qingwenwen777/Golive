package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

func TestNormalizeCategory(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"   ":          "",
		"all":          "",
		"All":          "",
		"ALL":          "",
		"すべて":          "",
		"Music":        "Music",
		"Apex Legends": "Apex Legends",
		"音楽":           "音楽",
	}
	for in, want := range cases {
		require.Equalf(t, want, service.NormalizeCategory(in), "input=%q", in)
	}
}

// Make sure the JSON wire shape never includes streamKey when the field is
// empty (non-publisher view). This guards the contract literally — a stray
// `streamKey` ever leaking would let viewers republish.
func TestStream_StripsStreamKeyWhenEmpty(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Channel: "c", ChannelID: "ch", Category: "Gaming"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "streamKey")
}

func TestStream_IncludesStreamKeyForOwner(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Channel: "c", ChannelID: "ch", Category: "Gaming", StreamKey: "lk_xxx"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"streamKey":"lk_xxx"`)
}

func TestStream_IncludesDescription(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Description: "creator intro", Channel: "c", ChannelID: "ch", Category: "Gaming"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"description":"creator intro"`)
}

func TestListCapsPageSize(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	started := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 105; i++ {
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID:           fmt.Sprintf("live-%03d", i),
			Title:        "Live",
			ChannelID:    fmt.Sprintf("ch-owner-%d", i),
			OwnerID:      fmt.Sprintf("owner-%d", i),
			Status:       model.StatusLive,
			StartedAt:    started.Add(time.Duration(i) * time.Second),
			ReplayStatus: model.ReplayStatusNone,
		}))
	}
	svc := service.NewRoomService(rooms, "")

	resp, err := svc.List(ctx, "", "", 1, 100000)
	require.NoError(t, err)
	require.Equal(t, 100, resp.Size)
	require.Len(t, resp.Items, 100)
	require.Equal(t, int64(105), resp.Total)

	resp, err = svc.List(ctx, "", "", 2, 100000)
	require.NoError(t, err)
	require.Len(t, resp.Items, 5)
}
