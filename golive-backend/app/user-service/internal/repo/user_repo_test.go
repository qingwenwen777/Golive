package repo

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

func TestUpdateAvatarRefreshesCreatorSnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE rooms (
	id varchar(64) primary key,
	owner_id varchar(36),
	avatar varchar(500)
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE fan_badges (
	user_id varchar(36),
	creator_id varchar(36),
	creator_avatar varchar(500),
	primary key (user_id, creator_id)
)`).Error)

	creator := model.User{
		ID:           "creator-1",
		Username:     "creator",
		DisplayName:  "Creator",
		PasswordHash: "hash",
		Avatar:       "/old.jpg",
		Role:         model.RoleUser,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&creator).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO rooms (id, owner_id, avatar) VALUES (?, ?, ?)`,
		"room-1", creator.ID, "/old.jpg",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO fan_badges (user_id, creator_id, creator_avatar) VALUES (?, ?, ?)`,
		"viewer-1", creator.ID, "/old.jpg",
	).Error)

	updated, err := users.UpdateAvatar(ctx, creator.ID, "/new.jpg")
	require.NoError(t, err)
	require.Equal(t, "/new.jpg", updated.Avatar)

	var roomAvatar string
	require.NoError(t, db.Raw(`SELECT avatar FROM rooms WHERE id = ?`, "room-1").Scan(&roomAvatar).Error)
	require.Equal(t, "/new.jpg", roomAvatar)

	var badgeAvatar string
	require.NoError(t, db.Raw(`SELECT creator_avatar FROM fan_badges WHERE creator_id = ?`, creator.ID).Scan(&badgeAvatar).Error)
	require.Equal(t, "/new.jpg", badgeAvatar)
}
