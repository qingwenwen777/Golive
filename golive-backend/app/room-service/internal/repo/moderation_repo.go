package repo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
)

var ErrModeratorNotFound = errors.New("moderator not found")

var (
	ErrReportDuplicate     = errors.New("report duplicate within 24h")
	ErrReportDailyLimit    = errors.New("daily report limit reached")
	ErrReportAlreadyClosed = errors.New("report group already handled")
	ErrBlockedWordExists   = errors.New("blocked word exists")
	ErrBlockedWordNotFound = errors.New("blocked word not found")
)

type ModerationRepo struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewModerationRepo(db *gorm.DB, rdb *redis.Client) *ModerationRepo {
	return &ModerationRepo{db: db, rdb: rdb}
}

func (r *ModerationRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.RoomModerator{},
		&model.RoomMute{},
		&model.ModeratorActionLog{},
		&model.ContentReport{},
		&model.AdminAuditLog{},
		&model.UserModerationState{},
		&model.UserSanctionLog{},
		&model.UnbanAppeal{},
		&model.BlockedWord{},
	)
}

type ModerationUser struct {
	ID          string
	Username    string
	DisplayName string
	Name        string
	Avatar      string
	Verified    bool
	Moderator   bool
	CreatedAt   *time.Time
}

type ModerationLogRow struct {
	ID              string
	OwnerID         string
	RoomID          string
	ActorID         string
	ActorName       string
	ActorAvatar     string
	TargetUserID    string
	TargetName      string
	TargetAvatar    string
	Action          string
	DurationMinutes int
	CreatedAt       time.Time
}

type ReportListFilter struct {
	Status     string
	TargetType string
	Reason     string
	Query      string
	Page       int
	Size       int
}

type ReportStats struct {
	Pending   int64
	Reviewing int64
	Today     int64
	Total     int64
}

type ReportGroupRow struct {
	ID               string
	GroupID          string
	TargetType       string
	TargetID         string
	TargetURL        string
	RoomID           string
	ChannelID        string
	TargetOwnerID    string
	TargetOwnerName  string
	TargetUserID     string
	TargetUserName   string
	TargetTitle      string
	TargetText       string
	Reason           string
	Status           string
	ReviewerID       string
	ResolutionAction string
	ResolutionNote   string
	DurationMinutes  int
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ReportCount      int64
	RecentCount      int64
}

type AdminAuditLogRow struct {
	ID             string
	Category       string
	Action         string
	ActorID        string
	ActorName      string
	TargetType     string
	TargetID       string
	TargetTitle    string
	TargetUserID   string
	TargetUserName string
	Note           string
	Metadata       string
	CreatedAt      time.Time
}

type AdminAuditStats struct {
	Today      int64
	Review     int64
	Permission int64
	System     int64
}

type UserRestriction struct {
	Banned        bool
	Muted         bool
	MuteRemaining time.Duration
	MuteExpiresAt *time.Time
	BanReason     string
	MuteReason    string
}

type AdminDashboardMetrics struct {
	OnlineRooms       int64
	OnlineViewers     int64
	TodayNewUsers     int64
	TodayRevenueCoins int64
	Health            []AdminHealthItem
}

type AdminHealthItem struct {
	Key     string
	Label   string
	Status  string
	Detail  string
	Checked bool
}

func (r *ModerationRepo) ListFollowers(ctx context.Context, ownerID, query string, page, size int, followerIDs []string) ([]ModerationUser, int64, error) {
	page, size = normalizeModerationPage(page, size)
	if len(followerIDs) == 0 {
		return []ModerationUser{}, 0, nil
	}
	q := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified,
CASE WHEN rm.user_id IS NULL THEN false ELSE true END AS moderator,
rm.created_at AS created_at
`).
		Joins("LEFT JOIN room_moderators AS rm ON rm.owner_id = ? AND rm.user_id = u.id AND rm.revoked_at IS NULL", ownerID).
		Where("u.id IN ?", followerIDs)
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("LOWER(COALESCE(u.username, '')) LIKE ? OR LOWER(COALESCE(u.display_name, '')) LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModerationUser
	err := q.Order("moderator DESC, u.updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) ListModerators(ctx context.Context, ownerID string, page, size int) ([]ModerationUser, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).
		Table("room_moderators AS rm").
		Joins("JOIN users AS u ON u.id = rm.user_id").
		Where("rm.owner_id = ? AND rm.revoked_at IS NULL", ownerID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModerationUser
	err := q.Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified,
true AS moderator,
rm.created_at AS created_at
`).
		Order("rm.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) AddModerator(ctx context.Context, ownerID, actorID, targetUserID string, now time.Time) error {
	actor, err := r.UserProfile(ctx, actorID)
	if err != nil {
		return err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"created_by": actorID,
				"created_at": now,
				"updated_at": now,
				"revoked_at": nil,
			}),
		}).Create(&model.RoomModerator{
			OwnerID:   ownerID,
			UserID:    targetUserID,
			CreatedBy: actorID,
			CreatedAt: now,
			UpdatedAt: now,
			RevokedAt: nil,
		}).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, "", actor, target, model.ModeratorActionAdd, 0, now)).Error
	})
}

