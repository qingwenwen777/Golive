package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	moderationRoleViewer    = "viewer"
	moderationRoleOwner     = "owner"
	moderationRoleModerator = "moderator"
)

var allowedMuteDurations = map[int]struct{}{
	5:  {},
	10: {},
	30: {},
	60: {},
}

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

var allowedSiteMuteDurations = map[int]struct{}{
	30:    {},
	120:   {},
	1440:  {},
	10080: {},
}

type ModerationService struct {
	moderation *repo.ModerationRepo
	rooms      *repo.RoomRepo
	social     *repo.SocialRepo
	live       *LiveService
	now        func() time.Time
}

func NewModerationService(moderation *repo.ModerationRepo, rooms *repo.RoomRepo, social *repo.SocialRepo) *ModerationService {
	return &ModerationService{
		moderation: moderation,
		rooms:      rooms,
		social:     social,
		now:        time.Now,
	}
}

func (s *ModerationService) SetLiveService(live *LiveService) {
	s.live = live
}

type ModerationUserDTO struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	Verified    bool   `json:"verified"`
	Moderator   bool   `json:"moderator"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type ModerationUserListResp struct {
	Items []ModerationUserDTO `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"size"`
}

type ModerationLogDTO struct {
	ID              string `json:"id"`
	OwnerID         string `json:"ownerId"`
	RoomID          string `json:"roomId,omitempty"`
	ActorID         string `json:"actorId"`
	ActorName       string `json:"actorName"`
	ActorAvatar     string `json:"actorAvatar,omitempty"`
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	TargetAvatar    string `json:"targetAvatar,omitempty"`
	Action          string `json:"action"`
	DurationMinutes int    `json:"durationMinutes,omitempty"`
	CreatedAt       string `json:"createdAt"`
}

type ModerationLogListResp struct {
	Items []ModerationLogDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
}

type RoomModerationState struct {
	RoomID               string `json:"roomId"`
	OwnerID              string `json:"ownerId"`
	Role                 string `json:"role"`
	CanModerate          bool   `json:"canModerate"`
	Muted                bool   `json:"muted"`
	MuteExpiresAt        string `json:"muteExpiresAt,omitempty"`
	MuteRemainingSeconds int64  `json:"muteRemainingSeconds,omitempty"`
}

type MuteReq struct {
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	TargetAvatar    string `json:"targetAvatar"`
	DurationMinutes int    `json:"durationMinutes"`
}

type MuteResp struct {
	RoomID          string `json:"roomId"`
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	DurationMinutes int    `json:"durationMinutes"`
	ExpiresAt       string `json:"expiresAt"`
}

type MuteStateResp struct {
	RoomID               string `json:"roomId"`
	TargetUserID         string `json:"targetUserId"`
	TargetName           string `json:"targetName,omitempty"`
	TargetAvatar         string `json:"targetAvatar,omitempty"`
	Muted                bool   `json:"muted"`
	MuteExpiresAt        string `json:"muteExpiresAt,omitempty"`
	MuteRemainingSeconds int64  `json:"muteRemainingSeconds,omitempty"`
}

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

type CreateBlockedWordReq struct {
	Word    string `json:"word"`
	Note    string `json:"note"`
	Enabled *bool  `json:"enabled"`
}

type BulkBlockedWordItem struct {
	Word string `json:"word"`
	Note string `json:"note"`
}

type BulkImportBlockedWordsReq struct {
	Items []BulkBlockedWordItem `json:"items"`
}

type BulkImportBlockedWordsResp struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

type UpdateBlockedWordReq struct {
	Word    *string `json:"word"`
	Note    *string `json:"note"`
	Enabled *bool   `json:"enabled"`
}

type BlockedWordDTO struct {
	ID        string `json:"id"`
	Word      string `json:"word"`
	Note      string `json:"note,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedBy string `json:"createdBy,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type BlockedWordListResp struct {
	Items []BlockedWordDTO `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

type AdminAuditLogDTO struct {
	ID             string `json:"id"`
	Category       string `json:"category"`
	Action         string `json:"action"`
	ActorID        string `json:"actorId"`
	ActorName      string `json:"actorName"`
	TargetType     string `json:"targetType,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	TargetTitle    string `json:"targetTitle,omitempty"`
	TargetUserID   string `json:"targetUserId,omitempty"`
	TargetUserName string `json:"targetUserName,omitempty"`
	Note           string `json:"note,omitempty"`
	Metadata       string `json:"metadata,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

type AdminAuditStatsDTO struct {
	Today      int64 `json:"today"`
	Review     int64 `json:"review"`
	Permission int64 `json:"permission"`
	System     int64 `json:"system"`
}

type AdminAuditLogListResp struct {
	Items []AdminAuditLogDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
	Stats AdminAuditStatsDTO `json:"stats"`
}

type AdminOverviewResp struct {
	OnlineRooms       int64            `json:"onlineRooms"`
	OnlineViewers     int64            `json:"onlineViewers"`
	TodayNewUsers     int64            `json:"todayNewUsers"`
	TodayRevenueCoins int64            `json:"todayRevenueCoins"`
	Health            []AdminHealthDTO `json:"health"`
}

type AdminHealthDTO struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Checked bool   `json:"checked"`
}

type CreateUnbanAppealReq struct {
	Reason string `json:"reason"`
}

func (s *ModerationService) ListFollowers(ctx context.Context, ownerID, query string, page, size int) (*ModerationUserListResp, error) {
	followerIDs, err := s.social.Followers(ctx, "ch-"+ownerID)
	if err != nil {
		return nil, err
	}
	followerIDs = excludeUserID(followerIDs, ownerID)
	items, total, err := s.moderation.ListFollowers(ctx, ownerID, query, page, size, followerIDs)
	if err != nil {
		return nil, err
	}
	return &ModerationUserListResp{
		Items: moderationUsers(items),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) ListModerators(ctx context.Context, ownerID string, page, size int) (*ModerationUserListResp, error) {
	items, total, err := s.moderation.ListModerators(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	return &ModerationUserListResp{
		Items: moderationUsers(items),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) AddModerator(ctx context.Context, ownerID, targetUserID string) (*ModerationUserDTO, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" || targetUserID == ownerID {
		return nil, errcode.New(400, "invalid moderator")
	}
	following, err := s.social.IsFollowing(ctx, targetUserID, "ch-"+ownerID)
	if err != nil {
		return nil, err
	}
	if !following {
		return nil, errcode.New(409, "moderator must be selected from your followers").WithReason("not_follower")
	}
	now := s.now()
	if err := s.moderation.AddModerator(ctx, ownerID, ownerID, targetUserID, now); err != nil {
		return nil, err
	}
	if err := s.syncOwnerActiveRooms(ctx, ownerID); err != nil {
		return nil, err
	}
	profile, err := s.moderation.UserProfile(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	profile.Moderator = true
	profile.CreatedAt = &now
	dto := moderationUser(profile)
	return &dto, nil
}

func (s *ModerationService) RemoveModerator(ctx context.Context, ownerID, targetUserID string) error {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" || targetUserID == ownerID {
		return errcode.New(400, "invalid moderator")
	}
	if err := s.moderation.RemoveModerator(ctx, ownerID, ownerID, targetUserID, s.now()); err != nil {
		if errors.Is(err, repo.ErrModeratorNotFound) {
			return errcode.New(404, "moderator not found")
		}
		return err
	}
	return s.syncOwnerActiveRooms(ctx, ownerID)
}

func (s *ModerationService) Logs(ctx context.Context, ownerID string, page, size int) (*ModerationLogListResp, error) {
	rows, total, err := s.moderation.Logs(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]ModerationLogDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, ModerationLogDTO{
			ID:              row.ID,
			OwnerID:         row.OwnerID,
			RoomID:          row.RoomID,
			ActorID:         row.ActorID,
			ActorName:       row.ActorName,
			ActorAvatar:     row.ActorAvatar,
			TargetUserID:    row.TargetUserID,
			TargetName:      row.TargetName,
			TargetAvatar:    row.TargetAvatar,
			Action:          row.Action,
			DurationMinutes: row.DurationMinutes,
			CreatedAt:       row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &ModerationLogListResp{Items: items, Total: total, Page: normalizePage(page), Size: normalizeSize(size)}, nil
}

func (s *ModerationService) AdminAuditLogs(ctx context.Context, adminID, category string, page, size int) (*AdminAuditLogListResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	rows, total, stats, err := s.moderation.ListAdminAuditLogs(ctx, category, page, size, s.now())
	if err != nil {
		return nil, err
	}
	items := make([]AdminAuditLogDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, AdminAuditLogDTO{
			ID:             row.ID,
			Category:       row.Category,
			Action:         row.Action,
			ActorID:        row.ActorID,
			ActorName:      row.ActorName,
			TargetType:     row.TargetType,
			TargetID:       row.TargetID,
			TargetTitle:    row.TargetTitle,
			TargetUserID:   row.TargetUserID,
			TargetUserName: row.TargetUserName,
			Note:           row.Note,
			Metadata:       row.Metadata,
			CreatedAt:      row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &AdminAuditLogListResp{
		Items: items,
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
		Stats: AdminAuditStatsDTO{
			Today:      stats.Today,
			Review:     stats.Review,
			Permission: stats.Permission,
			System:     stats.System,
		},
	}, nil
}

func (s *ModerationService) AdminOverview(ctx context.Context, adminID string) (*AdminOverviewResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	metrics, err := s.moderation.AdminDashboardMetrics(ctx, s.now())
	if err != nil {
		return nil, err
	}
	health := make([]AdminHealthDTO, 0, len(metrics.Health))
	for _, item := range metrics.Health {
		health = append(health, AdminHealthDTO{
			Key:     item.Key,
			Label:   item.Label,
			Status:  item.Status,
			Detail:  item.Detail,
			Checked: item.Checked,
		})
	}
	return &AdminOverviewResp{
		OnlineRooms:       metrics.OnlineRooms,
		OnlineViewers:     metrics.OnlineViewers,
		TodayNewUsers:     metrics.TodayNewUsers,
		TodayRevenueCoins: metrics.TodayRevenueCoins,
		Health:            health,
	}, nil
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
	reporter, err := s.moderation.UserProfile(ctx, reporterID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	report := &model.ContentReport{
		ID:              uuid.NewString(),
		ReporterID:      reporterID,
		ReporterName:    reporter.Name,
		ReporterAvatar:  reporter.Avatar,
		TargetType:      targetType,
		TargetID:        trimRunes(targetID, 128),
		TargetURL:       trimRunes(strings.TrimSpace(req.TargetURL), 800),
		RoomID:          trimRunes(strings.TrimSpace(req.RoomID), 64),
		ChannelID:       trimRunes(strings.TrimSpace(req.ChannelID), 64),
		TargetOwnerID:   trimRunes(strings.TrimSpace(req.TargetOwnerID), 36),
		TargetOwnerName: trimRunes(strings.TrimSpace(req.TargetOwnerName), 128),
		TargetUserID:    trimRunes(strings.TrimSpace(req.TargetUserID), 36),
		TargetUserName:  trimRunes(strings.TrimSpace(req.TargetUserName), 128),
		TargetTitle:     trimRunes(strings.TrimSpace(req.TargetTitle), 240),
		TargetText:      trimRunes(strings.TrimSpace(req.TargetText), 1000),
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
	if err := s.requireAdmin(ctx, adminID); err != nil {
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
	if err := s.requireAdmin(ctx, adminID); err != nil {
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
	dto.RecentCount = recentReportCount(children, s.now().Add(-time.Hour))
	dto.Reports = contentReportDTOs(children)
	return &dto, nil
}

func (s *ModerationService) UpdateReport(ctx context.Context, adminID, id string, req UpdateReportReq) (*ContentReportDTO, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
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
	duration := normalizeSanctionDuration(actions, req.DurationMinutes)
	note := trimRunes(strings.TrimSpace(req.Note), 1000)
	if err := s.applyReportActions(ctx, adminID, base, actions, note, duration); err != nil {
		return nil, err
	}
	now := s.now()
	resolutionAction := strings.Join(actions, ",")
	report, err := s.moderation.UpdateContentReportGroup(ctx, strings.TrimSpace(id), status, resolutionAction, adminID, note, duration, now)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.New(http.StatusNotFound, "report not found")
	}
	if errors.Is(err, repo.ErrReportAlreadyClosed) {
		return nil, errcode.New(http.StatusConflict, "report group already handled").WithReason("report_already_handled")
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
	targetLink := firstNonEmptyString(report.TargetURL, reportLink(report))
	switch action {
	case model.ReportActionDeleteContent:
		if err := s.moderation.DeleteReportedContent(ctx, report.TargetType, report.TargetID, report.RoomID, now); err != nil {
			return err
		}
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_content_deleted", "内容已被删除", moderationDeletedBody(report), targetLink, adminID, now)
		}
	case model.ReportActionWarnUser:
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_warning", "你收到一条平台警告", moderationWarnBody(note), targetLink, adminID, now)
			return s.moderation.ApplyUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionWarn, report.ID, note, 0, now)
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
			return s.moderation.ApplyUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionSiteMute, report.ID, note, durationMinutes, now)
		}
	case model.ReportActionBanUser:
		if targetUserID != "" {
			_ = s.notifyModeration(ctx, targetUserID, "moderation_ban", "账号已被封禁", moderationBanBody(note), targetLink, adminID, now)
			return s.moderation.ApplyUserSanction(ctx, targetUserID, targetUserName, adminID, model.UserSanctionBan, report.ID, note, 0, now)
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

func (s *ModerationService) ListBlockedWords(ctx context.Context, adminID string, page, size int) (*BlockedWordListResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	rows, total, err := s.moderation.ListBlockedWords(ctx, page, size)
	if err != nil {
		return nil, err
	}
	return &BlockedWordListResp{
		Items: blockedWordDTOs(rows),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) CreateBlockedWord(ctx context.Context, adminID string, req CreateBlockedWordReq) (*BlockedWordDTO, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	word := trimRunes(strings.TrimSpace(req.Word), 60)
	normalized := contentpolicy.NormalizeWord(word)
	if normalized == "" {
		return nil, errcode.New(http.StatusBadRequest, "blocked word is required").WithReason("word_required")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	now := s.now()
	row := &model.BlockedWord{
		ID:             uuid.NewString(),
		Word:           word,
		NormalizedWord: normalized,
		Note:           trimRunes(strings.TrimSpace(req.Note), 120),
		Enabled:        enabled,
		CreatedBy:      adminID,
		UpdatedBy:      adminID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.moderation.CreateBlockedWord(ctx, row); err != nil {
		if errors.Is(err, repo.ErrBlockedWordExists) {
			return nil, errcode.New(http.StatusConflict, "blocked word already exists").WithReason("word_exists")
		}
		return nil, err
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_create", adminID, "blocked_word", row.ID, row.Word, "", "", row.Note, now)
	dto := blockedWordDTO(*row)
	return &dto, nil
}

func (s *ModerationService) UpdateBlockedWord(ctx context.Context, adminID, id string, req UpdateBlockedWordReq) (*BlockedWordDTO, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"updated_by": adminID,
		"updated_at": s.now(),
	}
	if req.Word != nil {
		word := trimRunes(strings.TrimSpace(*req.Word), 60)
		if word == "" {
			return nil, errcode.New(http.StatusBadRequest, "blocked word is required").WithReason("word_required")
		}
		updates["word"] = word
		updates["normalized_word"] = contentpolicy.NormalizeWord(word)
	}
	if req.Note != nil {
		updates["note"] = trimRunes(strings.TrimSpace(*req.Note), 120)
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	row, err := s.moderation.UpdateBlockedWord(ctx, strings.TrimSpace(id), updates)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrBlockedWordExists):
			return nil, errcode.New(http.StatusConflict, "blocked word already exists").WithReason("word_exists")
		case errors.Is(err, repo.ErrBlockedWordNotFound):
			return nil, errcode.New(http.StatusNotFound, "blocked word not found")
		default:
			return nil, err
		}
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_update", adminID, "blocked_word", row.ID, row.Word, "", "", row.Note, s.now())
	dto := blockedWordDTO(*row)
	return &dto, nil
}

func (s *ModerationService) DeleteBlockedWord(ctx context.Context, adminID, id string) error {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return err
	}
	if err := s.moderation.DeleteBlockedWord(ctx, strings.TrimSpace(id)); err != nil {
		if errors.Is(err, repo.ErrBlockedWordNotFound) {
			return errcode.New(http.StatusNotFound, "blocked word not found")
		}
		return err
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_delete", adminID, "blocked_word", strings.TrimSpace(id), "", "", "", "", s.now())
	return nil
}

func (s *ModerationService) BulkImportBlockedWords(ctx context.Context, adminID string, req BulkImportBlockedWordsReq) (*BulkImportBlockedWordsResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errcode.New(http.StatusBadRequest, "no blocked words").WithReason("empty_import")
	}
	if len(req.Items) > 500 {
		return nil, errcode.New(http.StatusBadRequest, "too many blocked words").WithReason("too_many_items")
	}
	now := s.now()
	created := 0
	skipped := 0
	for _, item := range req.Items {
		word := trimRunes(strings.TrimSpace(item.Word), 60)
		normalized := contentpolicy.NormalizeWord(word)
		if normalized == "" {
			skipped++
			continue
		}
		row := &model.BlockedWord{
			ID:             uuid.NewString(),
			Word:           word,
			NormalizedWord: normalized,
			Note:           trimRunes(strings.TrimSpace(item.Note), 120),
			Enabled:        true,
			CreatedBy:      adminID,
			UpdatedBy:      adminID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.moderation.CreateBlockedWord(ctx, row); err != nil {
			if errors.Is(err, repo.ErrBlockedWordExists) {
				skipped++
				continue
			}
			return nil, err
		}
		created++
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_import", adminID, "blocked_word", "", "blocked words", "", "", "", now)
	return &BulkImportBlockedWordsResp{Created: created, Skipped: skipped}, nil
}

func (s *ModerationService) EnsureTextAllowed(ctx context.Context, texts ...string) error {
	if s == nil || s.moderation == nil {
		return nil
	}
	hit, err := s.moderation.BlockedWordHit(ctx, texts...)
	if err != nil {
		return err
	}
	if hit != "" {
		return errcode.New(http.StatusBadRequest, "content contains blocked word").WithReason("blocked_word")
	}
	return nil
}

func (s *ModerationService) EnsureUserCanInteract(ctx context.Context, userID string) error {
	if s == nil || s.moderation == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	restriction, err := s.moderation.UserRestriction(ctx, userID, s.now())
	if err != nil {
		return err
	}
	if restriction.Banned {
		return errcode.New(http.StatusForbidden, "user is banned").WithReason("user_banned")
	}
	if restriction.Muted {
		return errcode.New(http.StatusForbidden, "user is muted").WithReason("site_muted")
	}
	return nil
}

func (s *ModerationService) CreateUnbanAppeal(ctx context.Context, userID string, req CreateUnbanAppealReq) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	reason := trimRunes(strings.TrimSpace(req.Reason), 1000)
	if len([]rune(reason)) < 10 {
		return errcode.New(http.StatusBadRequest, "appeal reason is required").WithReason("appeal_reason_required")
	}
	restriction, err := s.moderation.UserRestriction(ctx, userID, s.now())
	if err != nil {
		return err
	}
	if !restriction.Banned {
		return errcode.New(http.StatusConflict, "user is not banned").WithReason("not_banned")
	}
	now := s.now()
	return s.moderation.CreateUnbanAppeal(ctx, &model.UnbanAppeal{
		ID:        uuid.NewString(),
		UserID:    userID,
		Reason:    reason,
		Status:    model.UnbanAppealPending,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *ModerationService) RoomState(ctx context.Context, roomID, userID string) (*RoomModerationState, error) {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	now := s.now()
	role := moderationRoleViewer
	if userID != "" && userID == room.OwnerID {
		role = moderationRoleOwner
	} else if userID != "" {
		ok, err := s.moderation.IsModerator(ctx, room.OwnerID, userID)
		if err != nil {
			return nil, err
		}
		if ok {
			role = moderationRoleModerator
		}
	}
	resp := &RoomModerationState{
		RoomID:      room.ID,
		OwnerID:     room.OwnerID,
		Role:        role,
		CanModerate: role == moderationRoleOwner || role == moderationRoleModerator,
	}
	if userID != "" {
		mute, err := s.moderation.CurrentMute(ctx, room.ID, userID, now)
		if err != nil {
			return nil, err
		}
		resp.Muted = mute != nil
		if mute != nil {
			resp.MuteExpiresAt = mute.ExpiresAt.UTC().Format(time.RFC3339)
			resp.MuteRemainingSeconds = int64(mute.ExpiresAt.Sub(now).Seconds())
			if resp.MuteRemainingSeconds < 0 {
				resp.MuteRemainingSeconds = 0
			}
		}
	}
	return resp, nil
}

func (s *ModerationService) Mute(ctx context.Context, roomID, actorID string, req MuteReq) (*MuteResp, error) {
	if actorID == "" {
		return nil, errcode.New(401, "Unauthorized")
	}
	if _, ok := allowedMuteDurations[req.DurationMinutes]; !ok {
		return nil, errcode.New(400, "duration must be one of 5, 10, 30, 60 minutes")
	}
	targetUserID := strings.TrimSpace(req.TargetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if targetUserID == room.OwnerID {
		return nil, errcode.New(409, "cannot mute the creator").WithReason("target_is_owner")
	}
	targetIsMod, err := s.moderation.IsModerator(ctx, room.OwnerID, targetUserID)
	if err != nil {
		return nil, err
	}
	if targetIsMod {
		return nil, errcode.New(409, "cannot mute a room moderator").WithReason("target_is_moderator")
	}

	role, err := s.moderationActorRole(ctx, room, actorID)
	if err != nil {
		return nil, err
	}
	if actorID == targetUserID {
		return nil, errcode.New(409, "cannot mute yourself")
	}

	mute, err := s.moderation.MuteUser(
		ctx,
		room.OwnerID,
		room.ID,
		actorID,
		role,
		targetUserID,
		strings.TrimSpace(req.TargetName),
		strings.TrimSpace(req.TargetAvatar),
		req.DurationMinutes,
		s.now(),
	)
	if err != nil {
		return nil, err
	}
	return &MuteResp{
		RoomID:          mute.RoomID,
		TargetUserID:    mute.TargetUserID,
		TargetName:      mute.TargetName,
		DurationMinutes: mute.DurationMinutes,
		ExpiresAt:       mute.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *ModerationService) MuteState(ctx context.Context, roomID, actorID, targetUserID string) (*MuteStateResp, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if _, err := s.moderationActorRole(ctx, room, actorID); err != nil {
		return nil, err
	}
	mute, err := s.moderation.CurrentMute(ctx, room.ID, targetUserID, s.now())
	if err != nil {
		return nil, err
	}
	return muteStateResp(room.ID, targetUserID, mute, s.now()), nil
}

func (s *ModerationService) Unmute(ctx context.Context, roomID, actorID, targetUserID string) (*MuteStateResp, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if _, err := s.moderationActorRole(ctx, room, actorID); err != nil {
		return nil, err
	}
	if actorID == targetUserID {
		return nil, errcode.New(409, "cannot unmute yourself")
	}
	if _, err := s.moderation.UnmuteUser(ctx, room.OwnerID, room.ID, actorID, targetUserID, "", "", s.now()); err != nil {
		return nil, err
	}
	return &MuteStateResp{
		RoomID:       room.ID,
		TargetUserID: targetUserID,
		Muted:        false,
	}, nil
}

func (s *ModerationService) syncOwnerActiveRooms(ctx context.Context, ownerID string) error {
	rooms, err := s.rooms.ActiveRoomsByOwner(ctx, ownerID)
	if err != nil {
		return err
	}
	return s.moderation.SyncActiveRooms(ctx, rooms)
}

func (s *ModerationService) moderationActorRole(ctx context.Context, room *model.Room, actorID string) (string, error) {
	if actorID == "" {
		return "", errcode.New(401, "Unauthorized")
	}
	if actorID == room.OwnerID {
		return moderationRoleOwner, nil
	}
	ok, err := s.moderation.IsModerator(ctx, room.OwnerID, actorID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errcode.New(403, "not allowed to moderate in this room")
	}
	return moderationRoleModerator, nil
}

func muteStateResp(roomID, targetUserID string, mute *model.RoomMute, now time.Time) *MuteStateResp {
	resp := &MuteStateResp{
		RoomID:       roomID,
		TargetUserID: targetUserID,
		Muted:        mute != nil,
	}
	if mute != nil {
		resp.TargetName = mute.TargetName
		resp.TargetAvatar = mute.TargetAvatar
		resp.MuteExpiresAt = mute.ExpiresAt.UTC().Format(time.RFC3339)
		resp.MuteRemainingSeconds = int64(mute.ExpiresAt.Sub(now).Seconds())
		if resp.MuteRemainingSeconds < 0 {
			resp.MuteRemainingSeconds = 0
		}
	}
	return resp
}

func excludeUserID(ids []string, userID string) []string {
	if userID == "" || len(ids) == 0 {
		return ids
	}
	out := ids[:0]
	for _, id := range ids {
		if id != userID {
			out = append(out, id)
		}
	}
	return out
}

func moderationUsers(rows []repo.ModerationUser) []ModerationUserDTO {
	out := make([]ModerationUserDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, moderationUser(row))
	}
	return out
}

func moderationUser(row repo.ModerationUser) ModerationUserDTO {
	dto := ModerationUserDTO{
		ID:          row.ID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Name:        row.Name,
		Avatar:      row.Avatar,
		Verified:    row.Verified,
		Moderator:   row.Moderator,
	}
	if row.CreatedAt != nil && !row.CreatedAt.IsZero() {
		dto.CreatedAt = row.CreatedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func (s *ModerationService) requireAdmin(ctx context.Context, userID string) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	ok, err := s.moderation.IsAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.New(http.StatusForbidden, "admin access required")
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
		Description:      row.Description,
		Status:           row.Status,
		ReviewerID:       row.ReviewerID,
		ResolutionAction: row.ResolutionAction,
		DurationMinutes:  row.DurationMinutes,
		ResolutionNote:   row.ResolutionNote,
		CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.ResolvedAt != nil && !row.ResolvedAt.IsZero() {
		dto.ResolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func blockedWordDTOs(rows []model.BlockedWord) []BlockedWordDTO {
	out := make([]BlockedWordDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, blockedWordDTO(row))
	}
	return out
}

func blockedWordDTO(row model.BlockedWord) BlockedWordDTO {
	return BlockedWordDTO{
		ID:        row.ID,
		Word:      row.Word,
		Note:      row.Note,
		Enabled:   row.Enabled,
		CreatedBy: row.CreatedBy,
		UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeSize(size int) int {
	if size < 1 {
		return 10
	}
	if size > 100 {
		return 100
	}
	return size
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

func statusForReportAction(action string) string {
	switch action {
	case model.ReportActionReview:
		return model.ReportStatusReviewing
	case model.ReportActionDismiss:
		return model.ReportStatusDismissed
	default:
		return model.ReportStatusResolved
	}
}

func normalizeSanctionDuration(actions []string, minutes int) int {
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
	return 1440
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

func (s *ModerationService) notifyModeration(ctx context.Context, userID, kind, title, body, link, adminID string, now time.Time) error {
	return s.moderation.CreateNotification(ctx, model.Notification{
		ID:        "mod-" + uuid.NewString(),
		UserID:    userID,
		Type:      kind,
		Title:     title,
		Body:      body,
		Link:      link,
		ActorID:   adminID,
		ActorName: "GoLive Admin",
		CreatedAt: now,
	})
}

func (s *ModerationService) logAdminAudit(ctx context.Context, category, action, actorID, targetType, targetID, targetTitle, targetUserID, targetUserName, note string, now time.Time) error {
	if strings.TrimSpace(actorID) == "" {
		return nil
	}
	return s.moderation.CreateAdminAuditLog(ctx, &model.AdminAuditLog{
		ID:             uuid.NewString(),
		Category:       category,
		Action:         action,
		ActorID:        actorID,
		TargetType:     trimRunes(strings.TrimSpace(targetType), 32),
		TargetID:       trimRunes(strings.TrimSpace(targetID), 128),
		TargetTitle:    trimRunes(strings.TrimSpace(targetTitle), 240),
		TargetUserID:   trimRunes(strings.TrimSpace(targetUserID), 36),
		TargetUserName: trimRunes(strings.TrimSpace(targetUserName), 128),
		Note:           trimRunes(strings.TrimSpace(note), 1000),
		CreatedAt:      now,
	})
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
