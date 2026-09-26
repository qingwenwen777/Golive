package repo

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateEmailVerificationClearsPlaceholdersAndKeepsProvenEmails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testMigrateEmailVerification(t, db)
}

func TestMigrateEmailVerificationMySQL(t *testing.T) {
	testMigrateEmailVerification(t, openTestMySQL(t))
}

func testMigrateEmailVerification(t *testing.T, db *gorm.DB) {
	ctx := context.Background()
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())

	// Recreate a table from before email_verified existed, then upgrade it.
	require.NoError(t, db.Exec(`ALTER TABLE users DROP COLUMN email_verified`).Error)
	insert := func(id, username string, email, googleSub any) {
		require.NoError(t, db.Exec(`INSERT INTO users
			(id, username, email, google_sub, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'hash', ?, ?)`,
			id, username, email, googleSub, time.Now(), time.Now()).Error)
	}
	insert("admin", "Admin", "admin@gmail.com", nil)           // seeded placeholder
	insert("invited", "bob", "bob@gmail.com", nil)             // email-code signup that happens to match
	insert("google", "carol", "carol@gmail.com", "google-sub") // Google-linked
	insert("manual", "dave", "dave@company.example", nil)      // set by an admin
	insert("empty", "erin", "", nil)
	require.NoError(t, db.Exec(`INSERT INTO invite_codes (id, code, created_by, used_by, created_at, updated_at)
		VALUES ('inv-1', 'CODE1', 'admin', 'invited', ?, ?)`, time.Now(), time.Now()).Error)
	require.NoError(t, users.AutoMigrate())
	// A row written after the upgrade is already classified and left alone.
	require.NoError(t, db.Exec(`INSERT INTO users
		(id, username, email, email_verified, password_hash, created_at, updated_at)
		VALUES ('current', 'frank', 'frank@gmail.com', ?, 'hash', ?, ?)`,
		false, time.Now(), time.Now()).Error)

	require.NoError(t, users.MigrateEmailVerification(ctx))
	require.NoError(t, users.MigrateEmailVerification(ctx))

	expect := map[string]struct {
		email    string
		verified bool
	}{
		"admin":   {"", false},
		"invited": {"bob@gmail.com", true},
		"google":  {"carol@gmail.com", true},
		"manual":  {"dave@company.example", false},
		"empty":   {"", false},
		"current": {"frank@gmail.com", false},
	}
	for id, want := range expect {
		u, err := users.FindByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want.email, u.EmailAddress(), id)
		if want.email == "" {
			require.Nil(t, u.Email, id)
		}
		require.Equal(t, want.verified, u.EmailVerified, id)
	}
	var unclassified int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM users WHERE email_verified IS NULL`).Scan(&unclassified).Error)
	require.Zero(t, unclassified)

	require.ErrorIs(t, users.ResetPasswordByUsernameEmail(ctx, "Admin", "admin@gmail.com", "x"), ErrUserNotFound)
	require.ErrorIs(t, users.ResetPasswordByUsernameEmail(ctx, "dave", "dave@company.example", "x"), ErrUserNotFound)
	require.NoError(t, users.ResetPasswordByUsernameEmail(ctx, "bob", "bob@gmail.com", "x"))
}