func (r *ModerationRepo) RemoveModerator(ctx context.Context, ownerID, actorID, targetUserID string, now time.Time) error {
	actor, err := r.UserProfile(ctx, actorID)
	if err != nil {
		return err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.RoomModerator{}).
			Where("owner_id = ? AND user_id = ? AND revoked_at IS NULL", ownerID, targetUserID).
			Updates(map[string]any{
				"revoked_at": now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrModeratorNotFound
		}
		return tx.Create(actionLog(ownerID, "", actor, target, model.ModeratorActionRemove, 0, now)).Error
	})
}

func (r *ModerationRepo) IsModerator(ctx context.Context, ownerID, userID string) (bool, error) {
	if ownerID == "" || userID == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.RoomModerator{}).
		Where("owner_id = ? AND user_id = ? AND revoked_at IS NULL", ownerID, userID).
		Count(&count).Error
	return count > 0, err
}

func (r *ModerationRepo) ActiveModeratorIDs(ctx context.Context, ownerID string) ([]string, error) {
	if ownerID == "" {
		return nil, nil
	}
	var ids []string
	err := r.db.WithContext(ctx).Model(&model.RoomModerator{}).
		Where("owner_id = ? AND revoked_at IS NULL", ownerID).
		Pluck("user_id", &ids).Error
	return ids, err
}

func (r *ModerationRepo) MuteUser(ctx context.Context, ownerID, roomID, operatorID, operatorRole, targetUserID, targetName, targetAvatar string, durationMinutes int, now time.Time) (*model.RoomMute, error) {
	actor, err := r.UserProfile(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err == nil {
		targetName = target.Name
		targetAvatar = target.Avatar
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		target = ModerationUser{ID: targetUserID, Name: strings.TrimSpace(targetName), Avatar: strings.TrimSpace(targetAvatar)}
		if target.Name == "" {
			target.Name = targetUserID
		}
	} else {
		return nil, err
	}

	expiresAt := now.Add(time.Duration(durationMinutes) * time.Minute)
	mute := &model.RoomMute{
		ID:              uuid.NewString(),
		RoomID:          roomID,
		OwnerID:         ownerID,
		TargetUserID:    targetUserID,
		TargetName:      targetName,
		TargetAvatar:    targetAvatar,
		OperatorID:      operatorID,
		OperatorRole:    operatorRole,
		DurationMinutes: durationMinutes,
		StartedAt:       now,
		ExpiresAt:       expiresAt,
		CreatedAt:       now,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(mute).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, roomID, actor, target, model.ModeratorActionMute, durationMinutes, now)).Error
	}); err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if err := r.rdb.Set(ctx, roomMuteKey(roomID, targetUserID), expiresAt.UTC().Format(time.RFC3339), expiresAt.Sub(now)).Err(); err != nil {
			return nil, err
		}
	}
	return mute, nil
}

func (r *ModerationRepo) CurrentMute(ctx context.Context, roomID, targetUserID string, now time.Time) (*model.RoomMute, error) {
	if roomID == "" || targetUserID == "" {
		return nil, nil
	}
	var mute model.RoomMute
	err := r.db.WithContext(ctx).
		Where("room_id = ? AND target_user_id = ? AND expires_at > ?", roomID, targetUserID, now).
		Order("expires_at DESC").
		Take(&mute).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if ttl := mute.ExpiresAt.Sub(now); ttl > 0 {
			_ = r.rdb.Set(ctx, roomMuteKey(roomID, targetUserID), mute.ExpiresAt.UTC().Format(time.RFC3339), ttl).Err()
		}
	}
	return &mute, nil
}

func (r *ModerationRepo) UnmuteUser(ctx context.Context, ownerID, roomID, operatorID, targetUserID, targetName, targetAvatar string, now time.Time) (*model.RoomMute, error) {
	actor, err := r.UserProfile(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	current, err := r.CurrentMute(ctx, roomID, targetUserID, now)
	if err != nil {
		return nil, err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err == nil {
		targetName = target.Name
		targetAvatar = target.Avatar
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		target = ModerationUser{ID: targetUserID, Name: strings.TrimSpace(targetName), Avatar: strings.TrimSpace(targetAvatar)}
		if target.Name == "" && current != nil {
			target.Name = current.TargetName
			target.Avatar = current.TargetAvatar
		}
		if target.Name == "" {
			target.Name = targetUserID
		}
	} else {
		return nil, err
	}
	if current == nil {
		if r.rdb != nil {
			_ = r.rdb.Del(ctx, roomMuteKey(roomID, targetUserID)).Err()
		}
		return nil, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.RoomMute{}).
			Where("room_id = ? AND target_user_id = ? AND expires_at > ?", roomID, targetUserID, now).
			Update("expires_at", now).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, roomID, actor, target, model.ModeratorActionUnmute, 0, now)).Error
	})
	if err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if err := r.rdb.Del(ctx, roomMuteKey(roomID, targetUserID)).Err(); err != nil {
			return nil, err
		}
	}
	current.ExpiresAt = now
	return current, nil
}

