package repo

import (
	"context"
	"errors"
	"time"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"gorm.io/gorm"
)

func (r *UserRepo) ReconcilePlatformVerification(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("1 = 1").Update(
		"verified",
		gorm.Expr(
			"CASE WHEN platform_verification_status = ? THEN TRUE ELSE FALSE END",
			model.PlatformVerificationApproved,
		),
	).Error
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

type AdminStatusStats struct {
	Pending  int64 `json:"pending"`
	Approved int64 `json:"approved"`
	Rejected int64 `json:"rejected"`
	Total    int64 `json:"total"`
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

func (r *UserRepo) ListCreatorApplicationsPage(ctx context.Context, page, size int) ([]CreatorApplicationView, int64, AdminStatusStats, error) {
	page, size = normalizeAdminPage(page, size)
	base := r.db.WithContext(ctx).
		Table("creator_applications AS ca").
		Joins("JOIN users ON users.id = ca.user_id")
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	rows := make([]CreatorApplicationView, 0)
	err := base.
		Select(`ca.id, ca.user_id, users.username, users.display_name, users.avatar,
			ca.reason, ca.status, ca.reviewer_id, ca.reject_reason, ca.reviewed_at, ca.created_at, ca.updated_at`).
		Order("CASE ca.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 WHEN 'approved' THEN 2 ELSE 3 END, ca.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	stats, err := r.applicationStatusStats(ctx, "creator_applications")
	if err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	return rows, total, stats, nil
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

func (r *UserRepo) ListPlatformApplicationsPage(ctx context.Context, page, size int) ([]PlatformApplicationView, int64, AdminStatusStats, error) {
	page, size = normalizeAdminPage(page, size)
	base := r.db.WithContext(ctx).
		Table("platform_applications AS pa").
		Joins("JOIN users ON users.id = pa.user_id")
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	rows := make([]PlatformApplicationView, 0)
	err := base.
		Select(`pa.id, pa.user_id, users.username, users.display_name, users.avatar,
			users.live_permission_status,
			pa.reason, pa.status, pa.reviewer_id, pa.reject_reason, pa.reviewed_at, pa.created_at, pa.updated_at`).
		Order("CASE pa.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 WHEN 'approved' THEN 2 ELSE 3 END, pa.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	stats, err := r.applicationStatusStats(ctx, "platform_applications")
	if err != nil {
		return nil, 0, AdminStatusStats{}, err
	}
	return rows, total, stats, nil
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

func (r *UserRepo) ListLiveCreatorsPage(ctx context.Context, page, size int) ([]LiveCreatorView, int64, error) {
	page, size = normalizeAdminPage(page, size)
	q := r.db.WithContext(ctx).
		Table("users").
		Where("live_permission_status = ?", model.LivePermissionApproved)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]LiveCreatorView, 0)
	err := q.Select("id, username, display_name, avatar, live_permission_status, updated_at").
		Order("updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *UserRepo) applicationStatusStats(ctx context.Context, table string) (AdminStatusStats, error) {
	var rows []struct {
		Status string
		Total  int64
	}
	if err := r.db.WithContext(ctx).
		Table(table).
		Select("status, COUNT(*) AS total").
		Group("status").
		Scan(&rows).Error; err != nil {
		return AdminStatusStats{}, err
	}
	var stats AdminStatusStats
	for _, row := range rows {
		stats.Total += row.Total
		switch row.Status {
		case "pending":
			stats.Pending += row.Total
		case "approved":
			stats.Approved += row.Total
		case "rejected":
			stats.Rejected += row.Total
		}
	}
	return stats, nil
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
