package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

var ErrUserNotFound = errors.New("user not found")
var ErrApplicationNotFound = errors.New("creator application not found")
var ErrApplicationAlreadyReviewed = errors.New("creator application already reviewed")

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// AutoMigrate creates / updates the users table.
func (r *UserRepo) AutoMigrate() error {
	return r.db.AutoMigrate(&model.User{}, &model.CreatorApplication{}, &model.CoinTransaction{})
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
	return &u, nil
}

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepo) CreateAdmin(ctx context.Context, u *model.User) error {
	u.Role = model.RoleAdmin
	u.LivePermissionStatus = model.LivePermissionApproved
	u.Verified = true
	return r.Create(ctx, u)
}

func (r *UserRepo) EnsureAdmin(ctx context.Context, username string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("username = ?", username).
		Updates(map[string]any{
			"role":                   model.RoleAdmin,
			"live_permission_status": model.LivePermissionApproved,
			"verified":               true,
		}).Error
}

func (r *UserRepo) SubmitCreatorApplication(ctx context.Context, userID string) (*model.CreatorApplication, *model.User, bool, error) {
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
					Status: model.LivePermissionPending,
				}
				created = true
				return tx.Create(&app).Error
			}
			return err
		case model.LivePermissionNone, model.LivePermissionRejected:
			app = model.CreatorApplication{
				ID:     newID(),
				UserID: userID,
				Status: model.LivePermissionPending,
			}
			if err := tx.Create(&app).Error; err != nil {
				return err
			}
			created = true
			if err := tx.Model(&model.User{}).
				Where("id = ?", userID).
				Update("live_permission_status", model.LivePermissionPending).Error; err != nil {
				return err
			}
			user.LivePermissionStatus = model.LivePermissionPending
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
	return &app, &user, created, nil
}

type CreatorApplicationView struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName,omitempty"`
	Avatar      string     `json:"avatar"`
	Status      string     `json:"status"`
	ReviewerID  string     `json:"reviewerId,omitempty"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (r *UserRepo) ListCreatorApplications(ctx context.Context) ([]CreatorApplicationView, error) {
	var rows []CreatorApplicationView
	err := r.db.WithContext(ctx).
		Table("creator_applications AS ca").
		Select(`ca.id, ca.user_id, users.username, users.display_name, users.avatar,
			ca.status, ca.reviewer_id, ca.reviewed_at, ca.created_at, ca.updated_at`).
		Joins("JOIN users ON users.id = ca.user_id").
		Order("CASE ca.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 WHEN 'approved' THEN 2 ELSE 3 END, ca.created_at DESC").
		Scan(&rows).Error
	return rows, err
}

func (r *UserRepo) ReviewCreatorApplication(ctx context.Context, id, reviewerID, status string) (*model.CreatorApplication, *model.User, error) {
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
		if err := tx.Model(&app).Updates(map[string]any{
			"status":      status,
			"reviewer_id": reviewerID,
			"reviewed_at": &now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).
			Where("id = ?", app.UserID).
			Update("live_permission_status", status).Error; err != nil {
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
	return &app, &user, nil
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
	return &u, nil
}

func newID() string {
	return uuid.NewString()
}
