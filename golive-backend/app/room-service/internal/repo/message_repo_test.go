package repo

import (
	"context"
	"fmt"
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

func TestSyncFanGroupsDoesNotRestoreManuallyKickedMember(t *testing.T) {
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

	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, groupID, "fan-1", "", nil, false, true, false, false, now.Add(time.Minute)))
	groups, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Len(t, groups, 1)
	requireFanGroupMemberCount(t, db, groupID, 2)

	var kicked model.FanGroupMember
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", groupID, "fan-1").Take(&kicked).Error)
	require.NotNil(t, kicked.KickedAt)
	require.Equal(t, model.FanGroupKickReasonManual, kicked.KickReason)

	require.NoError(t, repo.RequestFanGroupRejoin(ctx, groupID, "fan-1", now.Add(3*time.Minute)))
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", groupID, "fan-1").Take(&kicked).Error)
	require.NotNil(t, kicked.RejoinRequestedAt)
	require.NotNil(t, kicked.KickedAt)

	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, groupID, "fan-1", "", nil, false, false, true, false, now.Add(4*time.Minute)))
	requireFanGroupMemberCount(t, db, groupID, 3)
	kicked = model.FanGroupMember{}
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", groupID, "fan-1").Take(&kicked).Error)
	require.Nil(t, kicked.KickedAt)
	require.Nil(t, kicked.RejoinRequestedAt)
}

func newFanGroupTestRepo(t *testing.T) (*MessageRepo, *gorm.DB) {
	t.Helper()
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
	return repo, db
}

func fanID(i int) string { return fmt.Sprintf("fan-%03d", i) }

// activeFanGroups lists the groups where userID is an active member.
func activeFanGroups(t *testing.T, db *gorm.DB, userID string) []string {
	t.Helper()
	var groupIDs []string
	require.NoError(t, db.Model(&model.FanGroupMember{}).
		Where("user_id = ? AND kicked_at IS NULL", userID).
		Order("group_id").
		Pluck("group_id", &groupIDs).Error)
	return groupIDs
}

func TestSyncFanGroupsPreservesMutes(t *testing.T) {
	repo, db := newFanGroupTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	creatorID := "creator-1"
	insertFanBadge(t, db, "fan-1", creatorID, now)
	insertFanBadge(t, db, "admin-1", creatorID, now.Add(time.Second))

	groups, err := repo.SyncFanGroups(ctx, creatorID, "Creator", now)
	require.NoError(t, err)
	groupID := groups[0].ID
	mutedUntil := now.Add(time.Hour)
	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, groupID, "admin-1", model.FanGroupRoleAdmin, &mutedUntil, false, false, false, false, now))
	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, groupID, "fan-1", "", &mutedUntil, false, false, false, false, now))

	_, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(time.Minute))
	require.NoError(t, err)
	for _, userID := range []string{"fan-1", "admin-1"} {
		var member model.FanGroupMember
		require.NoError(t, db.Where("group_id = ? AND user_id = ?", groupID, userID).Take(&member).Error)
		require.Nil(t, member.KickedAt)
		require.NotNil(t, member.MutedUntil, "sync lifted the mute of %s", userID)
		require.True(t, member.MutedUntil.Equal(mutedUntil))
	}
	_, err = repo.SendFanGroupMessage(ctx, groupID, "fan-1", "hi", now.Add(2*time.Minute))
	require.ErrorIs(t, err, ErrFanGroupMuted)
}

// A badge level-up bumps fan_badges.updated_at; it must not move a manually
// kicked member into another group with a fresh, active membership.
func TestSyncFanGroupsKeepsKickedMemberOutAfterBadgeUpdate(t *testing.T) {
	repo, db := newFanGroupTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	creatorID := "creator-1"
	for i := 0; i < 200; i++ {
		insertFanBadge(t, db, fanID(i), creatorID, now.Add(time.Duration(i)*time.Second))
	}
	groups, err := repo.SyncFanGroups(ctx, creatorID, "Creator", now)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, []string{groups[0].ID}, activeFanGroups(t, db, fanID(5)))

	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, groups[0].ID, fanID(5), "", nil, false, true, false, false, now.Add(time.Minute)))
	require.NoError(t, db.Exec(`UPDATE fan_badges SET level = 3, updated_at = ? WHERE user_id = ?`, now.Add(time.Hour), fanID(5)).Error)

	_, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Empty(t, activeFanGroups(t, db, fanID(5)))
}

// When an earlier member leaves, later members shift groups. A manual kick
// and an active mute follow the member to the new group.
func TestSyncFanGroupsCarriesKickAndMuteAcrossGroups(t *testing.T) {
	repo, db := newFanGroupTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	creatorID := "creator-1"
	for i := 0; i < 201; i++ {
		insertFanBadge(t, db, fanID(i), creatorID, now.Add(time.Duration(i)*time.Second))
	}
	groups, err := repo.SyncFanGroups(ctx, creatorID, "Creator", now)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	group1, group2 := groups[0].ID, groups[1].ID
	kicked, muted := fanID(199), fanID(200)
	require.Equal(t, []string{group2}, activeFanGroups(t, db, kicked))
	mutedUntil := now.Add(3 * time.Hour)
	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, group2, kicked, "", nil, false, true, false, false, now.Add(time.Minute)))
	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, group2, muted, "", &mutedUntil, false, false, false, false, now.Add(time.Minute)))

	// Two earlier members leave the fan club: both shift into group 1.
	require.NoError(t, db.Exec(`DELETE FROM fan_badges WHERE user_id IN (?, ?)`, fanID(0), fanID(1)).Error)
	_, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, activeFanGroups(t, db, kicked))
	require.Equal(t, []string{group1}, activeFanGroups(t, db, muted))
	var member model.FanGroupMember
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", group1, muted).Take(&member).Error)
	require.NotNil(t, member.MutedUntil)
	require.True(t, member.MutedUntil.Equal(mutedUntil))

	// Approving the rejoin lifts the kick creator-wide; the next sync places
	// the member in exactly one group.
	require.NoError(t, repo.RequestFanGroupRejoin(ctx, group2, kicked, now.Add(2*time.Hour)))
	require.NoError(t, repo.UpdateFanGroupMember(ctx, creatorID, group2, kicked, "", nil, false, false, true, false, now.Add(2*time.Hour)))
	require.Equal(t, []string{group2}, activeFanGroups(t, db, kicked))
	_, err = repo.SyncFanGroups(ctx, creatorID, "Creator", now.Add(3*time.Hour))
	require.NoError(t, err)
	require.Equal(t, []string{group1}, activeFanGroups(t, db, kicked))
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
