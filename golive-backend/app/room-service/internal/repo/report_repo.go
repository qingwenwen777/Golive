package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"gorm.io/gorm"
)

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
	TargetVerified   bool
	Reason           string
	Status           string
	ReviewerID       string
	ReviewerName     string
	ReviewStartedAt  *time.Time
	ReviewExpiresAt  *time.Time
	ResolutionAction string
	ResolutionNote   string
	DurationMinutes  int
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ReportCount      int64
	RecentCount      int64
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
			openGroup, err := r.openReportGroup(ctx, tx, report.TargetType, report.TargetID, report.RoomID)
			if err != nil {
				return err
			}
			groupID := ""
			if openGroup != nil {
				groupID = strings.TrimSpace(openGroup.GroupID)
				if groupID == "" {
					groupID = openGroup.ID
				}
				if openGroup.Status == model.ReportStatusReviewing && openGroup.ReviewExpiresAt != nil && openGroup.ReviewExpiresAt.After(now) {
					report.Status = model.ReportStatusReviewing
					report.ReviewerID = openGroup.ReviewerID
					report.ReviewerName = openGroup.ReviewerName
					report.ReviewStartedAt = openGroup.ReviewStartedAt
					report.ReviewExpiresAt = openGroup.ReviewExpiresAt
					report.ResolutionAction = model.ReportActionReview
				}
			}
			if groupID == "" {
				groupID = uuid.NewString()
			}
			report.GroupID = groupID
		}
		return tx.Create(report).Error
	})
}

func (r *ModerationRepo) openReportGroup(ctx context.Context, tx *gorm.DB, targetType, targetID, roomID string) (*model.ContentReport, error) {
	var existing model.ContentReport
	q := tx.WithContext(ctx).Model(&model.ContentReport{}).
		Where("target_type = ? AND target_id = ? AND (status = ? OR status = ?)",
			targetType, targetID, model.ReportStatusPending, model.ReportStatusReviewing)
	if targetType == model.ReportTargetDanmu && roomID != "" {
		// Danmu ids are chosen by the sending client and only unique per room.
		q = q.Where("room_id = ?", roomID)
	}
	err := q.
		Order("CASE status WHEN 'reviewing' THEN 0 ELSE 1 END, created_at DESC").
		Limit(1).
		Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	groupID := strings.TrimSpace(existing.GroupID)
	if groupID == "" {
		groupID = existing.ID
		if err := tx.WithContext(ctx).Model(&model.ContentReport{}).
			Where("target_type = ? AND target_id = ? AND group_id = '' AND (status = ? OR status = ?)",
				targetType, targetID, model.ReportStatusPending, model.ReportStatusReviewing).
			Update("group_id", groupID).Error; err != nil {
			return nil, err
		}
	}
	existing.GroupID = groupID
	return &existing, nil
}

// VerifyLegacyReportTargets re-resolves open reports whose target fields are
// still what the reporter's client sent (target_verified = false: filed
// before targets were resolved server-side) and stores the server-side
// snapshot on them, so the user, text and link moderators see are what a
// sanction would hit. ids limits it to those reports. Reports whose content
// is gone stay unverified, cannot drive enforcement and are remembered so
// later calls skip them.
func (r *ModerationRepo) VerifyLegacyReportTargets(ctx context.Context, ids ...string) error {
	q := r.db.WithContext(ctx).
		Where("target_verified = ? AND (status = ? OR status = ?)", false, model.ReportStatusPending, model.ReportStatusReviewing)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var rows []model.ContentReport
	if err := q.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if _, gone := r.legacyTargetsGone.Load(row.ID); gone {
			continue
		}
		target, err := r.ResolveReportTarget(ctx, row.TargetType, row.TargetID, row.RoomID)
		if errors.Is(err, ErrReportTargetNotFound) {
			r.legacyTargetsGone.Store(row.ID, struct{}{})
			continue
		}
		if err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Model(&model.ContentReport{}).
			Where("id = ? AND target_verified = ?", row.ID, false).
			UpdateColumns(reportTargetColumns(target)).Error; err != nil {
			return err
		}
	}
	return nil
}

