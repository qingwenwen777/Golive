package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"gorm.io/gorm"
)

var allowedReportTargets = map[string]struct{}{
	model.ReportTargetRoom:        {},
	model.ReportTargetChannel:     {},
	model.ReportTargetDanmu:       {},
	model.ReportTargetPost:        {},
	model.ReportTargetPostComment: {},
	model.ReportTargetSuperChat:   {},
}

var allowedReportReasons = map[string]struct{}{
	model.ReportReasonSpam:       {},
	model.ReportReasonHarassment: {},
	model.ReportReasonSexual:     {},
	model.ReportReasonViolence:   {},
	model.ReportReasonHate:       {},
	model.ReportReasonScam:       {},
	model.ReportReasonIllegal:    {},
	model.ReportReasonOther:      {},
}

var allowedReportStatuses = map[string]struct{}{
	model.ReportStatusPending:   {},
	model.ReportStatusReviewing: {},
	model.ReportStatusResolved:  {},
	model.ReportStatusDismissed: {},
}

var allowedReportActions = map[string]struct{}{
	model.ReportActionReview:        {},
	model.ReportActionDismiss:       {},
	model.ReportActionDeleteContent: {},
	model.ReportActionWarnUser:      {},
	model.ReportActionWarnRoom:      {},
	model.ReportActionSiteMute:      {},
	model.ReportActionBanUser:       {},
	model.ReportActionForceEndLive:  {},
}

// CreateReportReq is the body of POST /rooms/reports. Only TargetType,
// TargetID, RoomID (used to locate danmu), Reason and Description are read;
// the other target fields the web client sends are ignored because the
// server resolves them from the reported content itself.
type CreateReportReq struct {
	TargetType      string `json:"targetType"`
	TargetID        string `json:"targetId"`
	TargetURL       string `json:"targetUrl"`
	RoomID          string `json:"roomId"`
	ChannelID       string `json:"channelId"`
	TargetOwnerID   string `json:"targetOwnerId"`
	TargetOwnerName string `json:"targetOwnerName"`
	TargetUserID    string `json:"targetUserId"`
	TargetUserName  string `json:"targetUserName"`
	TargetTitle     string `json:"targetTitle"`
	TargetText      string `json:"targetText"`
	Reason          string `json:"reason"`
	Description     string `json:"description"`
}

type UpdateReportReq struct {
	Status          string   `json:"status"`
	Action          string   `json:"action"`
	Actions         []string `json:"actions"`
	Note            string   `json:"note"`
	DurationMinutes int      `json:"durationMinutes"`
}

type ContentReportDTO struct {
	ID               string             `json:"id"`
	GroupID          string             `json:"groupId,omitempty"`
	ReporterID       string             `json:"reporterId"`
	ReporterName     string             `json:"reporterName"`
	ReporterAvatar   string             `json:"reporterAvatar,omitempty"`
	TargetType       string             `json:"targetType"`
	TargetID         string             `json:"targetId"`
	TargetURL        string             `json:"targetUrl,omitempty"`
	RoomID           string             `json:"roomId,omitempty"`
	ChannelID        string             `json:"channelId,omitempty"`
	TargetOwnerID    string             `json:"targetOwnerId,omitempty"`
	TargetOwnerName  string             `json:"targetOwnerName,omitempty"`
	TargetUserID     string             `json:"targetUserId,omitempty"`
	TargetUserName   string             `json:"targetUserName,omitempty"`
	TargetTitle      string             `json:"targetTitle,omitempty"`
	TargetText       string             `json:"targetText,omitempty"`
	Reason           string             `json:"reason"`
	Description      string             `json:"description,omitempty"`
	Status           string             `json:"status"`
	ReviewerID       string             `json:"reviewerId,omitempty"`
	ReviewerName     string             `json:"reviewerName,omitempty"`
	ReviewStartedAt  string             `json:"reviewStartedAt,omitempty"`
	ReviewExpiresAt  string             `json:"reviewExpiresAt,omitempty"`
	ResolutionAction string             `json:"resolutionAction,omitempty"`
	DurationMinutes  int                `json:"durationMinutes,omitempty"`
	ResolutionNote   string             `json:"resolutionNote,omitempty"`
	ResolvedAt       string             `json:"resolvedAt,omitempty"`
	CreatedAt        string             `json:"createdAt"`
	UpdatedAt        string             `json:"updatedAt"`
	ReportCount      int64              `json:"reportCount,omitempty"`
	RecentCount      int64              `json:"recentCount,omitempty"`
	Reports          []ContentReportDTO `json:"reports,omitempty"`
}

