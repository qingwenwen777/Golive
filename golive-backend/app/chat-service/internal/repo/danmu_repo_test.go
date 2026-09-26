package repo

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
)

// newTestDB returns a SQLite DB with the tables chat-service reads from
// other services (users, rooms, fan_badges, coin_transactions).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chat.db")), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, display_name TEXT, avatar TEXT)`,
		`CREATE TABLE rooms (id TEXT PRIMARY KEY, owner_id TEXT)`,
		`CREATE TABLE fan_badges (user_id TEXT, creator_id TEXT, total_contribution INTEGER)`,
		`CREATE TABLE coin_transactions (id TEXT, user_id TEXT, type TEXT, amount INTEGER)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	return db
}

func TestFanBadgesForRoomUsers_MatchesByUserIDOnly(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO rooms VALUES ('live-1', 'owner-1')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO users VALUES
		('fan', 'bigfan', 'Big Fan', ''),
		('copycat', 'copycat', 'Big Fan', '')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO fan_badges VALUES
		('fan', 'owner-1', 5000),
		('fan', 'other-owner', 90000)`).Error)

	r := NewDanmuRepo(db, 8)
	badges, err := r.FanBadgesForRoomUsers(context.Background(), "live-1", []string{"fan", "copycat", "fan"})
	require.NoError(t, err)
	require.Equal(t, map[string]*model.FanBadgePayload{"fan": {CreatorID: "owner-1", Level: 5}}, badges,
		"a user sharing the fan's display name must not get the badge")
}

func TestFanBadgesForRoomUsers_MissingTablesDegrade(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{})
	require.NoError(t, err)
	r := NewDanmuRepo(db, 8)
	badges, err := r.FanBadgesForRoomUsers(context.Background(), "live-1", []string{"u"})
	require.NoError(t, err)
	require.Empty(t, badges)
	profiles, err := r.UserProfiles(context.Background(), []string{"u"})
	require.NoError(t, err)
	require.Empty(t, profiles)
}

func TestUserProfiles(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO users VALUES
		('u1', 'luna', 'Luna', '/luna.png'),
		('u2', 'kabun', '', ''),
		('3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f', '3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f', '', '')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO coin_transactions VALUES
		('t1', 'u1', 'topup', 5000),
		('t2', 'u1', 'topup', 5000),
		('t3', 'u1', 'gift_spend', -3000)`).Error)

	r := NewDanmuRepo(db, 8)
	profiles, err := r.UserProfiles(context.Background(), []string{"u1", "u2", "3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f", "ghost"})
	require.NoError(t, err)
	require.Len(t, profiles, 3)
	require.Equal(t, "Luna", profiles["u1"].Name)
	require.Equal(t, "/luna.png", profiles["u1"].Avatar)
	require.Greater(t, profiles["u1"].Level, 1, "level comes from top-ups")
	require.Equal(t, "kabun", profiles["u2"].Name)
	require.Equal(t, 1, profiles["u2"].Level)
	require.Equal(t, "Creator 3f2a0c1e", profiles["3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f"].Name)
}

func TestAutoMigrateCreatesRoomTsIndexPerShard(t *testing.T) {
	db := newTestDB(t)
	r := NewDanmuRepo(db, 4)
	require.NoError(t, r.AutoMigrate())
	require.NoError(t, r.AutoMigrate(), "re-running the migration is a no-op")
	for i := 0; i < r.Shards(); i++ {
		table := fmt.Sprintf("danmus_%d", i)
		index := fmt.Sprintf("idx_%s_room_ts", table)
		require.True(t, db.Table(table).Migrator().HasIndex(&model.Danmu{}, index), index)
		var columns []string
		require.NoError(t, db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index).Scan(&columns).Error)
		require.Equal(t, []string{"room_id", "ts"}, columns, index)
	}
}