func (r *ModerationRepo) Logs(ctx context.Context, ownerID string, page, size int) ([]ModerationLogRow, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).Model(&model.ModeratorActionLog{}).Where("owner_id = ?", ownerID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModerationLogRow
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) CreateContentReport(ctx context.Context, report *model.ContentReport, now time.Time) error {
	if report == nil {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var daily int64
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if err := tx.Model(&model.ContentReport{}).
			Where("reporter_id = ? AND created_at >= ?", report.ReporterID, dayStart).
			Count(&daily).Error; err != nil {
			return err
		}
		if daily >= 20 {
			return ErrReportDailyLimit
		}
		var duplicate int64
		if err := tx.Model(&model.ContentReport{}).
			Where("reporter_id = ? AND target_type = ? AND target_id = ? AND created_at >= ?",
				report.ReporterID, report.TargetType, report.TargetID, now.Add(-24*time.Hour)).
			Count(&duplicate).Error; err != nil {
			return err
		}
		if duplicate > 0 {
			return ErrReportDuplicate
		}
		if strings.TrimSpace(report.GroupID) == "" {
			groupID, err := r.openReportGroupID(ctx, tx, report.TargetType, report.TargetID)
			if err != nil {
				return err
			}
			if groupID == "" {
				groupID = uuid.NewString()
			}
			report.GroupID = groupID
		}
		return tx.Create(report).Error
	})
}

func (r *ModerationRepo) openReportGroupID(ctx context.Context, tx *gorm.DB, targetType, targetID string) (string, error) {
	var existing model.ContentReport
	err := tx.WithContext(ctx).Model(&model.ContentReport{}).
		Where("target_type = ? AND target_id = ? AND (status = ? OR status = ?)",
			targetType, targetID, model.ReportStatusPending, model.ReportStatusReviewing).
		Order("created_at DESC").
		Limit(1).
		Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	groupID := strings.TrimSpace(existing.GroupID)
	if groupID == "" {
		groupID = existing.ID
		if err := tx.WithContext(ctx).Model(&model.ContentReport{}).
			Where("target_type = ? AND target_id = ? AND group_id = '' AND (status = ? OR status = ?)",
				targetType, targetID, model.ReportStatusPending, model.ReportStatusReviewing).
			Update("group_id", groupID).Error; err != nil {
			return "", err
		}
	}
	return groupID, nil
}