type ContentReportListResp struct {
	Items []ContentReportDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
	Stats ReportStatsDTO     `json:"stats"`
}

type ReportStatsDTO struct {
	Pending   int64 `json:"pending"`
	Reviewing int64 `json:"reviewing"`
	Today     int64 `json:"today"`
	Total     int64 `json:"total"`
}

func (s *ModerationService) CreateReport(ctx context.Context, reporterID string, req CreateReportReq) (*ContentReportDTO, error) {
	if reporterID == "" {
		return nil, errcode.ErrUnauthorized
	}
	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	if _, ok := allowedReportTargets[targetType]; !ok {
		return nil, errcode.New(http.StatusBadRequest, "invalid report target").WithReason("invalid_report_target")
	}
	targetID := strings.TrimSpace(req.TargetID)
	if targetID == "" {
		return nil, errcode.New(http.StatusBadRequest, "report target is required").WithReason("target_required")
	}
	reason := strings.ToLower(strings.TrimSpace(req.Reason))
	if _, ok := allowedReportReasons[reason]; !ok {
		return nil, errcode.New(http.StatusBadRequest, "report reason is required").WithReason("reason_required")
	}
	description := trimRunes(strings.TrimSpace(req.Description), 100)
	target, err := s.moderation.ResolveReportTarget(ctx, targetType, trimRunes(targetID, 128), trimRunes(strings.TrimSpace(req.RoomID), 64))
	if errors.Is(err, repo.ErrReportTargetNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report target not found").WithReason("target_not_found")
	}
	if err != nil {
		return nil, err
	}
	reporter, err := s.moderation.UserProfile(ctx, reporterID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.moderation.ReleaseExpiredContentReportReviews(ctx, now); err != nil {
		return nil, err
	}
	report := &model.ContentReport{
		ID:              uuid.NewString(),
		ReporterID:      reporterID,
		ReporterName:    reporter.Name,
		ReporterAvatar:  reporter.Avatar,
		TargetType:      targetType,
		TargetID:        trimRunes(target.TargetID, 128),
		TargetURL:       trimRunes(target.Link, 800),
		RoomID:          trimRunes(target.RoomID, 64),
		ChannelID:       trimRunes(target.ChannelID, 64),
		TargetOwnerID:   trimRunes(target.OwnerID, 36),
		TargetOwnerName: trimRunes(strings.TrimSpace(target.OwnerName), 128),
		TargetUserID:    trimRunes(target.UserID, 36),
		TargetUserName:  trimRunes(strings.TrimSpace(target.UserName), 128),
		TargetTitle:     trimRunes(strings.TrimSpace(target.Title), 240),
		TargetText:      trimRunes(strings.TrimSpace(target.Text), 1000),
		TargetVerified:  true,
		Reason:          reason,
		Description:     description,
		Status:          model.ReportStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.moderation.CreateContentReport(ctx, report, now); err != nil {
		switch {
		case errors.Is(err, repo.ErrReportDuplicate):
			return nil, errcode.New(http.StatusConflict, "report already submitted").WithReason("report_duplicate")
		case errors.Is(err, repo.ErrReportDailyLimit):
			return nil, errcode.New(http.StatusTooManyRequests, "daily report limit reached").WithReason("report_daily_limit")
		default:
			return nil, err
		}
	}
	dto := contentReportDTO(*report)
	return &dto, nil
}

func (s *ModerationService) ListReports(ctx context.Context, adminID string, filter repo.ReportListFilter) (*ContentReportListResp, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.moderation.ReleaseExpiredContentReportReviews(ctx, now); err != nil {
		return nil, err
	}
	filter.Page = normalizePage(filter.Page)
	filter.Size = normalizeSize(filter.Size)
	rows, total, stats, err := s.moderation.ListContentReportGroups(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ContentReportListResp{
		Items: contentReportGroupDTOs(rows),
		Total: total,
		Page:  filter.Page,
		Size:  filter.Size,
		Stats: ReportStatsDTO{
			Pending:   stats.Pending,
			Reviewing: stats.Reviewing,
			Today:     stats.Today,
			Total:     stats.Total,
		},
	}, nil
}

func (s *ModerationService) ReportDetail(ctx context.Context, adminID, id string) (*ContentReportDTO, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.moderation.ReleaseExpiredContentReportReviews(ctx, now); err != nil {
		return nil, err
	}
	report, err := s.moderation.GetContentReport(ctx, strings.TrimSpace(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report not found")
	}
	if err != nil {
		return nil, err
	}
	children, err := s.moderation.ContentReportsForGroup(ctx, *report)
	if err != nil {
		return nil, err
	}
	dto := contentReportDTO(*report)
	dto.ReportCount = int64(len(children))
	dto.RecentCount = recentReportCount(children, now.Add(-time.Hour))
	dto.Reports = contentReportDTOs(children)
	return &dto, nil
}

func (s *ModerationService) UpdateReport(ctx context.Context, adminID, id string, req UpdateReportReq) (*ContentReportDTO, error) {
	actorRole, err := s.requireContentModerator(ctx, adminID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.moderation.ReleaseExpiredContentReportReviews(ctx, now); err != nil {
		return nil, err
	}
	base, err := s.moderation.GetContentReport(ctx, strings.TrimSpace(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report not found")
	}
	if err != nil {
		return nil, err
	}
	if base.Status == model.ReportStatusResolved || base.Status == model.ReportStatusDismissed {
		return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
	}
	admin, err := s.moderation.UserProfile(ctx, adminID)
	if err != nil {
		return nil, err
	}
	policy, err := s.systemPolicy(ctx)
	if err != nil {
		return nil, err
	}
	reviewerName := firstNonEmptyString(admin.Name, admin.DisplayName, admin.Username, adminID)
	status := strings.ToLower(strings.TrimSpace(req.Status))
	actions, err := normalizeReportActions(req)
	if err != nil {
		return nil, err
	}
	if len(actions) > 0 {
		status = statusForReportActions(actions)
	} else {
		if _, ok := allowedReportStatuses[status]; !ok {
			return nil, errcode.New(http.StatusBadRequest, "invalid report status").WithReason("invalid_status")
		}
		if status == model.ReportStatusReviewing {
			actions = []string{model.ReportActionReview}
		} else if status == model.ReportStatusDismissed {
			actions = []string{model.ReportActionDismiss}
		}
	}
	var actionTarget *model.ContentReport
	if hasEnforcementAction(actions) {
		if err := ensureReportActionsFitTarget(base.TargetType, actions); err != nil {
			return nil, err
		}
		if actionTarget, err = s.verifiedReportTarget(ctx, base); err != nil {
			return nil, err
		}
		if err := s.ensureCanSanctionReportTarget(ctx, actorRole, actionTarget, actions); err != nil {
			return nil, err
		}
	}
	duration := policy.normalizeSanctionDuration(actions, req.DurationMinutes)
	note := trimRunes(strings.TrimSpace(req.Note), 1000)
	resolutionAction := strings.Join(actions, ",")
	reviewTTL := time.Duration(policy.ReportReviewTimeoutMinutes) * time.Minute

	if status == model.ReportStatusReviewing && len(actions) == 1 && actions[0] == model.ReportActionReview {
		report, err := s.moderation.ClaimContentReportGroup(ctx, strings.TrimSpace(id), adminID, reviewerName, now, reviewTTL)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(http.StatusNotFound, "report not found")
		}
		if errors.Is(err, repo.ErrReportAlreadyClosed) {
			return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
		}
		if errors.Is(err, repo.ErrReportClaimed) {
			return nil, errcode.New(http.StatusConflict, "report group claimed").WithReason("report_claimed")
		}
		if err != nil {
			return nil, err
		}
		_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, model.ReportActionReview, adminID, report.TargetType, report.ID, report.TargetTitle, firstNonEmptyString(report.TargetUserID, report.TargetOwnerID), firstNonEmptyString(report.TargetUserName, report.TargetOwnerName), note, now)
		dto := contentReportDTO(*report)
		return &dto, nil
	}

	if status == model.ReportStatusPending && len(actions) == 0 {
		report, err := s.moderation.UpdateContentReportGroup(ctx, strings.TrimSpace(id), status, "", adminID, reviewerName, note, 0, now)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(http.StatusNotFound, "report not found")
		}
		if errors.Is(err, repo.ErrReportAlreadyClosed) {
			return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
		}
		if errors.Is(err, repo.ErrReportClaimed) {
			return nil, errcode.New(http.StatusConflict, "report group claimed").WithReason("report_claimed")
		}
		if err != nil {
			return nil, err
		}
		dto := contentReportDTO(*report)
		return &dto, nil
	}

	claimed, err := s.moderation.ClaimContentReportGroup(ctx, strings.TrimSpace(id), adminID, reviewerName, now, reviewTTL)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report not found")
	}
	if errors.Is(err, repo.ErrReportAlreadyClosed) {
		return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
	}
	if errors.Is(err, repo.ErrReportClaimed) {
		return nil, errcode.New(http.StatusConflict, "report group claimed").WithReason("report_claimed")
	}
	if err != nil {
		return nil, err
	}
	if actionTarget != nil {
		// Keep the claimed row's id/status but act on the verified target.
		target := *claimed
		copyReportTarget(&target, actionTarget)
		claimed = &target
	}
	if err := s.applyReportActions(ctx, adminID, claimed, actions, note, duration); err != nil {
		return nil, err
	}
	report, err := s.moderation.UpdateContentReportGroup(ctx, strings.TrimSpace(id), status, resolutionAction, adminID, reviewerName, note, duration, now)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report not found")
	}
	if errors.Is(err, repo.ErrReportAlreadyClosed) {
		return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
	}
	if errors.Is(err, repo.ErrReportClaimed) {
		return nil, errcode.New(http.StatusConflict, "report group claimed").WithReason("report_claimed")
	}
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, status, adminID, report.TargetType, report.ID, report.TargetTitle, firstNonEmptyString(report.TargetUserID, report.TargetOwnerID), firstNonEmptyString(report.TargetUserName, report.TargetOwnerName), note, now)
	} else {
		for _, action := range actions {
			_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, action, adminID, report.TargetType, report.ID, report.TargetTitle, firstNonEmptyString(report.TargetUserID, report.TargetOwnerID), firstNonEmptyString(report.TargetUserName, report.TargetOwnerName), note, now)
		}
	}
	dto := contentReportDTO(*report)
	return &dto, nil
}

