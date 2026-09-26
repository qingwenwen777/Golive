package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUsernameTaken = errors.New("username already exists")
var (
	// A new name must not pass for another user's; see checkNewNames.
	ErrUsernameIsDisplayName = errors.New("username is another user's display name")
	ErrDisplayNameTaken      = errors.New("display name is another user's username")
)
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

// Create inserts u, refusing names that pass for another user's (see
// checkNewNames).
func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkNewNames(tx, u.ID, u.Username, u.DisplayName); err != nil {
			return err
		}
		return tx.Create(u).Error
	})
}

// checkNewNames keeps one user's name from passing for another's. Channel
// links and follow-by-name resolve a name to the user with that username (or
// the only user with that display name), so a username being set must not be
// another user's display name (ErrUsernameIsDisplayName) and a display name
// being set must not be another user's username (ErrDisplayNameTaken), both
// ignoring case. Pass "" for a name that is not changing: stored names are
// never re-checked, so an older clash does not stop its owners from saving
// their profile, and nobody's username or login changes.
func checkNewNames(tx *gorm.DB, userID, username, displayName string) error {
	if username != "" {
		taken, err := otherUserHasName(tx, "display_name", userID, username)
		if err != nil {
			return err
		}
		if taken {
			return ErrUsernameIsDisplayName
		}
	}
	if displayName != "" {
		taken, err := otherUserHasName(tx, "username", userID, displayName)
		if err != nil {
			return err
		}
		if taken {
			return ErrDisplayNameTaken
		}
	}
	return nil
}

// otherUserHasName reports whether a user other than userID has name, ignoring
// case, in column (username or display_name).
func otherUserHasName(tx *gorm.DB, column, userID, name string) (bool, error) {
	var ids []string
	err := tx.Model(&model.User{}).
		Where("LOWER("+column+") = ? AND id <> ?", strings.ToLower(name), userID).
		Limit(1).
		Pluck("id", &ids).Error
	return len(ids) > 0, err
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
		newUsername, newDisplayName := "", ""
		if displayName != nil {
			next := strings.TrimSpace(*displayName)
			if next != u.DisplayName {
				newDisplayName = next
			}
			updates["display_name"] = next
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
				newUsername = next
				updates["username"] = next
				updates["username_updated_at"] = now
			}
		}
		if err := checkNewNames(tx, id, newUsername, newDisplayName); err != nil {
			return err
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
	// fan_badges.creator_avatar is gift-service's snapshot; gift-service
	// replaces it with the current users.avatar when listing badges, so
	// user-service does not write gift-service's table.
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

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "unique constraint failed")
}

func newID() string {
	return uuid.NewString()
}