// reportTargetColumns is the content_reports snapshot of a resolved target,
// trimmed to the columns like CreateReport does.
func reportTargetColumns(target *ReportTarget) map[string]any {
	return map[string]any{
		"target_url":        trimForDB(target.Link, 800),
		"room_id":           trimForDB(target.RoomID, 64),
		"channel_id":        trimForDB(target.ChannelID, 64),
		"target_owner_id":   trimForDB(target.OwnerID, 36),
		"target_owner_name": trimForDB(target.OwnerName, 128),
		"target_user_id":    trimForDB(target.UserID, 36),
		"target_user_name":  trimForDB(target.UserName, 128),
		"target_title":      trimForDB(target.Title, 240),
		"target_text":       trimForDB(target.Text, 1000),
		"target_verified":   true,
	}
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
				TargetVerified:   row.TargetVerified,
				Reason:           row.Reason,
				Status:           row.Status,
				ReviewerID:       row.ReviewerID,
				ReviewerName:     row.ReviewerName,
				ReviewStartedAt:  row.ReviewStartedAt,
				ReviewExpiresAt:  row.ReviewExpiresAt,
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
		if row.Status == model.ReportStatusReviewing {
			group.ReviewerID = firstNonEmpty(group.ReviewerID, row.ReviewerID)
			group.ReviewerName = firstNonEmpty(group.ReviewerName, row.ReviewerName)
			group.ReviewStartedAt = firstNonEmptyTime(group.ReviewStartedAt, row.ReviewStartedAt)
			group.ReviewExpiresAt = firstNonEmptyTime(group.ReviewExpiresAt, row.ReviewExpiresAt)
			group.ResolutionAction = firstNonEmpty(group.ResolutionAction, row.ResolutionAction)
		}
		// Target fields come from one row as a whole, never merged across
		// reports, and a server-verified row always beats a legacy one.
		if row.TargetVerified && !group.TargetVerified {
			setReportGroupTarget(group, row)
		}
		if row.CreatedAt.After(group.CreatedAt) {
			group.ID = row.ID
			group.GroupID = firstNonEmpty(row.GroupID, group.GroupID)
			if row.TargetVerified || !group.TargetVerified {
				setReportGroupTarget(group, row)
			}
			group.Reason = firstNonEmpty(row.Reason, group.Reason)
			group.ReviewerID = firstNonEmpty(row.ReviewerID, group.ReviewerID)
			group.ReviewerName = firstNonEmpty(row.ReviewerName, group.ReviewerName)
			group.ReviewStartedAt = firstNonEmptyTime(row.ReviewStartedAt, group.ReviewStartedAt)
			group.ReviewExpiresAt = firstNonEmptyTime(row.ReviewExpiresAt, group.ReviewExpiresAt)
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

func (r *ModerationRepo) ReleaseExpiredContentReportReviews(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Model(&model.ContentReport{}).
		Where("status = ? AND (review_expires_at IS NULL OR review_expires_at <= ?)", model.ReportStatusReviewing, now).
		Updates(map[string]any{
			"status":            model.ReportStatusPending,
			"reviewer_id":       "",
			"reviewer_name":     "",
			"review_started_at": nil,
			"review_expires_at": nil,
			"resolution_action": "",
			"duration_minutes":  0,
			"resolution_note":   "",
			"resolved_at":       nil,
			"updated_at":        now,
		}).Error
}

func (r *ModerationRepo) ClaimContentReportGroup(ctx context.Context, id, reviewerID, reviewerName string, now time.Time, ttl time.Duration) (*model.ContentReport, error) {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	report, err := r.GetContentReport(ctx, id)
	if err != nil {
		return nil, err
	}
	if report.Status == model.ReportStatusResolved || report.Status == model.ReportStatusDismissed {
		return nil, ErrReportAlreadyClosed
	}
	hasOtherReviewer, err := r.contentReportGroupHasActiveReviewer(ctx, *report, reviewerID, now)
	if err != nil {
		return nil, err
	}
	if hasOtherReviewer {
		return nil, ErrReportClaimed
	}

	groupID, fallbackToTarget := reportUpdateGroupID(*report)
	startedAt := now
	if report.Status == model.ReportStatusReviewing &&
		report.ReviewerID == reviewerID &&
		report.ReviewStartedAt != nil &&
		!report.ReviewStartedAt.IsZero() &&
		report.ReviewExpiresAt != nil &&
		report.ReviewExpiresAt.After(now) {
		startedAt = *report.ReviewStartedAt
	}
	expiresAt := now.Add(ttl)
	updates := map[string]any{
		"status":            model.ReportStatusReviewing,
		"group_id":          groupID,
		"reviewer_id":       strings.TrimSpace(reviewerID),
		"reviewer_name":     trimForDB(reviewerName, 128),
		"review_started_at": startedAt,
		"review_expires_at": expiresAt,
		"resolution_action": model.ReportActionReview,
		"duration_minutes":  0,
		"resolution_note":   "",
		"resolved_at":       nil,
		"updated_at":        now,
	}
	q := r.contentReportGroupUpdateScope(ctx, *report, groupID, fallbackToTarget).
		Where(
			"status = ? OR (status = ? AND (reviewer_id = ? OR reviewer_id = '' OR review_expires_at IS NULL OR review_expires_at <= ?))",
			model.ReportStatusPending,
			model.ReportStatusReviewing,
			reviewerID,
			now,
		)
	res := q.Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		current, getErr := r.GetContentReport(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		if current.Status == model.ReportStatusResolved || current.Status == model.ReportStatusDismissed {
			return nil, ErrReportAlreadyClosed
		}
		return nil, ErrReportClaimed
	}
	return r.GetContentReport(ctx, id)
}

func (r *ModerationRepo) UpdateContentReport(ctx context.Context, id, status, reviewerID, note string, now time.Time) (*model.ContentReport, error) {
	return r.UpdateContentReportGroup(ctx, id, status, "", reviewerID, "", note, 0, now)
}

func (r *ModerationRepo) UpdateContentReportGroup(ctx context.Context, id, status, action, reviewerID, reviewerName, note string, durationMinutes int, now time.Time) (*model.ContentReport, error) {
	report, err := r.GetContentReport(ctx, id)
	if err != nil {
		return nil, err
	}
	if report.Status == model.ReportStatusResolved || report.Status == model.ReportStatusDismissed {
		return nil, ErrReportAlreadyClosed
	}
	hasOtherReviewer, err := r.contentReportGroupHasActiveReviewer(ctx, *report, reviewerID, now)
	if err != nil {
		return nil, err
	}
	if hasOtherReviewer {
		return nil, ErrReportClaimed
	}
	groupID, fallbackToTarget := reportUpdateGroupID(*report)
	updates := map[string]any{
		"status":            status,
		"group_id":          groupID,
		"reviewer_id":       reviewerID,
		"reviewer_name":     trimForDB(reviewerName, 128),
		"resolution_action": action,
		"duration_minutes":  durationMinutes,
		"resolution_note":   note,
		"updated_at":        now,
	}
	if status == model.ReportStatusResolved || status == model.ReportStatusDismissed {
		updates["resolved_at"] = now
		updates["review_started_at"] = nil
		updates["review_expires_at"] = nil
	} else {
		updates["resolved_at"] = nil
		if status == model.ReportStatusPending {
			updates["reviewer_id"] = ""
			updates["reviewer_name"] = ""
			updates["review_started_at"] = nil
			updates["review_expires_at"] = nil
			updates["resolution_action"] = ""
			updates["duration_minutes"] = 0
			updates["resolution_note"] = ""
		}
	}
	q := r.contentReportGroupUpdateScope(ctx, *report, groupID, fallbackToTarget).
		Where("status = ? OR status = ?", model.ReportStatusPending, model.ReportStatusReviewing)
	res := q.Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrReportAlreadyClosed
	}
	return r.GetContentReport(ctx, id)
}

func (r *ModerationRepo) contentReportGroupHasActiveReviewer(ctx context.Context, report model.ContentReport, reviewerID string, now time.Time) (bool, error) {
	groupID, fallbackToTarget := reportUpdateGroupID(report)
	var count int64
	err := r.contentReportGroupReadScope(ctx, report, groupID, fallbackToTarget).
		Where("status = ? AND reviewer_id <> '' AND reviewer_id <> ? AND review_expires_at > ?",
			model.ReportStatusReviewing, strings.TrimSpace(reviewerID), now).
		Count(&count).Error
	return count > 0, err
}

func (r *ModerationRepo) contentReportGroupUpdateScope(ctx context.Context, report model.ContentReport, groupID string, fallbackToTarget bool) *gorm.DB {
	return contentReportGroupScope(r.db.WithContext(ctx).Model(&model.ContentReport{}), report, groupID, fallbackToTarget)
}

func (r *ModerationRepo) contentReportGroupReadScope(ctx context.Context, report model.ContentReport, groupID string, fallbackToTarget bool) *gorm.DB {
	return contentReportGroupScope(r.db.WithContext(ctx).Model(&model.ContentReport{}), report, groupID, fallbackToTarget)
}

func contentReportGroupScope(q *gorm.DB, report model.ContentReport, groupID string, fallbackToTarget bool) *gorm.DB {
	if !fallbackToTarget && strings.TrimSpace(groupID) != "" {
		return q.Where("group_id = ?", groupID)
	}
	return q.Where("target_type = ? AND target_id = ? AND (status = ? OR status = ?)",
		report.TargetType, report.TargetID, model.ReportStatusPending, model.ReportStatusReviewing)
}

func reportUpdateGroupID(report model.ContentReport) (string, bool) {
	groupID := strings.TrimSpace(report.GroupID)
	if groupID == "" {
		return report.ID, true
	}
	return groupID, false
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

// DeleteReportedContent deletes reported content stored by room-service
// (posts and post comments). Chat messages and super chats belong to
// chat-service / gift-service and are removed through their APIs.
func (r *ModerationRepo) DeleteReportedContent(ctx context.Context, targetType, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	switch targetType {
	case model.ReportTargetPost:
		return r.deletePostAny(ctx, targetID)
	case model.ReportTargetPostComment:
		return r.deletePostCommentAny(ctx, targetID)
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

func setReportGroupTarget(group *ReportGroupRow, row model.ContentReport) {
	group.TargetURL = row.TargetURL
	group.RoomID = row.RoomID
	group.ChannelID = row.ChannelID
	group.TargetOwnerID = row.TargetOwnerID
	group.TargetOwnerName = row.TargetOwnerName
	group.TargetUserID = row.TargetUserID
	group.TargetUserName = row.TargetUserName
	group.TargetTitle = row.TargetTitle
	group.TargetText = row.TargetText
	group.TargetVerified = row.TargetVerified
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
		case model.ReportStatusReviewing:
			return 0
		case model.ReportStatusPending:
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