func normalizeReportActions(req UpdateReportReq) ([]string, error) {
	raw := req.Actions
	if len(raw) == 0 && strings.TrimSpace(req.Action) != "" {
		raw = []string{req.Action}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	actions := make([]string, 0, len(raw))
	for _, item := range raw {
		action := strings.ToLower(strings.TrimSpace(item))
		if action == "" {
			continue
		}
		if _, ok := allowedReportActions[action]; !ok {
			return nil, errcode.New(http.StatusBadRequest, "invalid report action").WithReason("invalid_action")
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		return nil, nil
	}
	if len(actions) > 1 {
		for _, action := range actions {
			if action == model.ReportActionDismiss || action == model.ReportActionReview {
				return nil, errcode.New(http.StatusBadRequest, "exclusive report action").WithReason("exclusive_action")
			}
		}
	}
	return actions, nil
}

// hasEnforcementAction reports whether actions touch content, users or rooms
// (anything beyond claiming or dismissing the report).
func hasEnforcementAction(actions []string) bool {
	for _, action := range actions {
		if action != model.ReportActionReview && action != model.ReportActionDismiss {
			return true
		}
	}
	return false
}

// ensureReportActionsFitTarget rejects room-level actions on reports about
// other content: a danmu report must not end the room it was posted in.
func ensureReportActionsFitTarget(targetType string, actions []string) error {
	if targetType == model.ReportTargetRoom {
		return nil
	}
	for _, action := range actions {
		if action == model.ReportActionWarnRoom || action == model.ReportActionForceEndLive {
			return errcode.New(http.StatusBadRequest, "action does not apply to this report target").WithReason("action_not_allowed_for_target")
		}
	}
	return nil
}

// verifiedReportTarget re-resolves the reported content so enforcement acts on
// its real author/room. When the content is gone we fall back to the stored
// snapshot only if it was resolved server-side at report time; legacy rows
// carry client-supplied ids and cannot drive sanctions.
func (s *ModerationService) verifiedReportTarget(ctx context.Context, report *model.ContentReport) (*model.ContentReport, error) {
	target, err := s.moderation.ResolveReportTarget(ctx, report.TargetType, report.TargetID, report.RoomID)
	if err == nil {
		out := *report
		out.RoomID = target.RoomID
		out.ChannelID = target.ChannelID
		out.TargetOwnerID = target.OwnerID
		out.TargetOwnerName = target.OwnerName
		out.TargetUserID = target.UserID
		out.TargetUserName = target.UserName
		out.TargetURL = target.Link
		out.TargetVerified = true
		return &out, nil
	}
	if !errors.Is(err, repo.ErrReportTargetNotFound) {
		return nil, err
	}
	if report.TargetVerified {
		return report, nil
	}
	return nil, errcode.New(http.StatusConflict, "report target can no longer be verified").WithReason("target_unverified")
}

// ensureCanSanctionReportTarget stops reports from being used against staff:
// admins are never sanctioned through reports, and only an admin may act on a
// platform moderator.
func (s *ModerationService) ensureCanSanctionReportTarget(ctx context.Context, actorRole string, report *model.ContentReport, actions []string) error {
	targets := make([]string, 0, 2)
	for _, action := range actions {
		switch action {
		case model.ReportActionWarnUser, model.ReportActionSiteMute, model.ReportActionBanUser:
			targets = append(targets, firstNonEmptyString(report.TargetUserID, report.TargetOwnerID))
		case model.ReportActionWarnRoom, model.ReportActionForceEndLive:
			targets = append(targets, report.TargetOwnerID)
		}
	}
	for _, userID := range targets {
		if userID == "" {
			continue
		}
		role, err := s.moderation.UserRole(ctx, userID)
		if err != nil {
			return err
		}
		switch {
		case role == repo.RoleAdmin:
			return errcode.New(http.StatusForbidden, "admins cannot be sanctioned through reports").WithReason("target_is_admin")
		case role == repo.RoleModerator && actorRole != repo.RoleAdmin:
			return errcode.New(http.StatusForbidden, "only admins can sanction moderators").WithReason("target_is_moderator")
		}
	}
	return nil
}

func copyReportTarget(dst, src *model.ContentReport) {
	dst.RoomID = src.RoomID
	dst.ChannelID = src.ChannelID
	dst.TargetOwnerID = src.TargetOwnerID
	dst.TargetOwnerName = src.TargetOwnerName
	dst.TargetUserID = src.TargetUserID
	dst.TargetUserName = src.TargetUserName
	dst.TargetURL = src.TargetURL
	dst.TargetVerified = src.TargetVerified
}

func (s *ModerationService) applyReportActions(ctx context.Context, adminID string, report *model.ContentReport, actions []string, note string, durationMinutes int) error {
	for _, action := range actions {
		if err := s.applyReportAction(ctx, adminID, report, action, note, durationMinutes); err != nil {
			return err
		}
	}
	return nil
}

func (s *ModerationService) applyReportAction(ctx context.Context, adminID string, report *model.ContentReport, action, note string, durationMinutes int) error {
	if report == nil || action == "" || action == model.ReportActionReview || action == model.ReportActionDismiss {
		return nil
	}
	now := s.now()
	targetUserID := firstNonEmptyString(report.TargetUserID, report.TargetOwnerID)
	targetUserName := firstNonEmptyString(report.TargetUserName, report.TargetOwnerName)
	targetLink := firstNonEmptyString(safeReportLink(report.TargetURL), reportLink(report))
	switch action {
	case model.ReportActionDeleteContent:
		if err := s.deleteReportedContent(ctx, adminID, report); err != nil {
			return err
		}
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_content_deleted", "内容已被删除", moderationDeletedBody(report), targetLink, adminID, now)
		}
	case model.ReportActionWarnUser:
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_warning", "你收到一条平台警告", moderationWarnBody(note), targetLink, adminID, now)
			return s.moderation.RecordUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionWarn, report.ID, note, 0, now)
		}
	case model.ReportActionWarnRoom:
		roomID := firstNonEmptyString(report.RoomID, report.TargetID)
		if s.live != nil && roomID != "" {
			_ = s.live.PublishSystemNotice(ctx, roomID, moderationRoomWarnBody(note))
		}
		if report.TargetOwnerID != "" {
			_ = s.notifyModeration(ctx, report.TargetOwnerID, "moderation_room_warning", "直播间收到平台警告", moderationRoomWarnBody(note), targetLink, adminID, now)
		}
	case model.ReportActionSiteMute:
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_site_mute", "你已被全站禁言", moderationMuteBody(durationMinutes, note), targetLink, adminID, now)
			if durationMinutes <= 0 {
				return errcode.New(http.StatusBadRequest, "invalid mute duration").WithReason("invalid_mute_duration")
			}
			mutedUntil := now.Add(time.Duration(durationMinutes) * time.Minute)
			if err := s.setUserRestriction(ctx, targetUserID, UserRestrictionUpdate{
				Action:     RestrictionMute,
				Reason:     note,
				MutedUntil: &mutedUntil,
				OperatorID: adminID,
			}); err != nil {
				return err
			}
			return s.moderation.RecordUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionSiteMute, report.ID, note, durationMinutes, now)
		}
	case model.ReportActionBanUser:
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_ban", "账号已被封禁", moderationBanBody(note), targetLink, adminID, now)
			// user-service owns the ban; its ban path also revokes the
			// user's refresh tokens.
			if err := s.setUserRestriction(ctx, targetUserID, UserRestrictionUpdate{
				Action:     RestrictionBan,
				Reason:     note,
				OperatorID: adminID,
			}); err != nil {
				return err
			}
			if err := s.moderation.RecordUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionBan, report.ID, note, 0, now); err != nil {
				return err
			}
			return s.endUserLiveRooms(ctx, targetUserID)
		}
	case model.ReportActionForceEndLive:
		roomID := firstNonEmptyString(report.RoomID, report.TargetID)
		if s.live != nil && roomID != "" {
			if err := s.live.ForceStopRoom(ctx, roomID); err != nil {
				return err
			}
		}
		if report.TargetOwnerID != "" {
			_ = s.notifyModeration(ctx, report.TargetOwnerID, "moderation_live_ended", "直播已被管理员结束", moderationForceEndBody(note), targetLink, adminID, now)
		}
	}
	return nil
}