func (r *ModerationRepo) ListContentReports(ctx context.Context, filter ReportListFilter) ([]model.ContentReport, int64, error) {
	page, size := normalizeModerationPage(filter.Page, filter.Size)
	q := r.db.WithContext(ctx).Model(&model.ContentReport{})
	if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if targetType := strings.TrimSpace(filter.TargetType); targetType != "" && targetType != "all" {
		q = q.Where("target_type = ?", targetType)
	}
	if reason := strings.TrimSpace(filter.Reason); reason != "" && reason != "all" {
		q = q.Where("reason = ?", reason)
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" {
		like := "%" + query + "%"
		q = q.Where(`
LOWER(COALESCE(reporter_name, '')) LIKE ?
OR LOWER(COALESCE(target_owner_name, '')) LIKE ?
OR LOWER(COALESCE(target_user_name, '')) LIKE ?
OR LOWER(COALESCE(target_title, '')) LIKE ?
OR LOWER(COALESCE(target_text, '')) LIKE ?
OR LOWER(COALESCE(description, '')) LIKE ?
OR LOWER(COALESCE(target_id, '')) LIKE ?
`, like, like, like, like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ContentReport
	err := q.Order("CASE status WHEN 'pending' THEN 0 WHEN 'reviewing' THEN 1 WHEN 'resolved' THEN 2 ELSE 3 END, created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) ListContentReportGroups(ctx context.Context, filter ReportListFilter) ([]ReportGroupRow, int64, ReportStats, error) {
	q := r.db.WithContext(ctx).Model(&model.ContentReport{})
	q = applyReportFilters(q, filter)

	var rows []model.ContentReport
	if err := q.Order("created_at DESC").Limit(2000).Find(&rows).Error; err != nil {
		return nil, 0, ReportStats{}, err
	}

	recentAfter := time.Now().Add(-time.Hour)
	groups := make([]ReportGroupRow, 0, len(rows))
	byKey := make(map[string]int, len(rows))
	for _, row := range rows {
		key := reportGroupKey(row)
		idx, ok := byKey[key]
		if !ok {
			byKey[key] = len(groups)
			groups = append(groups, ReportGroupRow{
				ID:               row.ID,
				GroupID:          row.GroupID,
				TargetType:       row.TargetType,
				TargetID:         row.TargetID,
				TargetURL:        row.TargetURL,
				RoomID:           row.RoomID,
				ChannelID:        row.ChannelID,
				TargetOwnerID:    row.TargetOwnerID,
				TargetOwnerName:  row.TargetOwnerName,
				TargetUserID:     row.TargetUserID,
				TargetUserName:   row.TargetUserName,
				TargetTitle:      row.TargetTitle,
				TargetText:       row.TargetText,
				Reason:           row.Reason,
				Status:           row.Status,
				ReviewerID:       row.ReviewerID,
				ResolutionAction: row.ResolutionAction,
				ResolutionNote:   row.ResolutionNote,
				DurationMinutes:  row.DurationMinutes,
				ResolvedAt:       row.ResolvedAt,
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
				ReportCount:      0,
				RecentCount:      0,
			})
			idx = len(groups) - 1
		}
		group := &groups[idx]
		group.ReportCount++
		if row.CreatedAt.After(recentAfter) {
			group.RecentCount++
		}
		group.Status = mergeReportGroupStatus(group.Status, row.Status)
		if row.CreatedAt.After(group.CreatedAt) {
			group.ID = row.ID
			group.GroupID = firstNonEmpty(row.GroupID, group.GroupID)
			group.TargetURL = firstNonEmpty(row.TargetURL, group.TargetURL)
			group.RoomID = firstNonEmpty(row.RoomID, group.RoomID)
			group.ChannelID = firstNonEmpty(row.ChannelID, group.ChannelID)
			group.TargetOwnerID = firstNonEmpty(row.TargetOwnerID, group.TargetOwnerID)
			group.TargetOwnerName = firstNonEmpty(row.TargetOwnerName, group.TargetOwnerName)
			group.TargetUserID = firstNonEmpty(row.TargetUserID, group.TargetUserID)
			group.TargetUserName = firstNonEmpty(row.TargetUserName, group.TargetUserName)
			group.TargetTitle = firstNonEmpty(row.TargetTitle, group.TargetTitle)
			group.TargetText = firstNonEmpty(row.TargetText, group.TargetText)
			group.Reason = firstNonEmpty(row.Reason, group.Reason)
			group.ReviewerID = firstNonEmpty(row.ReviewerID, group.ReviewerID)
			group.ResolutionAction = firstNonEmpty(row.ResolutionAction, group.ResolutionAction)
			group.ResolutionNote = firstNonEmpty(row.ResolutionNote, group.ResolutionNote)
			group.DurationMinutes = row.DurationMinutes
			group.ResolvedAt = row.ResolvedAt
			group.CreatedAt = row.CreatedAt
		}
		if row.UpdatedAt.After(group.UpdatedAt) {
			group.UpdatedAt = row.UpdatedAt
		}
	}

	stats := ReportStats{}
	for _, group := range groups {
		switch group.Status {
		case model.ReportStatusPending:
			stats.Pending++
		case model.ReportStatusReviewing:
			stats.Reviewing++
		}
		if sameDay(group.CreatedAt, time.Now()) {
			stats.Today++
		}
	}
	stats.Total = int64(len(groups))

	page, size := normalizeModerationPage(filter.Page, filter.Size)
	total := int64(len(groups))
	start := (page - 1) * size
	if start >= len(groups) {
		return []ReportGroupRow{}, total, stats, nil
	}
	end := start + size
	if end > len(groups) {
		end = len(groups)
	}
	return groups[start:end], total, stats, nil
}

func (r *ModerationRepo) ContentReportsForGroup(ctx context.Context, report model.ContentReport) ([]model.ContentReport, error) {
	var rows []model.ContentReport
	q := r.db.WithContext(ctx).Model(&model.ContentReport{})
	if report.GroupID != "" {
		q = q.Where("group_id = ?", report.GroupID)
	} else if report.Status == model.ReportStatusPending || report.Status == model.ReportStatusReviewing {
		q = q.Where("target_type = ? AND target_id = ? AND (status = ? OR status = ?)",
			report.TargetType, report.TargetID, model.ReportStatusPending, model.ReportStatusReviewing)
	} else {
		q = q.Where("id = ?", report.ID)
	}
	err := q.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (r *ModerationRepo) ContentReportStats(ctx context.Context, now time.Time) (ReportStats, error) {
	var stats ReportStats
	if err := r.db.WithContext(ctx).Model(&model.ContentReport{}).
		Where("status = ?", model.ReportStatusPending).Count(&stats.Pending).Error; err != nil {
		return stats, err
	}
	if err := r.db.WithContext(ctx).Model(&model.ContentReport{}).
		Where("status = ?", model.ReportStatusReviewing).Count(&stats.Reviewing).Error; err != nil {
		return stats, err
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := r.db.WithContext(ctx).Model(&model.ContentReport{}).
		Where("created_at >= ?", dayStart).Count(&stats.Today).Error; err != nil {
		return stats, err
	}
	if err := r.db.WithContext(ctx).Model(&model.ContentReport{}).Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *ModerationRepo) GetContentReport(ctx context.Context, id string) (*model.ContentReport, error) {
	var report model.ContentReport
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *ModerationRepo) UpdateContentReport(ctx context.Context, id, status, reviewerID, note string, now time.Time) (*model.ContentReport, error) {
	return r.UpdateContentReportGroup(ctx, id, status, "", reviewerID, note, 0, now)
}

func (r *ModerationRepo) UpdateContentReportGroup(ctx context.Context, id, status, action, reviewerID, note string, durationMinutes int, now time.Time) (*model.ContentReport, error) {
	report, err := r.GetContentReport(ctx, id)
	if err != nil {
		return nil, err
	}
	if report.Status == model.ReportStatusResolved || report.Status == model.ReportStatusDismissed {
		return nil, ErrReportAlreadyClosed
	}
	if report.GroupID == "" {
		report.GroupID = report.ID
	}
	updates := map[string]any{
		"status":            status,
		"group_id":          report.GroupID,
		"reviewer_id":       reviewerID,
		"resolution_action": action,
		"duration_minutes":  durationMinutes,
		"resolution_note":   note,
		"updated_at":        now,
	}
	if status == model.ReportStatusResolved || status == model.ReportStatusDismissed {
		updates["resolved_at"] = now
	} else {
		updates["resolved_at"] = nil
	}
	q := r.db.WithContext(ctx).Model(&model.ContentReport{})
	if report.GroupID != "" {
		q = q.Where("group_id = ? AND (status = ? OR status = ?)", report.GroupID, model.ReportStatusPending, model.ReportStatusReviewing)
	} else {
		q = q.Where("target_type = ? AND target_id = ? AND (status = ? OR status = ?)",
			report.TargetType, report.TargetID, model.ReportStatusPending, model.ReportStatusReviewing)
	}
	res := q.Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrReportAlreadyClosed
	}
	return r.GetContentReport(ctx, id)
}

func applyReportFilters(q *gorm.DB, filter ReportListFilter) *gorm.DB {
	if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if targetType := strings.TrimSpace(filter.TargetType); targetType != "" && targetType != "all" {
		q = q.Where("target_type = ?", targetType)
	}
	if reason := strings.TrimSpace(filter.Reason); reason != "" && reason != "all" {
		q = q.Where("reason = ?", reason)
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" {
		like := "%" + query + "%"
		q = q.Where(`
LOWER(COALESCE(reporter_name, '')) LIKE ?
OR LOWER(COALESCE(target_owner_name, '')) LIKE ?
OR LOWER(COALESCE(target_user_name, '')) LIKE ?
OR LOWER(COALESCE(target_title, '')) LIKE ?
OR LOWER(COALESCE(target_text, '')) LIKE ?
OR LOWER(COALESCE(description, '')) LIKE ?
OR LOWER(COALESCE(target_id, '')) LIKE ?
`, like, like, like, like, like, like, like)
	}
	return q
}

func (r *ModerationRepo) DeleteReportedContent(ctx context.Context, targetType, targetID, roomID string, now time.Time) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	switch targetType {
	case model.ReportTargetPost:
		return r.deletePostAny(ctx, targetID)
	case model.ReportTargetPostComment:
		return r.deletePostCommentAny(ctx, targetID)
	case model.ReportTargetDanmu:
		return r.hideDanmu(ctx, strings.TrimSpace(roomID), targetID, now)
	case model.ReportTargetSuperChat:
		return r.db.WithContext(ctx).
			Table("super_chat_orders").
			Where("order_id = ? AND status = ?", targetID, "success").
			Updates(map[string]any{"status": "failed", "fail_reason": "moderated"}).Error
	default:
		return nil
	}
}

func (r *ModerationRepo) deletePostAny(ctx context.Context, postID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post model.ChannelPost
		if err := tx.Where("id = ?", postID).Take(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var commentIDs []string
		if err := tx.Model(&model.PostComment{}).Where("post_id = ?", postID).Pluck("id", &commentIDs).Error; err != nil {
			return err
		}
		if len(commentIDs) > 0 {
			if err := tx.Where("comment_id IN ?", commentIDs).Delete(&model.PostCommentLike{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", commentIDs).Delete(&model.PostComment{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("post_id = ?", postID).Delete(&model.PostLike{}).Error; err != nil {
			return err
		}
		return tx.Delete(&post).Error
	})
}

func (r *ModerationRepo) deletePostCommentAny(ctx context.Context, commentID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target model.PostComment
		if err := tx.Where("id = ?", commentID).Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var comments []model.PostComment
		if err := tx.Where("post_id = ?", target.PostID).Find(&comments).Error; err != nil {
			return err
		}
		children := make(map[string][]string, len(comments))
		for _, comment := range comments {
			if comment.ParentID != "" {
				children[comment.ParentID] = append(children[comment.ParentID], comment.ID)
			}
		}
		ids := make([]string, 0, 3)
		var walk func(string)
		walk = func(id string) {
			ids = append(ids, id)
			for _, childID := range children[id] {
				walk(childID)
			}
		}
		walk(target.ID)
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Where("comment_id IN ?", ids).Delete(&model.PostCommentLike{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Delete(&model.PostComment{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ChannelPost{}).
			Where("id = ?", target.PostID).
			Update("comment_count", gorm.Expr("CASE WHEN comment_count >= ? THEN comment_count - ? ELSE 0 END", len(ids), len(ids))).Error; err != nil {
			return err
		}
		if target.ParentID != "" {
			if err := tx.Model(&model.PostComment{}).
				Where("id = ?", target.ParentID).
				Update("reply_count", gorm.Expr("CASE WHEN reply_count > 0 THEN reply_count - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ModerationRepo) hideDanmu(ctx context.Context, roomID, danmuID string, now time.Time) error {
	if roomID == "" {
		return nil
	}
	for i := 0; i < 8; i++ {
		table := fmt.Sprintf("danmus_%d", i)
		err := r.db.WithContext(ctx).Table(table).
			Where("room_id = ? AND id = ?", roomID, danmuID).
			Update("deleted_at", now).Error
		if err != nil && !isMissingTableName(err) && !isMissingColumn(err) {
			return err
		}
	}
	return nil
}

func (r *ModerationRepo) CreateNotification(ctx context.Context, n model.Notification) error {
	if strings.TrimSpace(n.UserID) == "" {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error
}

func (r *ModerationRepo) CreateAdminAuditLog(ctx context.Context, log *model.AdminAuditLog) error {
	if log == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *ModerationRepo) ListAdminAuditLogs(ctx context.Context, category string, page, size int, now time.Time) ([]AdminAuditLogRow, int64, AdminAuditStats, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).Model(&model.AdminAuditLog{})
	if category := strings.TrimSpace(category); category != "" && category != "all" {
		q = q.Where("category = ?", category)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, AdminAuditStats{}, err
	}
	var rows []AdminAuditLogRow
	err := q.Select(`
admin_audit_logs.id,
admin_audit_logs.category,
admin_audit_logs.action,
admin_audit_logs.actor_id,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), admin_audit_logs.actor_id) AS actor_name,
admin_audit_logs.target_type,
admin_audit_logs.target_id,
admin_audit_logs.target_title,
admin_audit_logs.target_user_id,
admin_audit_logs.target_user_name,
admin_audit_logs.note,
admin_audit_logs.metadata,
admin_audit_logs.created_at
`).
		Joins("LEFT JOIN users AS u ON u.id = admin_audit_logs.actor_id").
		Order("admin_audit_logs.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, AdminAuditStats{}, err
	}
	stats, err := r.AdminAuditStats(ctx, now)
	if err != nil {
		return nil, 0, AdminAuditStats{}, err
	}
	return rows, total, stats, nil
}

func (r *ModerationRepo) AdminAuditStats(ctx context.Context, now time.Time) (AdminAuditStats, error) {
	var stats AdminAuditStats
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := r.db.WithContext(ctx).Model(&model.AdminAuditLog{}).Where("created_at >= ?", dayStart).Count(&stats.Today).Error; err != nil {
		return stats, err
	}
	for _, item := range []struct {
		category string
		out      *int64
	}{
		{model.AdminAuditCategoryReview, &stats.Review},
		{model.AdminAuditCategoryPermission, &stats.Permission},
		{model.AdminAuditCategorySystem, &stats.System},
	} {
		if err := r.db.WithContext(ctx).Model(&model.AdminAuditLog{}).Where("category = ?", item.category).Count(item.out).Error; err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (r *ModerationRepo) ApplyUserSanction(ctx context.Context, targetUserID, targetName, operatorID, action, sourceReportID, note string, durationMinutes int, now time.Time) error {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil
	}
	if profile, err := r.UserProfile(ctx, targetUserID); err == nil && strings.TrimSpace(targetName) == "" {
		targetName = profile.Name
	}
	var expiresAt *time.Time
	if action == model.UserSanctionSiteMute && durationMinutes > 0 {
		expires := now.Add(time.Duration(durationMinutes) * time.Minute)
		expiresAt = &expires
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"updated_by": operatorID,
			"updated_at": now,
		}
		state := model.UserModerationState{
			UserID:    targetUserID,
			UpdatedBy: operatorID,
			UpdatedAt: now,
			CreatedAt: now,
		}
		switch action {
		case model.UserSanctionBan:
			state.Banned = true
			state.BanReason = note
			updates["banned"] = true
			updates["ban_reason"] = note
		case model.UserSanctionUnban:
			state.Banned = false
			state.MutedUntil = nil
			updates["banned"] = false
			updates["ban_reason"] = ""
			updates["muted_until"] = nil
			updates["mute_reason"] = ""
		case model.UserSanctionSiteMute:
			state.MutedUntil = expiresAt
			state.MuteReason = note
			updates["muted_until"] = expiresAt
			updates["mute_reason"] = note
		}
		if action == model.UserSanctionBan || action == model.UserSanctionUnban || action == model.UserSanctionSiteMute {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}},
				DoUpdates: clause.Assignments(updates),
			}).Create(&state).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.UserSanctionLog{
			ID:              uuid.NewString(),
			TargetUserID:    targetUserID,
			TargetUserName:  trimForDB(targetName, 128),
			Action:          action,
			OperatorID:      operatorID,
			SourceReportID:  sourceReportID,
			Note:            note,
			DurationMinutes: durationMinutes,
			ExpiresAt:       expiresAt,
			CreatedAt:       now,
		}).Error
	})
	if err != nil {
		return err
	}
	return r.syncUserRestriction(ctx, targetUserID, now)
}

func (r *ModerationRepo) UserRestriction(ctx context.Context, userID string, now time.Time) (UserRestriction, error) {
	var state model.UserModerationState
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Take(&state).Error
	restriction := UserRestriction{}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return UserRestriction{}, err
	}
	if err == nil {
		restriction.Banned = state.Banned
		restriction.BanReason = state.BanReason
		restriction.MuteReason = state.MuteReason
	}
	if err == nil && state.MutedUntil != nil && state.MutedUntil.After(now) {
		restriction.Muted = true
		restriction.MuteExpiresAt = state.MutedUntil
		restriction.MuteRemaining = state.MutedUntil.Sub(now)
	}
	var userState struct {
		Banned    bool
		BanReason string
	}
	userErr := r.db.WithContext(ctx).
		Table("users").
		Select("COALESCE(banned, false) AS banned, COALESCE(ban_reason, '') AS ban_reason").
		Where("id = ?", userID).
		Take(&userState).Error
	if userErr != nil && !errors.Is(userErr, gorm.ErrRecordNotFound) && !isMissingTableName(userErr) && !isMissingColumn(userErr) {
		return UserRestriction{}, userErr
	}
	if userErr == nil && userState.Banned {
		restriction.Banned = true
		if strings.TrimSpace(restriction.BanReason) == "" {
			restriction.BanReason = userState.BanReason
		}
	}
	return restriction, nil
}

func (r *ModerationRepo) SyncUserRestrictions(ctx context.Context, now time.Time) error {
	if r.rdb == nil {
		return nil
	}
	var states []model.UserModerationState
	if err := r.db.WithContext(ctx).
		Where("banned = ? OR muted_until > ?", true, now).
		Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		if err := r.syncUserRestriction(ctx, state.UserID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModerationRepo) syncUserRestriction(ctx context.Context, userID string, now time.Time) error {
	if r.rdb == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	restriction, err := r.UserRestriction(ctx, userID, now)
	if err != nil {
		return err
	}
	pipe := r.rdb.Pipeline()
	if restriction.Banned {
		pipe.Set(ctx, contentpolicy.RedisSiteBanPrefix+userID, "1", 0)
	} else {
		pipe.Del(ctx, contentpolicy.RedisSiteBanPrefix+userID)
	}
	if restriction.Muted && restriction.MuteRemaining > 0 {
		pipe.Set(ctx, contentpolicy.RedisSiteMutePrefix+userID, restriction.MuteExpiresAt.UTC().Format(time.RFC3339), restriction.MuteRemaining)
	} else {
		pipe.Del(ctx, contentpolicy.RedisSiteMutePrefix+userID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModerationRepo) CreateUnbanAppeal(ctx context.Context, appeal *model.UnbanAppeal) error {
	if appeal == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(appeal).Error
}

func (r *ModerationRepo) ListBlockedWords(ctx context.Context, page, size int) ([]model.BlockedWord, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).Model(&model.BlockedWord{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.BlockedWord
	err := q.Order("enabled DESC, updated_at DESC, created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) ActiveBlockedWords(ctx context.Context) ([]model.BlockedWord, error) {
	var rows []model.BlockedWord
	err := r.db.WithContext(ctx).Model(&model.BlockedWord{}).
		Where("enabled = ?", true).
		Order("updated_at DESC, created_at DESC").
		Find(&rows).Error
	return rows, err
}

func (r *ModerationRepo) CreateBlockedWord(ctx context.Context, word *model.BlockedWord) error {
	if err := r.db.WithContext(ctx).Create(word).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrBlockedWordExists
		}
		return err
	}
	return r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) UpdateBlockedWord(ctx context.Context, id string, updates map[string]any) (*model.BlockedWord, error) {
	var row model.BlockedWord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.BlockedWord{}).Where("id = ?", id).Updates(updates)
		if res.Error != nil {
			if strings.Contains(strings.ToLower(res.Error.Error()), "duplicate") || strings.Contains(strings.ToLower(res.Error.Error()), "unique") {
				return ErrBlockedWordExists
			}
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBlockedWordNotFound
		}
		return tx.Where("id = ?", id).Take(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return &row, r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) DeleteBlockedWord(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&model.BlockedWord{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBlockedWordNotFound
	}
	return r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) BlockedWordHit(ctx context.Context, texts ...string) (string, error) {
	if len(texts) == 0 {
		return "", nil
	}
	rows, err := r.ActiveBlockedWords(ctx)
	if err != nil {
		return "", err
	}
	words := make([]string, 0, len(rows))
	for _, row := range rows {
		words = append(words, row.NormalizedWord)
	}
	for _, text := range texts {
		if hit := contentpolicy.Hit(text, words); hit != "" {
			return hit, nil
		}
	}
	return "", nil
}

func (r *ModerationRepo) SyncBlockedWords(ctx context.Context) error {
	if r.rdb == nil {
		return nil
	}
	rows, err := r.ActiveBlockedWords(ctx)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, contentpolicy.RedisBlockedWordsKey)
	if len(rows) > 0 {
		members := make([]any, 0, len(rows))
		for _, row := range rows {
			if word := contentpolicy.NormalizeWord(row.NormalizedWord); word != "" {
				members = append(members, word)
			}
		}
		if len(members) > 0 {
			pipe.SAdd(ctx, contentpolicy.RedisBlockedWordsKey, members...)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModerationRepo) AdminDashboardMetrics(ctx context.Context, now time.Time) (AdminDashboardMetrics, error) {
	metrics := AdminDashboardMetrics{
		Health: []AdminHealthItem{
			{Key: "room-service", Label: "room-service", Status: "ok", Detail: "HTTP service responding", Checked: true},
			{Key: "api-gateway", Label: "api-gateway", Status: "unknown", Detail: "Health check not exposed to room-service", Checked: false},
			{Key: "user-service", Label: "user-service", Status: "unknown", Detail: "Health check not exposed to room-service", Checked: false},
			{Key: "gift-service", Label: "gift-service", Status: "unknown", Detail: "Health check not exposed to room-service", Checked: false},
			{Key: "chat-service", Label: "chat-service", Status: "unknown", Detail: "Health check not exposed to room-service", Checked: false},
			{Key: "im-gateway", Label: "im-gateway", Status: "unknown", Detail: "Health check not exposed to room-service", Checked: false},
			{Key: "nsq", Label: "NSQ", Status: "unknown", Detail: "No NSQ probe configured", Checked: false},
		},
	}
	if sqlDB, err := r.db.DB(); err != nil {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "mysql", Label: "MySQL", Status: "down", Detail: err.Error(), Checked: true})
	} else if err := sqlDB.PingContext(ctx); err != nil {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "mysql", Label: "MySQL", Status: "down", Detail: err.Error(), Checked: true})
	} else {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "mysql", Label: "MySQL", Status: "ok", Detail: "Connected", Checked: true})
	}
	if r.rdb == nil {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "redis", Label: "Redis", Status: "unknown", Detail: "Redis client not configured", Checked: false})
	} else if err := r.rdb.Ping(ctx).Err(); err != nil {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "redis", Label: "Redis", Status: "down", Detail: err.Error(), Checked: true})
	} else {
		metrics.Health = append(metrics.Health, AdminHealthItem{Key: "redis", Label: "Redis", Status: "ok", Detail: "Connected", Checked: true})
	}

	type roomRow struct {
		ID      string
		Viewers int64
	}
	var rooms []roomRow
	err := r.db.WithContext(ctx).
		Table("rooms").
		Select("id, viewers").
		Where("status IN ?", []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Scan(&rooms).Error
	if err != nil && !isMissingTableName(err) {
		return metrics, err
	}
	metrics.OnlineRooms = int64(len(rooms))
	for _, room := range rooms {
		viewers := room.Viewers
		if r.rdb != nil {
			values, err := r.rdb.HMGet(ctx, "roommetrics:"+room.ID, "viewers", "peak").Result()
			if err == nil && len(values) > 0 && values[0] != nil {
				viewers = parseRedisDashboardInt(values[0])
			}
		}
		metrics.OnlineViewers += viewers
	}

	dayStart := beijingDayStartUTC(now)
	err = r.db.WithContext(ctx).Table("users").Where("created_at >= ?", dayStart).Count(&metrics.TodayNewUsers).Error
	if err != nil && !isMissingTableName(err) {
		return metrics, err
	}
	err = r.db.WithContext(ctx).
		Table("coin_transactions").
		Select("COALESCE(SUM(amount), 0)").
		Where("type = ? AND amount > 0 AND created_at >= ?", "topup", dayStart).
		Row().
		Scan(&metrics.TodayRevenueCoins)
	if err != nil && !isMissingTableName(err) {
		return metrics, err
	}
	return metrics, nil
}

