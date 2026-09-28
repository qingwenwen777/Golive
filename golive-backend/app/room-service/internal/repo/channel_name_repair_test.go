package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// Rooms were stored with a raw user uuid, or the English "Creator <owner id
// prefix>" label, as the channel name of a creator without a name. The
// startup repair gives them the owner's name, or clears the channel so the
// apps can label it; real names stay. The query is MySQL-only.
func TestFixUUIDChannelsRepairsChannelsThatAreNotNames(t *testing.T) {
	ctx := context.Background()
	db := openTestMySQL(t)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	require.NoError(t, db.Exec(`CREATE TABLE users (id varchar(36) primary key, username varchar(64), display_name varchar(64), avatar varchar(500))`).Error)

	const (
		named   = "11111111-1111-4111-8111-111111111111"
		unnamed = "22222222-2222-4222-8222-222222222222"
		deleted = "33333333-3333-4333-8333-333333333333"
	)
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, display_name, avatar) VALUES
		(?, 'luna', 'Luna', '/luna.png'), (?, ?, '', '')`, named, unnamed, unnamed).Error)
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for id, room := range map[string][2]string{ // owner, channel
		"uuid":             {named, named},
		"label":            {named, "Creator 11111111"},
		"uuid-unnamed":     {unnamed, unnamed},
		"label-unnamed":    {unnamed, "Creator 22222222"},
		"label-no-user":    {deleted, "Creator 33333333"},
		"real-name":        {named, "Creator Fans"},
		"someone-elses-id": {named, "Creator 22222222"},
	} {
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID: id, Title: id, OwnerID: room[0], Channel: room[1],
			Status: model.StatusEnded, StartedAt: now, ReplayStatus: model.ReplayStatusNone,
		}))
	}

	n, err := rooms.FixUUIDChannels(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, n)
	for id, channel := range map[string]string{
		"uuid":             "Luna",
		"label":            "Luna",
		"uuid-unnamed":     "",
		"label-unnamed":    "",
		"label-no-user":    "",
		"real-name":        "Creator Fans",
		"someone-elses-id": "Creator 22222222",
	} {
		room, err := rooms.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, channel, room.Channel, id)
	}
	room, err := rooms.GetByID(ctx, "label")
	require.NoError(t, err)
	require.Equal(t, "/luna.png", room.Avatar, "the owner's avatar comes along")

	n, err = rooms.FixUUIDChannels(ctx)
	require.NoError(t, err)
	require.Zero(t, n, "nothing left to repair")
}
