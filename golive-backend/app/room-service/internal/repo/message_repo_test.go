package repo

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

func TestSyncFanGroupsCanRunAgainAndAddNewMembers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	repo := NewMessageRepo(db)
	require.NoError(t, repo.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE fan_badges (
	user_id TEXT NOT NULL,
	creator_id TEXT NOT NULL,
	creator_name TEXT,
	creator_avatar TEXT,
	total_contribution INTEGER DEFAULT 0,
	level INTEGER DEFAULT 1,
	created_at DATETIME,
	updated_at DATETIME
)`).Error)

	ctx := context.Background()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	creatorID := "creator-1"
	insertFanBadge(t, db, "fan-1", creatorID, now)
	insertFanBadge(t, db, "fan-2", creatorID, now.Add(time.Second))

	groups, err := repo.SyncFanGroups(ctx, creatorID, "Creator", now)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	groupID := groups[0].ID
	requireFanGroupMemberCount(t, db, groupID, 3)

	insertFanBadge(t, db, "fan-3", creatorID, now.Add(2*time.Second))
	groups, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, groupID, groups[0].ID)
	requireFanGroupMemberCount(t, db, groupID, 4)

	var fan3 model.FanGroupMember
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", groupID, "fan-3").Take(&fan3).Error)
	require.Nil(t, fan3.KickedAt)
}

func insertFanBadge(t *testing.T, db *gorm.DB, userID, creatorID string, updatedAt time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO fan_badges (user_id, creator_id, creator_name, creator_avatar, updated_at, created_at)
VALUES (?, ?, 'Creator', '', ?, ?)`,
		userID,
		creatorID,
		updatedAt,
		updatedAt,
	).Error)
}

func requireFanGroupMemberCount(t *testing.T, db *gorm.DB, groupID string, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.FanGroupMember{}).
		Where("group_id = ? AND kicked_at IS NULL", groupID).
		Count(&count).Error)
	require.Equal(t, want, count)
}