func (r *ModerationRepo) IsAdmin(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var role string
	err := r.db.WithContext(ctx).
		Table("users").
		Select("COALESCE(role, 'user')").
		Where("id = ?", userID).
		Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return role == "admin" || role == "moderator", err
}

func (r *ModerationRepo) UserProfile(ctx context.Context, userID string) (ModerationUser, error) {
	var row ModerationUser
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified
`).
		Where("u.id = ?", userID).
		Take(&row).Error
	return row, err
}

func (r *ModerationRepo) SyncRoomModerators(ctx context.Context, roomID, ownerID string) error {
	if r.rdb == nil || roomID == "" || ownerID == "" {
		return nil
	}
	ids, err := r.ActiveModeratorIDs(ctx, ownerID)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, roomOwnerKey(roomID), ownerID, 0)
	pipe.Del(ctx, roomModeratorsKey(roomID))
	if len(ids) > 0 {
		members := make([]any, 0, len(ids))
		for _, id := range ids {
			members = append(members, id)
		}
		pipe.SAdd(ctx, roomModeratorsKey(roomID), members...)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModerationRepo) SyncActiveRooms(ctx context.Context, rooms []model.Room) error {
	for _, room := range rooms {
		if err := r.SyncRoomModerators(ctx, room.ID, room.OwnerID); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModerationRepo) SyncActiveMutes(ctx context.Context, now time.Time) error {
	if r.rdb == nil {
		return nil
	}
	var mutes []model.RoomMute
	if err := r.db.WithContext(ctx).
		Where("expires_at > ?", now).
		Find(&mutes).Error; err != nil {
		return err
	}
	if len(mutes) == 0 {
		return nil
	}
	pipe := r.rdb.Pipeline()
	for _, mute := range mutes {
		ttl := mute.ExpiresAt.Sub(now)
		if ttl <= 0 {
			continue
		}
		pipe.Set(ctx, roomMuteKey(mute.RoomID, mute.TargetUserID), mute.ExpiresAt.UTC().Format(time.RFC3339), ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func actionLog(ownerID, roomID string, actor, target ModerationUser, action string, durationMinutes int, at time.Time) *model.ModeratorActionLog {
	return &model.ModeratorActionLog{
		ID:              uuid.NewString(),
		OwnerID:         ownerID,
		RoomID:          roomID,
		ActorID:         actor.ID,
		ActorName:       actor.Name,
		ActorAvatar:     actor.Avatar,
		TargetUserID:    target.ID,
		TargetName:      target.Name,
		TargetAvatar:    target.Avatar,
		Action:          action,
		DurationMinutes: durationMinutes,
		CreatedAt:       at,
	}
}

func normalizeModerationPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func reportGroupKey(row model.ContentReport) string {
	if row.GroupID != "" {
		return "group\x00" + row.GroupID
	}
	if row.Status == model.ReportStatusPending || row.Status == model.ReportStatusReviewing {
		return "open\x00" + strings.TrimSpace(row.TargetType) + "\x00" + strings.TrimSpace(row.TargetID)
	}
	return "closed\x00" + row.ID
}

func mergeReportGroupStatus(current, next string) string {
	rank := func(status string) int {
		switch status {
		case model.ReportStatusPending:
			return 0
		case model.ReportStatusReviewing:
			return 1
		case model.ReportStatusResolved:
			return 2
		default:
			return 3
		}
	}
	if current == "" || rank(next) < rank(current) {
		return next
	}
	return current
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func trimForDB(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func isMissingTableName(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

func isMissingColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown column") || strings.Contains(msg, "no such column")
}

func beijingDayStartUTC(now time.Time) time.Time {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).UTC()
}

func parseRedisDashboardInt(value any) int64 {
	switch v := value.(type) {
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

func roomModeratorsKey(roomID string) string { return "room:moderators:" + roomID }
func roomOwnerKey(roomID string) string      { return "room:owner:" + roomID }
func roomMuteKey(roomID, userID string) string {
	return "room:mute:" + roomID + ":" + userID
}
