package repo_test

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

// newChannelOwnerDB seeds users as user-service stores them; later rows were
// updated more recently.
func newChannelOwnerDB(t *testing.T, users ...[3]string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (id varchar(36) primary key, username varchar(64), display_name varchar(64), avatar varchar(500), verified boolean, updated_at datetime)`).Error)
	updated := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for i, u := range users {
		require.NoError(t, db.Exec(`INSERT INTO users (id, username, display_name, avatar, verified, updated_at) VALUES (?, ?, ?, '', false, ?)`,
			u[0], u[1], u[2], updated.Add(time.Duration(i)*time.Hour)).Error)
	}
	require.NoError(t, repo.NewRoomRepo(db).AutoMigrate())
	return db
}

// A user who renames themselves to a creator's username, and updates their
// profile last, must not take over /channel/<username>, following by name or
// channel reports.
func TestChannelKeyPrefersUsernameOverAnyDisplayName(t *testing.T) {
	ctx := context.Background()
	db := newChannelOwnerDB(t,
		[3]string{"creator-id", "kabun", "Kabun Live"},
		[3]string{"impostor-id", "impostor", "kabun"},
	)
	rooms := repo.NewRoomRepo(db)

	for _, key := range []string{"kabun", "ch-kabun"} {
		ownerID, err := rooms.ResolveOwnerID(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, "creator-id", ownerID, key)
	}
	target, err := repo.NewModerationRepo(db, nil).ResolveReportTarget(ctx, model.ReportTargetChannel, "kabun", "")
	require.NoError(t, err)
	require.Equal(t, "creator-id", target.OwnerID)
}

// Display names are not unique: one names a channel only while exactly one
// user has it.
func TestChannelKeyResolvesDisplayNameOnlyWhenUnique(t *testing.T) {
	ctx := context.Background()
	db := newChannelOwnerDB(t,
		[3]string{"solo-id", "solo", "Solo Streamer"},
		[3]string{"twin-a", "twin_a", "Twins"},
		[3]string{"twin-b", "twin_b", "Twins"},
	)
	rooms := repo.NewRoomRepo(db)
	moderation := repo.NewModerationRepo(db, nil)

	ownerID, err := rooms.ResolveOwnerID(ctx, "Solo Streamer")
	require.NoError(t, err)
	require.Equal(t, "solo-id", ownerID)
	target, err := moderation.ResolveReportTarget(ctx, model.ReportTargetChannel, "Solo Streamer", "")
	require.NoError(t, err)
	require.Equal(t, "solo-id", target.OwnerID)

	// A room labelled with the shared name does not settle it either.
	require.NoError(t, rooms.Upsert(ctx, &model.Room{ID: "room-twin", OwnerID: "twin-b", ChannelID: "ch-twin-b", Channel: "Twins", Status: model.StatusEnded, StartedAt: time.Now()}))
	_, err = rooms.ResolveOwnerID(ctx, "Twins")
	require.ErrorIs(t, err, repo.ErrRoomNotFound)
	_, err = moderation.ResolveReportTarget(ctx, model.ReportTargetChannel, "Twins", "")
	require.ErrorIs(t, err, repo.ErrReportTargetNotFound)
}

// Keys that name no user fall back to rooms: owner and channel ids as
// before, a channel label (chosen by whoever went live) only while a single
// owner uses it.
func TestChannelKeyFallsBackToRoomsWithoutGuessing(t *testing.T) {
	ctx := context.Background()
	db := newChannelOwnerDB(t)
	rooms := repo.NewRoomRepo(db)
	started := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for i, room := range []model.Room{
		{ID: "legacy-1", OwnerID: "legacy-owner", ChannelID: "ch-legacy-owner", Channel: "Morning Show"},
		{ID: "legacy-2", OwnerID: "legacy-owner", ChannelID: "ch-legacy-owner", Channel: "Morning Show"},
		{ID: "late-a", OwnerID: "owner-a", ChannelID: "ch-owner-a", Channel: "Late Show"},
		{ID: "late-b", OwnerID: "owner-b", ChannelID: "ch-owner-b", Channel: "Late Show"},
	} {
		room.Status = model.StatusEnded
		room.StartedAt = started.Add(time.Duration(i) * time.Hour)
		require.NoError(t, rooms.Upsert(ctx, &room))
	}

	for key, want := range map[string]string{"legacy-owner": "legacy-owner", "ch-owner-b": "owner-b", "Morning Show": "legacy-owner"} {
		ownerID, err := rooms.ResolveOwnerID(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, want, ownerID, key)
	}
	for _, key := range []string{"Late Show", "nobody"} {
		_, err := rooms.ResolveOwnerID(ctx, key)
		require.ErrorIs(t, err, repo.ErrRoomNotFound, key)
	}
}
