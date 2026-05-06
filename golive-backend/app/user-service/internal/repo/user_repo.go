package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
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
var ErrInsufficientCoins = errors.New("insufficient available coins")
var ErrUnbanAppealNotFound = errors.New("unban appeal not found")

type UsernameCooldownError struct {
	AvailableAt time.Time
}

func (e *UsernameCooldownError) Error() string { return ErrUsernameCooldown.Error() }

type UserRepo struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) WithRedis(rdb *redis.Client) *UserRepo {
	r.rdb = rdb
	return r
}

// AutoMigrate creates / updates the users table.
func (r *UserRepo) AutoMigrate() error {
	if err := r.db.AutoMigrate(&model.User{}, &model.InviteCode{}, &model.CreatorApplication{}, &model.PlatformApplication{}, &model.CoinTransaction{}, &model.AdminAuditLog{}, &model.UserModerationState{}, &model.UnbanAppeal{}); err != nil {
		return err
	}
	return r.ensureSearchIndexes()
}

func (r *UserRepo) ensureSearchIndexes() error {
	if r.db == nil || r.db.Dialector.Name() != "mysql" {
		return nil
	}
	var count int64
	if err := r.db.Raw(`
SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'users'
  AND index_name = 'ft_users_search'
`).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.db.Exec("CREATE FULLTEXT INDEX ft_users_search ON users (username, display_name, id)").Error
}

