package repo_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// TestAutoMigrateCreatesHotPathIndexes checks the composite indexes behind
// paginated reads: message threads, notifications, fan group chats and
// members, and a creator's rooms by status.
func TestAutoMigrateCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, repo.NewRoomRepo(db).AutoMigrate())
	require.NoError(t, repo.NewMessageRepo(db).AutoMigrate())
	require.NoError(t, repo.NewAppointmentRepo(db).AutoMigrate())

	want := map[string][]string{
		"idx_direct_messages_thread_created":   {"thread_id", "created_at"},
		"idx_notifications_user_created":       {"user_id", "created_at"},
		"idx_fan_group_messages_group_created": {"group_id", "created_at"},
		"idx_fan_group_members_group_kicked":   {"group_id", "kicked_at"},
		"idx_rooms_owner_status":               {"owner_id", "status"},
	}
	for index, columns := range want {
		var got []string
		require.NoError(t, db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index).Scan(&got).Error)
		require.Equal(t, columns, got, index)
	}
}