// deleteReportedContent removes the reported content. Posts and comments
// live in room-service; chat messages and super chats are removed by the
// services that own them.
func (s *ModerationService) deleteReportedContent(ctx context.Context, adminID string, report *model.ContentReport) error {
	targetID := strings.TrimSpace(report.TargetID)
	if targetID == "" {
		return nil
	}
	switch report.TargetType {
	case model.ReportTargetDanmu:
		roomID := strings.TrimSpace(report.RoomID)
		if roomID == "" {
			return nil
		}
		if s.owners.Chat == nil {
			return errors.New("chat-service client is not configured")
		}
		return s.owners.Chat.HideChatMessage(ctx, roomID, targetID)
	case model.ReportTargetSuperChat:
		// Hidden, not failed: the payment stands and is not refunded here.
		if s.owners.SuperChats == nil {
			return errors.New("gift-service client is not configured")
		}
		return s.owners.SuperChats.ModerateSuperChat(ctx, targetID, adminID)
	default:
		return s.moderation.DeleteReportedContent(ctx, report.TargetType, targetID)
	}
}

func (s *ModerationService) setUserRestriction(ctx context.Context, userID string, update UserRestrictionUpdate) error {
	if s.owners.Users == nil {
		return errors.New("user-service client is not configured")
	}
	return s.owners.Users.SetUserRestriction(ctx, userID, update)
}

