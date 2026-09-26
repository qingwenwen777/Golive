package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
		newUsername, newDisplayName := "", ""
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
				newUsername = next
				updates["username"] = next
			}
		}
		if displayName != nil {
			next := strings.TrimSpace(*displayName)
			if next != u.DisplayName {
				newDisplayName = next
			}
			updates["display_name"] = next
		}
		if err := checkNewNames(tx, userID, newUsername, newDisplayName); err != nil {
			return err
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

// AdminSetUserBan sets or clears a ban on both users and
// user_moderation_states and syncs the Redis ban flag. operatorID (may be
// empty) is recorded as the state's updated_by.
func (r *UserRepo) AdminSetUserBan(ctx context.Context, userID string, banned bool, reason, operatorID string) (*model.User, error) {
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
		if operatorID = strings.TrimSpace(operatorID); operatorID != "" {
			state.UpdatedBy = operatorID
			stateUpdates["updated_by"] = operatorID
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

// SetUserMute sets (mutedUntil non-nil) or clears a site-wide mute in
// user_moderation_states and syncs the Redis mute flag.
func (r *UserRepo) SetUserMute(ctx context.Context, userID string, mutedUntil *time.Time, reason, operatorID string) error {
	now := time.Now().UTC()
	cleanReason := strings.TrimSpace(reason)
	if mutedUntil == nil {
		cleanReason = ""
	}
	operatorID = strings.TrimSpace(operatorID)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u model.User
		if err := tx.Select("id").Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		state := model.UserModerationState{
			UserID:     userID,
			MutedUntil: mutedUntil,
			MuteReason: cleanReason,
			UpdatedBy:  operatorID,
			UpdatedAt:  now,
			CreatedAt:  now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"muted_until": mutedUntil,
				"mute_reason": cleanReason,
				"updated_by":  operatorID,
				"updated_at":  now,
			}),
		}).Create(&state).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	return r.syncUserMuteCache(ctx, userID, mutedUntil, now)
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

func (r *UserRepo) syncUserMuteCache(ctx context.Context, userID string, mutedUntil *time.Time, now time.Time) error {
	if r.rdb == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	key := contentpolicy.RedisSiteMutePrefix + strings.TrimSpace(userID)
	if mutedUntil != nil && mutedUntil.After(now) {
		return r.rdb.Set(ctx, key, mutedUntil.UTC().Format(time.RFC3339), mutedUntil.Sub(now)).Err()
	}
	return r.rdb.Del(ctx, key).Err()
}
