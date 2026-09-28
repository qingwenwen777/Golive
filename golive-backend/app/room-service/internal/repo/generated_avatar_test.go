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

func TestClearGeneratedAvatarsKeepsUploadedPhotos(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	updated := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	avatars := map[string]string{
		"cartoon":  "https://api.dicebear.com/7.x/avataaars/svg?seed=Luna&skinColor=ffdbb4",
		"initials": "https://api.dicebear.com/7.x/initials/svg?seed=Kai",
		"uploaded": "https://cdn.example.com/avatars/rin.webp",
		"none":     "",
	}
	for id, avatar := range avatars {
		require.NoError(t, db.Create(&model.Room{
			ID: id, Title: id, OwnerID: id, Status: model.StatusEnded, Avatar: avatar,
			CreatedAt: updated, UpdatedAt: updated,
		}).Error)
	}

	n, err := rooms.ClearGeneratedAvatars(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	want := map[string]string{
		"cartoon": "", "initials": "", "none": "",
		"uploaded": "https://cdn.example.com/avatars/rin.webp",
	}
	for id, avatar := range want {
		var room model.Room
		require.NoError(t, db.Where("id = ?", id).Take(&room).Error)
		require.Equal(t, avatar, room.Avatar, id)
		require.True(t, room.UpdatedAt.Equal(updated), "%s: updated_at changed to %s", id, room.UpdatedAt)
	}

	n, err = rooms.ClearGeneratedAvatars(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}
