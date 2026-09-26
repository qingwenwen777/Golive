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

func TestNewNamesCannotPassForAnotherUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testNewNamesCannotPassForAnotherUser(t, db)
}

func TestNewNamesCannotPassForAnotherUserMySQL(t *testing.T) {
	testNewNamesCannotPassForAnotherUser(t, openTestMySQL(t))
}

func testNewNamesCannotPassForAnotherUser(t *testing.T, db *gorm.DB) {
	ctx := context.Background()
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	create := func(id, username, displayName string) error {
		return users.Create(ctx, &model.User{ID: id, Username: username, DisplayName: displayName, PasswordHash: "hash", Role: model.RoleUser})
	}
	name := func(s string) *string { return &s }
	now := time.Now()

	require.NoError(t, create("creator", "kabun", "Kabun Live"))
	require.NoError(t, create("solo", "solo_streams", "Solo"))
	require.NoError(t, create("fan", "fan", "Fan"))

	require.ErrorIs(t, create("copy-1", "copycat", "KaBun"), ErrDisplayNameTaken)
	require.ErrorIs(t, create("copy-2", "SOLO", "Copy"), ErrUsernameIsDisplayName)
	_, err := users.UpdateProfile(ctx, "fan", nil, name("KABUN"), now, time.Hour)
	require.ErrorIs(t, err, ErrDisplayNameTaken)
	_, err = users.UpdateProfile(ctx, "fan", name("solo"), name("Fan"), now, time.Hour)
	require.ErrorIs(t, err, ErrUsernameIsDisplayName)
	_, err = users.AdminUpdateProfile(ctx, "fan", nil, name("Kabun"))
	require.ErrorIs(t, err, ErrDisplayNameTaken)

	// A user's own username is fine as display name, in any case.
	u, err := users.UpdateProfile(ctx, "creator", nil, name("KABUN"), now, time.Hour)
	require.NoError(t, err)
	require.Equal(t, "KABUN", u.DisplayName)
	fan, err := users.FindByID(ctx, "fan")
	require.NoError(t, err)
	require.Equal(t, "fan", fan.Username)
	require.Equal(t, "Fan", fan.DisplayName)
}
