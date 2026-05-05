package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUsernameTaken = errors.New("username already exists")
var ErrEmailTaken = errors.New("email already exists")
var (
	ErrGoogleAlreadyLinked = errors.New("google account already linked")
	ErrGoogleNotLinked     = errors.New("google account not linked")
)
var ErrInviteNotFound = errors.New("invite code not found")
var ErrInviteUsed = errors.New("invite code already used")
var ErrUsernameCooldown = errors.New("username change cooldown")
var ErrApplicationNotFound = errors.New("creator application not found")
var ErrApplicationAlreadyReviewed = errors.New("creator application already reviewed")
var ErrPlatformApplicationNotFound = errors.New("platform application not found")
var ErrPlatformApplicationAlreadyReviewed = errors.New("platform application already reviewed")
var ErrLivePermissionRequired = errors.New("approved live permission is required")

type UsernameCooldownError struct {
	AvailableAt time.Time
}

func (e *UsernameCooldownError) Error() string { return ErrUsernameCooldown.Error() }

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// AutoMigrate creates / updates the users table.
func (r *UserRepo) AutoMigrate() error {
	return r.db.AutoMigrate(&model.User{}, &model.InviteCode{}, &model.CreatorApplication{}, &model.PlatformApplication{}, &model.CoinTransaction{})
}

func (r *UserRepo) hydrateUserLevel(ctx context.Context, u *model.User) error {
	if u == nil || strings.TrimSpace(u.ID) == "" {
		return nil
	}
	var total int64
	err := r.db.WithContext(ctx).
		Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND type = ? AND amount > 0", u.ID, model.CoinTxTopup).
		Row().
		Scan(&total)
	if err != nil {
		return err
	}
	u.TotalTopupCoins = total
	return nil
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).Take(&u).Error
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

func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&u).Error
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

