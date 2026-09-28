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

func TestClearGeneratedAvatarsKeepsUploadedPhotos(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())

	updated := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	avatars := map[string]string{
		"cartoon":  "https://api.dicebear.com/7.x/avataaars/svg?seed=Luna&skinColor=ffdbb4",
		"initials": "https://api.dicebear.com/7.x/initials/svg?seed=Kai",
		"http":     "http://api.dicebear.com/7.x/avataaars/svg?seed=Mio",
		"uploaded": "https://cdn.example.com/avatars/rin.webp",
		"none":     "",
	}
	for id, avatar := range avatars {
		require.NoError(t, db.Create(&model.User{
			ID: id, Username: id, PasswordHash: "hash", Avatar: avatar,
			CreatedAt: updated, UpdatedAt: updated,
		}).Error)
	}

	n, err := users.ClearGeneratedAvatars(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)

	want := map[string]string{
		"cartoon": "", "initials": "", "http": "", "none": "",
		"uploaded": "https://cdn.example.com/avatars/rin.webp",
	}
	for id, avatar := range want {
		var u model.User
		require.NoError(t, db.Where("id = ?", id).Take(&u).Error)
		require.Equal(t, avatar, u.Avatar, id)
		require.True(t, u.UpdatedAt.Equal(updated), "%s: updated_at changed to %s", id, u.UpdatedAt)
	}

	n, err = users.ClearGeneratedAvatars(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
}