// endUserLiveRooms force-ends every active room owned by userID so a ban also
// takes the user off air instead of only blocking their next GoLive.
func (s *ModerationService) endUserLiveRooms(ctx context.Context, userID string) error {
	if s.live == nil || s.rooms == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	rooms, err := s.rooms.ActiveRoomsByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, room := range rooms {
		if err := s.live.ForceStopRoom(ctx, room.ID); err != nil {
			return err
		}
	}
	return nil
}

func contentReportDTOs(rows []model.ContentReport) []ContentReportDTO {
	out := make([]ContentReportDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, contentReportDTO(row))
	}
	return out
}

func contentReportGroupDTOs(rows []repo.ReportGroupRow) []ContentReportDTO {
	out := make([]ContentReportDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ContentReportDTO{
			ID:               row.ID,
			GroupID:          row.GroupID,
			TargetType:       row.TargetType,
			TargetID:         row.TargetID,
			TargetURL:        safeReportLink(row.TargetURL),
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
			ReviewerName:     row.ReviewerName,
			ResolutionAction: row.ResolutionAction,
			DurationMinutes:  row.DurationMinutes,
			ResolutionNote:   row.ResolutionNote,
			CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339),
			ReportCount:      row.ReportCount,
			RecentCount:      row.RecentCount,
		})
		if row.ResolvedAt != nil && !row.ResolvedAt.IsZero() {
			out[len(out)-1].ResolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339)
		}
		if row.ReviewStartedAt != nil && !row.ReviewStartedAt.IsZero() {
			out[len(out)-1].ReviewStartedAt = row.ReviewStartedAt.UTC().Format(time.RFC3339)
		}
		if row.ReviewExpiresAt != nil && !row.ReviewExpiresAt.IsZero() {
			out[len(out)-1].ReviewExpiresAt = row.ReviewExpiresAt.UTC().Format(time.RFC3339)
		}
	}
	return out
}

