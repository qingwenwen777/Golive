package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *UserRepo) RegisterWithInvite(ctx context.Context, u *model.User, inviteCode string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.User
		err := tx.Where("username = ?", u.Username).Take(&existing).Error
		if err == nil {
			return ErrUsernameTaken
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := checkNewNames(tx, u.ID, u.Username, u.DisplayName); err != nil {
			return err
		}

		if u.Email != nil {
			err = tx.Where("email = ?", *u.Email).Take(&existing).Error
			if err == nil {
				return ErrEmailTaken
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		var invite model.InviteCode
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code = ?", strings.ToUpper(strings.TrimSpace(inviteCode))).
			Take(&invite).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInviteNotFound
		}
		if err != nil {
			return err
		}
		if invite.UsedAt != nil || invite.UsedBy != "" {
			return ErrInviteUsed
		}

		if err := tx.Create(u).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		return tx.Model(&invite).Updates(map[string]any{
			"used_by": u.ID,
			"used_at": &now,
		}).Error
	})
}

// ResetPasswordByEmail and ResetPasswordByUsernameEmail only match verified
// emails, so an unproven address can never be used to take over an account.
func (r *UserRepo) ResetPasswordByEmail(ctx context.Context, email, hash string) error {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("email = ? AND email_verified = ?", email, true).
		Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepo) ResetPasswordByUsernameEmail(ctx context.Context, username, email, hash string) error {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("username = ? AND email = ? AND email_verified = ?", username, email, true).
		Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateEmail sets an address nobody has proven ownership of yet, so it is
// stored unverified and cannot be used for password reset.
func (r *UserRepo) UpdateEmail(ctx context.Context, id, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&u).Error; err != nil {
			return err
		}
		var existing model.User
		err := tx.Where("email = ? AND id <> ?", email, id).Take(&existing).Error
		if err == nil {
			return ErrEmailTaken
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&u).Updates(map[string]any{
			"email":          email,
			"email_verified": false,
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) LinkGoogleAccount(ctx context.Context, id, googleSub, googleEmail string, linkedAt time.Time) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&u).Error; err != nil {
			return err
		}

		var existing model.User
		err := tx.Where("google_sub = ? AND id <> ?", googleSub, id).Take(&existing).Error
		if err == nil {
			return ErrGoogleAlreadyLinked
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		updates := map[string]any{
			"google_sub":       googleSub,
			"google_linked_at": linkedAt,
		}
		if googleEmail != "" {
			if googleEmail != u.EmailAddress() {
				err := tx.Where("email = ? AND id <> ?", googleEmail, id).Take(&existing).Error
				if err == nil {
					return ErrEmailTaken
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				updates["email"] = googleEmail
			}
			// Google only issues credentials with email_verified set.
			updates["email_verified"] = true
		}

		if err := tx.Model(&u).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) UnlinkGoogleAccount(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&u).Error; err != nil {
			return err
		}
		if u.GoogleSub == nil || strings.TrimSpace(*u.GoogleSub) == "" {
			return ErrGoogleNotLinked
		}
		if err := tx.Model(&u).Updates(map[string]any{
			"google_sub":       nil,
			"google_linked_at": nil,
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// MigrateEmailVerification classifies users whose email_verified is NULL,
// i.e. rows created before the column existed. Earlier versions stored an
// invented lower(username)+"@gmail.com" address for local signups, seeded and
// admin-created accounts (and backfilled it for rows without an email), which
// let whoever owns that mailbox reset the password.
//
//   - Accounts registered with an invite (email code or Google) or linked to
//     Google proved their address: verified.
//   - Placeholder or empty addresses are cleared to NULL: unverified.
//   - Anything else (e.g. set by an admin) is kept but unverified.
//
// It only touches NULL rows, so it is safe to run on every startup.
func (r *UserRepo) MigrateEmailVerification(ctx context.Context) error {
	var rows []struct {
		ID        string
		Username  string
		Email     *string
		GoogleSub *string
		Invited   bool
	}
	if err := r.db.WithContext(ctx).
		Table("users").
		Select(`users.id, users.username, users.email, users.google_sub,
			EXISTS (SELECT 1 FROM invite_codes ic WHERE ic.used_by = users.id) AS invited`).
		Where("users.email_verified IS NULL").
		Scan(&rows).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			email := ""
			if row.Email != nil {
				email = strings.ToLower(strings.TrimSpace(*row.Email))
			}
			placeholder := strings.ToLower(strings.TrimSpace(row.Username)) + "@gmail.com"
			updates := map[string]any{"email_verified": false}
			switch {
			case email == "":
				updates["email"] = nil
			case row.Invited || (row.GoogleSub != nil && strings.TrimSpace(*row.GoogleSub) != ""):
				updates["email_verified"] = true
			case email == placeholder:
				updates["email"] = nil
			}
			if err := tx.Model(&model.User{}).
				Where("id = ? AND email_verified IS NULL", row.ID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *UserRepo) CreateAdmin(ctx context.Context, u *model.User) error {
	u.Role = model.RoleAdmin
	u.LivePermissionStatus = model.LivePermissionApproved
	return r.Create(ctx, u)
}

func (r *UserRepo) EnsureAdmin(ctx context.Context, username string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("username = ?", username).
		Updates(map[string]any{
			"role":                   model.RoleAdmin,
			"live_permission_status": model.LivePermissionApproved,
		}).Error
}