func (r *UserRepo) CreateAdminAuditLog(ctx context.Context, log *model.AdminAuditLog) error {
	if log == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(log).Error
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

func (r *UserRepo) ResetPasswordByUsernameEmail(ctx context.Context, username, email, hash string) error {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("username = ? AND email = ?", username, email).
		Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

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
		if err := tx.Model(&u).Update("email", email).Error; err != nil {
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
		if err := r.updateCreatorAvatarReferences(ctx, tx, id, avatar); err != nil {
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

func (r *UserRepo) updateCreatorAvatarReferences(ctx context.Context, tx *gorm.DB, id, avatar string) error {
	avatar = strings.TrimSpace(avatar)
	if strings.TrimSpace(id) == "" {
		return nil
	}
	if err := tx.WithContext(ctx).
		Table("rooms").
		Where("owner_id = ?", id).
		Update("avatar", avatar).Error; err != nil && !isMissingRelation(err) {
		return err
	}
	if err := tx.WithContext(ctx).
		Table("fan_badges").
		Where("creator_id = ?", id).
		Update("creator_avatar", avatar).Error; err != nil && !isMissingRelation(err) {
		return err
	}
	return nil
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

type AdminUserListFilter struct {
	Query  string
	Role   string
	Status string
	Page   int
	Size   int
}

type AdminUserStats struct {
	Total          int64 `json:"total"`
	Active         int64 `json:"active"`
	Banned         int64 `json:"banned"`
	Admins         int64 `json:"admins"`
	Moderators     int64 `json:"moderators"`
	PendingAppeals int64 `json:"pendingAppeals"`
}

type AdminUserView struct {
	ID                               string    `json:"id"`
	Username                         string    `json:"username"`
	Email                            string    `json:"email,omitempty"`
	DisplayName                      string    `json:"displayName,omitempty"`
	Avatar                           string    `json:"avatar"`
	Cover                            string    `json:"cover,omitempty"`
	CoinBalance                      int64     `json:"coinBalance"`
	FrozenCoins                      int64     `json:"frozenCoins"`
	Banned                           bool      `json:"banned"`
	BanReason                        string    `json:"banReason,omitempty"`
	PendingAppeals                   int64     `json:"pendingAppeals"`
	Verified                         bool      `json:"verified"`
	Role                             string    `json:"role"`
	LivePermissionStatus             string    `json:"livePermissionStatus"`
	LivePermissionRejectReason       string    `json:"livePermissionRejectReason,omitempty"`
	PlatformVerificationStatus       string    `json:"platformVerificationStatus"`
	PlatformVerificationRejectReason string    `json:"platformVerificationRejectReason,omitempty"`
	CreatedAt                        time.Time `json:"createdAt"`
	UpdatedAt                        time.Time `json:"updatedAt"`
}

type AdminLiveRecord struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Viewers     int64      `json:"viewers"`
	PeakViewers int64      `json:"peakViewers"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}

type AdminReportRecord struct {
	ID               string     `json:"id"`
	TargetType       string     `json:"targetType"`
	TargetID         string     `json:"targetId"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"`
	ReporterID       string     `json:"reporterId"`
	ReporterName     string     `json:"reporterName"`
	TargetTitle      string     `json:"targetTitle,omitempty"`
	TargetText       string     `json:"targetText,omitempty"`
	ResolutionAction string     `json:"resolutionAction,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	ResolvedAt       *time.Time `json:"resolvedAt,omitempty"`
}

type AdminUnbanAppealRecord struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"`
	ReviewerID string     `json:"reviewerId,omitempty"`
	Reviewer   string     `json:"reviewer,omitempty"`
	ReviewNote string     `json:"reviewNote,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type AdminUserDetail struct {
	User             AdminUserView            `json:"user"`
	CoinTransactions []model.CoinTransaction  `json:"coinTransactions"`
	LiveRecords      []AdminLiveRecord        `json:"liveRecords"`
	ReportRecords    []AdminReportRecord      `json:"reportRecords"`
	AppealRecords    []AdminUnbanAppealRecord `json:"appealRecords"`
}

func normalizeAdminPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func (r *UserRepo) AdminListUsers(ctx context.Context, filter AdminUserListFilter) ([]AdminUserView, int64, AdminUserStats, error) {
	page, size := normalizeAdminPage(filter.Page, filter.Size)
	q := r.db.WithContext(ctx).Table("users")
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("LOWER(COALESCE(username, '')) LIKE ? OR LOWER(COALESCE(display_name, '')) LIKE ? OR LOWER(COALESCE(email, '')) LIKE ?", like, like, like)
	}
	role := strings.TrimSpace(filter.Role)
	if role != "" && role != "all" {
		q = q.Where("role = ?", role)
	}
	switch strings.TrimSpace(filter.Status) {
	case "active":
		q = q.Where("COALESCE(banned, false) = ?", false)
	case "banned":
		q = q.Where("COALESCE(banned, false) = ?", true)
	case "appeal_pending":
		q = q.Where("EXISTS (SELECT 1 FROM unban_appeals ua WHERE ua.user_id = users.id AND ua.status IN (?, ?))", model.UnbanAppealPending, model.UnbanAppealReviewing)
	case "frozen":
		q = q.Where("COALESCE(frozen_coins, 0) > 0")
	case "live_approved":
		q = q.Where("live_permission_status = ?", model.LivePermissionApproved)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, AdminUserStats{}, err
	}
	rows := make([]AdminUserView, 0)
	err := q.Select(adminUserSelectSQL()).
		Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, AdminUserStats{}, err
	}
	stats, err := r.AdminUserStats(ctx)
	if err != nil {
		return nil, 0, AdminUserStats{}, err
	}
	return rows, total, stats, nil
}

func (r *UserRepo) AdminUserStats(ctx context.Context) (AdminUserStats, error) {
	var stats AdminUserStats
	db := r.db.WithContext(ctx).Table("users")
	if err := db.Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	count := func(where string, args ...any) (int64, error) {
		var n int64
		err := r.db.WithContext(ctx).Table("users").Where(where, args...).Count(&n).Error
		return n, err
	}
	var err error
	if stats.Active, err = count("COALESCE(banned, false) = ?", false); err != nil {
		return stats, err
	}
	if stats.Banned, err = count("COALESCE(banned, false) = ?", true); err != nil {
		return stats, err
	}
	if stats.Admins, err = count("role = ?", model.RoleAdmin); err != nil {
		return stats, err
	}
	if stats.Moderators, err = count("role = ?", model.RoleModerator); err != nil {
		return stats, err
	}
	if stats.PendingAppeals, err = count("EXISTS (SELECT 1 FROM unban_appeals ua WHERE ua.user_id = users.id AND ua.status IN (?, ?))", model.UnbanAppealPending, model.UnbanAppealReviewing); err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *UserRepo) AdminUserDetail(ctx context.Context, userID string) (*AdminUserDetail, error) {
	var user AdminUserView
	err := r.db.WithContext(ctx).
		Table("users").
		Select(adminUserSelectSQL()).
		Where("id = ?", userID).
		Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	coinRows, err := r.ListCoinTransactions(ctx, userID, 30)
	if err != nil {
		return nil, err
	}
	liveRows, err := r.adminLiveRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	reportRows, err := r.adminReportRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	appealRows, err := r.adminUnbanAppealRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AdminUserDetail{
		User:             user,
		CoinTransactions: coinRows,
		LiveRecords:      liveRows,
		ReportRecords:    reportRows,
		AppealRecords:    appealRows,
	}, nil
}

func (r *UserRepo) AdminUpdateProfile(ctx context.Context, userID string, username, displayName *string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if username != nil {
			next := strings.TrimSpace(*username)
			if next != "" && next != u.Username {
				var existing model.User
				err := tx.Where("username = ? AND id <> ?", next, userID).Take(&existing).Error
				if err == nil {
					return ErrUsernameTaken
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				updates["username"] = next
			}
		}
		if displayName != nil {
			updates["display_name"] = strings.TrimSpace(*displayName)
		}
		if len(updates) > 0 {
			if err := tx.Model(&u).Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", userID).Take(&u).Error
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

func (r *UserRepo) AdminUpdateRole(ctx context.Context, userID, role string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		updates := map[string]any{"role": role}
		if role == model.RoleModerator {
			updates["verified"] = false
		}
		if err := tx.Model(&u).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", userID).Take(&u).Error
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

func (r *UserRepo) AdminSetUserBan(ctx context.Context, userID string, banned bool, reason string) (*model.User, error) {
	var u model.User
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		cleanReason := strings.TrimSpace(reason)
		updates := map[string]any{
			"banned":     banned,
			"ban_reason": "",
		}
		if banned {
			updates["ban_reason"] = cleanReason
		}
		if err := tx.Model(&u).Updates(updates).Error; err != nil {
			return err
		}
		stateUpdates := map[string]any{
			"banned":     banned,
			"ban_reason": "",
			"updated_at": now,
		}
		state := model.UserModerationState{
			UserID:    userID,
			Banned:    banned,
			UpdatedAt: now,
			CreatedAt: now,
		}
		if banned {
			state.BanReason = cleanReason
			stateUpdates["ban_reason"] = cleanReason
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(stateUpdates),
		}).Create(&state).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", userID).Take(&u).Error
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
	return &u, r.syncUserBanCache(ctx, userID, banned)
}

func (r *UserRepo) AdminAdjustCoins(ctx context.Context, userID, action string, amount int64, note, operatorID string) (*model.User, *model.CoinTransaction, error) {
	if amount <= 0 {
		return nil, nil, ErrInsufficientCoins
	}
	var u model.User
	var coinTx *model.CoinTransaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		txType := model.CoinTxAdminAdjust
		txAmount := int64(0)
		title := "Admin coin adjustment"
		switch action {
		case "add":
			txAmount = amount
			if err := tx.Model(&u).UpdateColumn("coin_balance", gorm.Expr("coin_balance + ?", amount)).Error; err != nil {
				return err
			}
		case "deduct":
			txAmount = -amount
			res := tx.Model(&model.User{}).
				Where("id = ? AND coin_balance >= ?", userID, amount).
				UpdateColumn("coin_balance", gorm.Expr("coin_balance - ?", amount))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrInsufficientCoins
			}
		case "freeze":
			txType = model.CoinTxAdminFreeze
			title = "Admin coin freeze"
			res := tx.Model(&model.User{}).
				Where("id = ? AND coin_balance - COALESCE(frozen_coins, 0) >= ?", userID, amount).
				UpdateColumn("frozen_coins", gorm.Expr("COALESCE(frozen_coins, 0) + ?", amount))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrInsufficientCoins
			}
		case "unfreeze":
			txType = model.CoinTxAdminUnfreeze
			title = "Admin coin unfreeze"
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				UpdateColumn("frozen_coins", gorm.Expr("CASE WHEN COALESCE(frozen_coins, 0) >= ? THEN COALESCE(frozen_coins, 0) - ? ELSE 0 END", amount, amount)).Error; err != nil {
				return err
			}
		default:
			return errors.New("invalid coin action")
		}
		if err := tx.Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		coinTx = &model.CoinTransaction{
			ID:             newID(),
			UserID:         userID,
			Type:           txType,
			Amount:         txAmount,
			BalanceAfter:   u.CoinBalance,
			Title:          title,
			Description:    strings.TrimSpace(note),
			SourceType:     "admin",
			SourceID:       operatorID,
			CounterpartyID: operatorID,
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

func (r *UserRepo) adminLiveRecords(ctx context.Context, userID string) ([]AdminLiveRecord, error) {
	rows := make([]AdminLiveRecord, 0)
	err := r.db.WithContext(ctx).
		Table("rooms").
		Select("id, title, status, viewers, peak_viewers, started_at, ended_at").
		Where("owner_id = ?", userID).
		Order("started_at DESC, created_at DESC").
		Limit(20).
		Scan(&rows).Error
	if isMissingRelation(err) {
		return rows, nil
	}
	return rows, err
}

func (r *UserRepo) adminReportRecords(ctx context.Context, userID string) ([]AdminReportRecord, error) {
	rows := make([]AdminReportRecord, 0)
	err := r.db.WithContext(ctx).
		Table("content_reports AS cr").
		Select(`cr.id, cr.target_type, cr.target_id, cr.reason, cr.status,
			cr.reporter_id, COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), cr.reporter_id) AS reporter_name,
			cr.target_title, cr.target_text, cr.resolution_action, cr.created_at, cr.resolved_at`).
		Joins("LEFT JOIN users AS u ON u.id = cr.reporter_id").
		Where("cr.reporter_id = ? OR cr.target_user_id = ? OR cr.target_owner_id = ?", userID, userID, userID).
		Order("cr.created_at DESC").
		Limit(30).
		Scan(&rows).Error
	if isMissingRelation(err) {
		return rows, nil
	}
	return rows, err
}

func (r *UserRepo) adminUnbanAppealRecords(ctx context.Context, userID string) ([]AdminUnbanAppealRecord, error) {
	rows := make([]AdminUnbanAppealRecord, 0)
	err := r.db.WithContext(ctx).
		Table("unban_appeals AS ua").
		Select(`ua.id, ua.user_id, ua.reason, ua.status, ua.reviewer_id,
			COALESCE(NULLIF(reviewer.display_name, ''), NULLIF(reviewer.username, ''), ua.reviewer_id) AS reviewer,
			ua.review_note, ua.reviewed_at, ua.created_at, ua.updated_at`).
		Joins("LEFT JOIN users AS reviewer ON reviewer.id = ua.reviewer_id").
		Where("ua.user_id = ?", userID).
		Order("CASE ua.status WHEN 'pending' THEN 0 WHEN 'reviewing' THEN 1 ELSE 2 END, ua.created_at DESC").
		Limit(30).
		Scan(&rows).Error
	if isMissingRelation(err) {
		return rows, nil
	}
	return rows, err
}

func (r *UserRepo) AdminReviewUnbanAppeal(ctx context.Context, userID, appealID, reviewerID, status, note string) (*model.UnbanAppeal, *model.User, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != model.UnbanAppealApproved && status != model.UnbanAppealRejected && status != model.UnbanAppealReviewing {
		status = model.UnbanAppealReviewing
	}
	var appeal model.UnbanAppeal
	var u model.User
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", strings.TrimSpace(appealID), strings.TrimSpace(userID)).Take(&appeal).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"status":      status,
			"reviewer_id": strings.TrimSpace(reviewerID),
			"review_note": strings.TrimSpace(note),
			"updated_at":  now,
		}
		if status == model.UnbanAppealApproved || status == model.UnbanAppealRejected {
			updates["reviewed_at"] = now
		} else {
			updates["reviewed_at"] = nil
		}
		if err := tx.Model(&model.UnbanAppeal{}).Where("id = ?", appeal.ID).Updates(updates).Error; err != nil {
			return err
		}
		if status == model.UnbanAppealApproved {
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Updates(map[string]any{"banned": false, "ban_reason": ""}).Error; err != nil {
				return err
			}
			state := model.UserModerationState{
				UserID:    userID,
				Banned:    false,
				UpdatedBy: strings.TrimSpace(reviewerID),
				UpdatedAt: now,
				CreatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"banned":      false,
					"ban_reason":  "",
					"muted_until": nil,
					"mute_reason": "",
					"updated_by":  strings.TrimSpace(reviewerID),
					"updated_at":  now,
				}),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", appeal.ID).Take(&appeal).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", userID).Take(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrUnbanAppealNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, err
	}
	if status == model.UnbanAppealApproved {
		if err := r.syncUserBanCache(ctx, userID, false); err != nil {
			return nil, nil, err
		}
	}
	return &appeal, &u, nil
}

func adminUserSelectSQL() string {
	return `id, username, email, display_name, avatar, cover, coin_balance, COALESCE(frozen_coins, 0) AS frozen_coins,
		COALESCE(banned, false) AS banned, COALESCE(ban_reason, '') AS ban_reason,
		(SELECT COUNT(1) FROM unban_appeals ua WHERE ua.user_id = users.id AND ua.status IN ('pending', 'reviewing')) AS pending_appeals,
		verified, role,
		live_permission_status, live_permission_reject_reason,
		platform_verification_status, platform_verification_reject_reason,
		created_at, updated_at`
}

func (r *UserRepo) syncUserBanCache(ctx context.Context, userID string, banned bool) error {
	if r.rdb == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	key := contentpolicy.RedisSiteBanPrefix + strings.TrimSpace(userID)
	if banned {
		return r.rdb.Set(ctx, key, "1", 0).Err()
	}
	return r.rdb.Del(ctx, key).Err()
}

func isMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "unknown column") ||
		strings.Contains(msg, "no such column")
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