func (r *UserRepo) FindByGoogleSub(ctx context.Context, sub string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("google_sub = ?", sub).Take(&u).Error
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

func (r *UserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&u).Error
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

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

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

		err = tx.Where("email = ?", u.Email).Take(&existing).Error
		if err == nil {
			return ErrEmailTaken
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
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

func (r *UserRepo) ResetPasswordByEmail(ctx context.Context, email, hash string) error {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("email = ?", email).
		Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
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
		if googleEmail != "" && googleEmail != u.Email {
			err := tx.Where("email = ? AND id <> ?", googleEmail, id).Take(&existing).Error
			if err == nil {
				return ErrEmailTaken
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			updates["email"] = googleEmail
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

func (r *UserRepo) BackfillMissingEmails(ctx context.Context) error {
	var users []model.User
	if err := r.db.WithContext(ctx).
		Where("email IS NULL OR email = ?", "").
		Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		email := strings.ToLower(strings.TrimSpace(u.Username)) + "@gmail.com"
		if err := r.db.WithContext(ctx).Model(&model.User{}).
			Where("id = ?", u.ID).
			Update("email", email).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *UserRepo) ReconcilePlatformVerification(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("1 = 1").Update(
		"verified",
		gorm.Expr(
			"CASE WHEN platform_verification_status = ? THEN TRUE ELSE FALSE END",
			model.PlatformVerificationApproved,
		),
	).Error
}

func (r *UserRepo) UpdateProfile(
	ctx context.Context,
	id string,
	username *string,
	displayName *string,
	now time.Time,
	cooldown time.Duration,
) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}

		updates := map[string]any{}
		if displayName != nil {
			updates["display_name"] = strings.TrimSpace(*displayName)
		}
		if username != nil {
			next := strings.TrimSpace(*username)
			if next != "" && next != u.Username {
				if u.UsernameUpdatedAt != nil {
					availableAt := u.UsernameUpdatedAt.Add(cooldown)
					if now.Before(availableAt) {
						return &UsernameCooldownError{AvailableAt: availableAt}
					}
				}
				var existing model.User
				err := tx.Where("username = ? AND id <> ?", next, id).Take(&existing).Error
				if err == nil {
					return ErrUsernameTaken
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				updates["username"] = next
				updates["username_updated_at"] = now
			}
		}
		if len(updates) > 0 {
			if err := tx.Model(&u).Updates(updates).Error; err != nil {
				return err
			}
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

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("password_hash", hash).Error
}

func (r *UserRepo) CreateAdmin(ctx context.Context, u *model.User) error {
	u.Role = model.RoleAdmin
	u.LivePermissionStatus = model.LivePermissionApproved
	return r.Create(ctx, u)
}

func (r *UserRepo) CreateInviteCode(ctx context.Context, createdBy string) (*model.InviteCode, error) {
	for i := 0; i < 8; i++ {
		code, err := randomInviteCode(10)
		if err != nil {
			return nil, err
		}
		var existing model.InviteCode
		err = r.db.WithContext(ctx).Where("code = ?", code).Take(&existing).Error
		if err == nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		invite := &model.InviteCode{
			ID:        newID(),
			Code:      code,
			CreatedBy: createdBy,
		}
		if err := r.db.WithContext(ctx).Create(invite).Error; err != nil {
			return nil, err
		}
		return invite, nil
	}
	return nil, errors.New("could not generate unique invite code")
}

type InviteCodeView struct {
	ID              string     `json:"id"`
	Code            string     `json:"code"`
	CreatedBy       string     `json:"createdBy"`
	Used            bool       `json:"used"`
	UsedBy          string     `json:"usedBy,omitempty"`
	UsedUsername    string     `json:"usedUsername,omitempty"`
	UsedDisplayName string     `json:"usedDisplayName,omitempty"`
	UsedEmail       string     `json:"usedEmail,omitempty"`
	UsedAt          *time.Time `json:"usedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

func (r *UserRepo) ListInviteCodes(ctx context.Context) ([]InviteCodeView, error) {
	rows := make([]InviteCodeView, 0)
	err := r.db.WithContext(ctx).
		Table("invite_codes AS ic").
		Select(`ic.id, ic.code, ic.created_by, ic.used_by,
			users.username AS used_username, users.display_name AS used_display_name, users.email AS used_email,
			ic.used_at, ic.created_at`).
		Joins("LEFT JOIN users ON users.id = ic.used_by").
		Order("ic.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Used = rows[i].UsedAt != nil || rows[i].UsedBy != ""
	}
	return rows, nil
}

func (r *UserRepo) DeleteUnusedInviteCode(ctx context.Context, id string) (*model.InviteCode, error) {
	var invite model.InviteCode
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&invite).Error; err != nil {
			return err
		}
		if invite.UsedBy != "" || invite.UsedAt != nil {
			return ErrInviteUsed
		}
		return tx.Delete(&invite).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

func (r *UserRepo) EnsureAdmin(ctx context.Context, username string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("username = ?", username).
		Updates(map[string]any{
			"role":                   model.RoleAdmin,
			"live_permission_status": model.LivePermissionApproved,
		}).Error
}

func (r *UserRepo) SubmitCreatorApplication(ctx context.Context, userID, reason string) (*model.CreatorApplication, *model.User, bool, error) {
	var app model.CreatorApplication
	var user model.User
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&user).Error; err != nil {
			return err
		}
		if user.LivePermissionStatus == "" {
			user.LivePermissionStatus = model.LivePermissionNone
		}

		switch user.LivePermissionStatus {
		case model.LivePermissionApproved:
			return nil
		case model.LivePermissionPending:
			err := tx.Where("user_id = ? AND status = ?", userID, model.LivePermissionPending).
				Order("created_at DESC").
				Take(&app).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				app = model.CreatorApplication{
					ID:     newID(),
					UserID: userID,
					Reason: reason,
					Status: model.LivePermissionPending,
				}
				created = true
				if err := tx.Create(&app).Error; err != nil {
					return err
				}
				return nil
			}
			return err
		case model.LivePermissionNone, model.LivePermissionRejected:
			app = model.CreatorApplication{
				ID:     newID(),
				UserID: userID,
				Reason: reason,
				Status: model.LivePermissionPending,
			}
			if err := tx.Create(&app).Error; err != nil {
				return err
			}
			created = true
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Updates(map[string]any{
					"live_permission_status":        model.LivePermissionPending,
					"live_permission_reject_reason": "",
				}).Error; err != nil {
				return err
			}
			user.LivePermissionStatus = model.LivePermissionPending
			user.LivePermissionRejectReason = ""
			return nil
		default:
			return errors.New("invalid live permission status")
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.hydrateUserLevel(ctx, &user); err != nil {
		return nil, nil, false, err
	}
	return &app, &user, created, nil
}

type CreatorApplicationView struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"displayName,omitempty"`
	Avatar       string     `json:"avatar"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	ReviewerID   string     `json:"reviewerId,omitempty"`
	RejectReason string     `json:"rejectReason,omitempty"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (r *UserRepo) ListCreatorApplications(ctx context.Context) ([]CreatorApplicationView, error) {
	rows := make([]CreatorApplicationView, 0)
	err := r.db.WithContext(ctx).
		Table("creator_applications AS ca").
		Select(`ca.id, ca.user_id, users.username, users.display_name, users.avatar,
			ca.reason, ca.status, ca.reviewer_id, ca.reject_reason, ca.reviewed_at, ca.created_at, ca.updated_at`).
		Joins("JOIN users ON users.id = ca.user_id").
		Order("CASE ca.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 WHEN 'approved' THEN 2 ELSE 3 END, ca.created_at DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *UserRepo) ReviewCreatorApplication(ctx context.Context, id, reviewerID, status, rejectReason string) (*model.CreatorApplication, *model.User, error) {
	if status != model.LivePermissionApproved && status != model.LivePermissionRejected {
		return nil, nil, errors.New("invalid review status")
	}

	var app model.CreatorApplication
	var user model.User
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&app).Error; err != nil {
			return err
		}
		if app.Status != model.LivePermissionPending {
			return ErrApplicationAlreadyReviewed
		}
		updates := map[string]any{
			"status":      status,
			"reviewer_id": reviewerID,
			"reviewed_at": &now,
		}
		if status == model.LivePermissionRejected {
			updates["reject_reason"] = rejectReason
		} else {
			updates["reject_reason"] = ""
		}
		if err := tx.Model(&app).Updates(updates).Error; err != nil {
			return err
		}
		userUpdates := map[string]any{
			"live_permission_status": status,
		}
		if status == model.LivePermissionRejected {
			userUpdates["live_permission_reject_reason"] = rejectReason
			userUpdates["platform_verification_status"] = model.PlatformVerificationRejected
			userUpdates["platform_verification_reject_reason"] = rejectReason
			userUpdates["verified"] = false
		} else {
			userUpdates["live_permission_reject_reason"] = ""
		}
		if err := tx.Model(&model.User{}).
			Where("id = ?", app.UserID).
			Updates(userUpdates).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", app.ID).Take(&app).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", app.UserID).Take(&user).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrApplicationNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUserLevel(ctx, &user); err != nil {
		return nil, nil, err
	}
	return &app, &user, nil
}

func (r *UserRepo) SubmitPlatformApplication(ctx context.Context, userID, reason string) (*model.PlatformApplication, *model.User, bool, error) {
	var app model.PlatformApplication
	var user model.User
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&user).Error; err != nil {
			return err
		}
		if user.LivePermissionStatus != model.LivePermissionApproved {
			return ErrLivePermissionRequired
		}
		if user.PlatformVerificationStatus == "" {
			user.PlatformVerificationStatus = model.PlatformVerificationNone
		}

		switch user.PlatformVerificationStatus {
		case model.PlatformVerificationApproved:
			return nil
		case model.PlatformVerificationPending:
			err := tx.Where("user_id = ? AND status = ?", userID, model.PlatformVerificationPending).
				Order("created_at DESC").
				Take(&app).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				app = model.PlatformApplication{
					ID:     newID(),
					UserID: userID,
					Reason: reason,
					Status: model.PlatformVerificationPending,
				}
				created = true
				return tx.Create(&app).Error
			}
			return err
		case model.PlatformVerificationNone, model.PlatformVerificationRejected:
			app = model.PlatformApplication{
				ID:     newID(),
				UserID: userID,
				Reason: reason,
				Status: model.PlatformVerificationPending,
			}
			if err := tx.Create(&app).Error; err != nil {
				return err
			}
			created = true
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Updates(map[string]any{
					"platform_verification_status":        model.PlatformVerificationPending,
					"platform_verification_reject_reason": "",
					"verified":                            false,
				}).Error; err != nil {
				return err
			}
			user.PlatformVerificationStatus = model.PlatformVerificationPending
			user.PlatformVerificationRejectReason = ""
			user.Verified = false
			return nil
		default:
			return errors.New("invalid platform verification status")
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.hydrateUserLevel(ctx, &user); err != nil {
		return nil, nil, false, err
	}
	return &app, &user, created, nil
}

type PlatformApplicationView struct {
	ID                   string     `json:"id"`
	UserID               string     `json:"userId"`
	Username             string     `json:"username"`
	DisplayName          string     `json:"displayName,omitempty"`
	Avatar               string     `json:"avatar"`
	LivePermissionStatus string     `json:"livePermissionStatus"`
	Reason               string     `json:"reason"`
	Status               string     `json:"status"`
	ReviewerID           string     `json:"reviewerId,omitempty"`
	RejectReason         string     `json:"rejectReason,omitempty"`
	ReviewedAt           *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

func (r *UserRepo) ListPlatformApplications(ctx context.Context) ([]PlatformApplicationView, error) {
	rows := make([]PlatformApplicationView, 0)
	err := r.db.WithContext(ctx).
		Table("platform_applications AS pa").
		Select(`pa.id, pa.user_id, users.username, users.display_name, users.avatar,
			users.live_permission_status,
			pa.reason, pa.status, pa.reviewer_id, pa.reject_reason, pa.reviewed_at, pa.created_at, pa.updated_at`).
		Joins("JOIN users ON users.id = pa.user_id").
		Order("CASE pa.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 WHEN 'approved' THEN 2 ELSE 3 END, pa.created_at DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *UserRepo) ReviewPlatformApplication(ctx context.Context, id, reviewerID, status, rejectReason string) (*model.PlatformApplication, *model.User, error) {
	if status != model.PlatformVerificationApproved && status != model.PlatformVerificationRejected {
		return nil, nil, errors.New("invalid review status")
	}

	var app model.PlatformApplication
	var user model.User
	now := time.Now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&app).Error; err != nil {
			return err
		}
		if app.Status != model.PlatformVerificationPending {
			return ErrPlatformApplicationAlreadyReviewed
		}
		if err := tx.Where("id = ?", app.UserID).Take(&user).Error; err != nil {
			return err
		}
		if user.LivePermissionStatus != model.LivePermissionApproved {
			return ErrLivePermissionRequired
		}

		updates := map[string]any{
			"status":      status,
			"reviewer_id": reviewerID,
			"reviewed_at": &now,
		}
		if status == model.PlatformVerificationRejected {
			updates["reject_reason"] = rejectReason
		} else {
			updates["reject_reason"] = ""
		}
		if err := tx.Model(&app).Updates(updates).Error; err != nil {
			return err
		}

		userUpdates := map[string]any{
			"platform_verification_status": status,
			"verified":                     status == model.PlatformVerificationApproved,
		}
		if status == model.PlatformVerificationRejected {
			userUpdates["platform_verification_reject_reason"] = rejectReason
		} else {
			userUpdates["platform_verification_reject_reason"] = ""
		}
		if err := tx.Model(&model.User{}).
			Where("id = ?", app.UserID).
			Updates(userUpdates).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", app.ID).Take(&app).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", app.UserID).Take(&user).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrPlatformApplicationNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUserLevel(ctx, &user); err != nil {
		return nil, nil, err
	}
	return &app, &user, nil
}

type LiveCreatorView struct {
	ID                   string    `json:"id"`
	Username             string    `json:"username"`
	DisplayName          string    `json:"displayName,omitempty"`
	Avatar               string    `json:"avatar"`
	LivePermissionStatus string    `json:"livePermissionStatus"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func (r *UserRepo) ListLiveCreators(ctx context.Context) ([]LiveCreatorView, error) {
	rows := make([]LiveCreatorView, 0)
	err := r.db.WithContext(ctx).
		Table("users").
		Select("id, username, display_name, avatar, live_permission_status, updated_at").
		Where("live_permission_status = ?", model.LivePermissionApproved).
		Order("updated_at DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *UserRepo) SetLivePermissionStatus(ctx context.Context, userID, status string) (*model.User, error) {
	if status != model.LivePermissionApproved && status != model.LivePermissionRejected {
		return nil, errors.New("invalid live permission status")
	}
	var user model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&user).Error; err != nil {
			return err
		}
		rejectReason := ""
		if status == model.LivePermissionRejected {
			rejectReason = "Live permission disabled by administrator."
		}
		if err := tx.Model(&user).Updates(map[string]any{
			"live_permission_status":        status,
			"live_permission_reject_reason": rejectReason,
		}).Error; err != nil {
			return err
		}
		if status == model.LivePermissionRejected {
			now := time.Now()
			if err := tx.Model(&model.PlatformApplication{}).
				Where("user_id = ? AND status = ?", userID, model.PlatformVerificationPending).
				Updates(map[string]any{
					"status":        model.PlatformVerificationRejected,
					"reject_reason": rejectReason,
					"reviewed_at":   &now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Model(&user).Updates(map[string]any{
				"platform_verification_status":        model.PlatformVerificationRejected,
				"platform_verification_reject_reason": rejectReason,
				"verified":                            false,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", userID).Take(&user).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.hydrateUserLevel(ctx, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) IncrementCoins(ctx context.Context, id string, delta int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		if err := tx.Model(&u).UpdateColumn("coin_balance", gorm.Expr("coin_balance + ?", delta)).Error; err != nil {
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

func (r *UserRepo) IncrementCoinsWithTransaction(
	ctx context.Context,
	id string,
	delta int64,
	txType string,
	title string,
	description string,
	sourceType string,
	sourceID string,
	roomID string,
	counterpartyID string,
) (*model.User, *model.CoinTransaction, error) {
	var u model.User
	var coinTx *model.CoinTransaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		if err := tx.Model(&u).UpdateColumn("coin_balance", gorm.Expr("coin_balance + ?", delta)).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		coinTx = &model.CoinTransaction{
			ID:             newID(),
			UserID:         id,
			Type:           txType,
			Amount:         delta,
			BalanceAfter:   u.CoinBalance,
			Title:          title,
			Description:    description,
			SourceType:     sourceType,
			SourceID:       sourceID,
			RoomID:         roomID,
			CounterpartyID: counterpartyID,
		}
		return tx.Create(coinTx).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, err
	}
	return &u, coinTx, nil
}

func (r *UserRepo) ClaimDailyCoinReward(
	ctx context.Context,
	id string,
	taskSourceID string,
	title string,
	description string,
	reward int64,
) (*model.User, *model.CoinTransaction, bool, error) {
	var u model.User
	var coinTx model.CoinTransaction
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		err := tx.Where("user_id = ? AND type = ? AND source_id = ?", id, model.CoinTxDailyTask, taskSourceID).
			Take(&coinTx).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&u).UpdateColumn("coin_balance", gorm.Expr("coin_balance + ?", reward)).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		coinTx = model.CoinTransaction{
			ID:           newID(),
			UserID:       id,
			Type:         model.CoinTxDailyTask,
			Amount:       reward,
			BalanceAfter: u.CoinBalance,
			Title:        title,
			Description:  description,
			SourceType:   "daily_task",
			SourceID:     taskSourceID,
		}
		if err := tx.Create(&coinTx).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, false, err
	}
	return &u, &coinTx, created, nil
}

func (r *UserRepo) ListCoinTransactions(ctx context.Context, id string, limit int) ([]model.CoinTransaction, error) {
	if limit <= 0 {
		limit = 80
	}
	if limit > 200 {
		limit = 200
	}
	var rows []model.CoinTransaction
	err := r.db.WithContext(ctx).
		Where("user_id = ?", id).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *UserRepo) UpdateAvatar(ctx context.Context, id, avatar string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		if err := tx.Model(&u).Update("avatar", avatar).Error; err != nil {
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

func (r *UserRepo) UpdateCover(ctx context.Context, id, cover string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		if err := tx.Model(&u).Update("cover", cover).Error; err != nil {
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

func newID() string {
	return uuid.NewString()
}

func randomInviteCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}