func contentReportDTO(row model.ContentReport) ContentReportDTO {
	dto := ContentReportDTO{
		ID:               row.ID,
		GroupID:          row.GroupID,
		ReporterID:       row.ReporterID,
		ReporterName:     row.ReporterName,
		ReporterAvatar:   row.ReporterAvatar,
		TargetType:       row.TargetType,
		TargetID:         row.TargetID,
		TargetURL:        safeReportLink(row.TargetURL),
		RoomID:           row.RoomID,
		ChannelID:        row.ChannelID,
		TargetOwnerID:    row.TargetOwnerID,
		TargetOwnerName:  row.TargetOwnerName,
		TargetUserID:     row.TargetUserID,
		TargetUserName:   row.TargetUserName,
		TargetTitle:      row.TargetTitle,
		TargetText:       row.TargetText,
		Reason:           row.Reason,
		Description:      row.Description,
		Status:           row.Status,
		ReviewerID:       row.ReviewerID,
		ReviewerName:     row.ReviewerName,
		ResolutionAction: row.ResolutionAction,
		DurationMinutes:  row.DurationMinutes,
		ResolutionNote:   row.ResolutionNote,
		CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.ResolvedAt != nil && !row.ResolvedAt.IsZero() {
		dto.ResolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339)
	}
	if row.ReviewStartedAt != nil && !row.ReviewStartedAt.IsZero() {
		dto.ReviewStartedAt = row.ReviewStartedAt.UTC().Format(time.RFC3339)
	}
	if row.ReviewExpiresAt != nil && !row.ReviewExpiresAt.IsZero() {
		dto.ReviewExpiresAt = row.ReviewExpiresAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func statusForReportActions(actions []string) string {
	if len(actions) == 0 {
		return model.ReportStatusReviewing
	}
	if len(actions) == 1 {
		switch actions[0] {
		case model.ReportActionReview:
			return model.ReportStatusReviewing
		case model.ReportActionDismiss:
			return model.ReportStatusDismissed
		}
	}
	return model.ReportStatusResolved
}

func (p adminSystemPolicy) normalizeSanctionDuration(actions []string, minutes int) int {
	hasSiteMute := false
	for _, action := range actions {
		if action == model.ReportActionSiteMute {
			hasSiteMute = true
			break
		}
	}
	if !hasSiteMute {
		return 0
	}
	if _, ok := allowedSiteMuteDurations[minutes]; ok {
		return minutes
	}
	return p.DefaultSiteMuteMinutes
}

func recentReportCount(rows []model.ContentReport, after time.Time) int64 {
	var count int64
	for _, row := range rows {
		if row.CreatedAt.After(after) {
			count++
		}
	}
	return count
}

// safeReportLink only lets relative in-app paths through. Legacy reports
// stored whatever URL the reporter sent, and the value ends up in admin links
// and official moderation notifications.
func safeReportLink(raw string) string {
	link := strings.TrimSpace(raw)
	if !strings.HasPrefix(link, "/") || strings.HasPrefix(link, "//") || strings.ContainsAny(link, "\\\r\n\t") {
		return ""
	}
	return link
}

func reportLink(report *model.ContentReport) string {
	if report == nil {
		return ""
	}
	switch report.TargetType {
	case model.ReportTargetRoom, model.ReportTargetDanmu, model.ReportTargetSuperChat:
		if report.RoomID != "" {
			return "/live/" + report.RoomID
		}
	case model.ReportTargetChannel, model.ReportTargetPost, model.ReportTargetPostComment:
		if report.ChannelID != "" {
			return "/channel/" + strings.TrimPrefix(report.ChannelID, "ch-")
		}
		if report.TargetOwnerID != "" {
			return "/channel/" + report.TargetOwnerID
		}
	}
	return ""
}

func moderationDeletedBody(report *model.ContentReport) string {
	switch report.TargetType {
	case model.ReportTargetPost:
		return "你的帖子因违反社区规范已被删除。"
	case model.ReportTargetPostComment:
		return "你的评论因违反社区规范已被删除。"
	case model.ReportTargetDanmu:
		return "你的弹幕因违反社区规范已被删除，回放中也不会继续显示。"
	case model.ReportTargetSuperChat:
		return "你的 SuperChat 因违反社区规范已被删除，回放中也不会继续显示。"
	default:
		return "你的内容因违反社区规范已被删除。"
	}
}

func moderationWarnBody(note string) string {
	if strings.TrimSpace(note) != "" {
		return "请注意平台社区规范。处理备注：" + strings.TrimSpace(note)
	}
	return "请注意平台社区规范，避免再次发布违规内容。"
}

func moderationRoomWarnBody(note string) string {
	if strings.TrimSpace(note) != "" {
		return "你的直播间收到平台警告。处理备注：" + strings.TrimSpace(note)
	}
	return "你的直播间收到平台警告，请及时调整直播内容。"
}

func moderationMuteBody(minutes int, note string) string {
	body := "你已被全站禁言，禁言期间无法发送弹幕、评论和 SuperChat。"
	if minutes > 0 {
		body = body + " 时长：" + (time.Duration(minutes) * time.Minute).String()
	}
	if strings.TrimSpace(note) != "" {
		body = body + " 处理备注：" + strings.TrimSpace(note)
	}
	return body
}

func moderationBanBody(note string) string {
	if strings.TrimSpace(note) != "" {
		return "你的账号已被封禁，可提交解封申请。处理备注：" + strings.TrimSpace(note)
	}
	return "你的账号已被封禁，可提交解封申请。"
}

func moderationForceEndBody(note string) string {
	if strings.TrimSpace(note) != "" {
		return "直播因违反社区规范已被管理员强制结束。处理备注：" + strings.TrimSpace(note)
	}
	return "直播因违反社区规范已被管理员强制结束。"
}
