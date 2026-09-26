package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

var allowedSiteMuteDurations = map[int]struct{}{
	30:    {},
	120:   {},
	1440:  {},
	10080: {},
}

var allowedReviewTimeoutMinutes = map[int]struct{}{
	10: {},
	15: {},
	30: {},
	45: {},
	60: {},
}

const (
	defaultReportReviewTimeoutMinutes = 30
	defaultSiteMuteMinutes            = 1440
	systemSettingReportReviewTimeout  = "report_review_timeout_minutes"
	systemSettingDefaultSiteMute      = "default_site_mute_minutes"
)

type SystemRuntimeConfig struct {
	ServiceName         string
	LogLevel            string
	LiveFLVBase         string
	StreamKeyTTL        time.Duration
	ReplayRecordDir     string
	ReplayBunnyEnabled  bool
	ReplayUploadTimeout time.Duration
	CoverPublicURL      string
	PostPublicURL       string
}

func (s *ModerationService) SetSystemRuntimeConfig(cfg SystemRuntimeConfig) {
	s.runtime = cfg
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

type AdminSystemRuntimeItemDTO struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type AdminSystemSettingsResp struct {
	RegistrationPolicy         string                      `json:"registrationPolicy"`
	LiveReviewEnabled          bool                        `json:"liveReviewEnabled"`
	ContentPolicyLevel         string                      `json:"contentPolicyLevel"`
	ReportReviewTimeoutMinutes int                         `json:"reportReviewTimeoutMinutes"`
	SiteMuteDurations          []int                       `json:"siteMuteDurations"`
	DefaultSiteMuteMinutes     int                         `json:"defaultSiteMuteMinutes"`
	Runtime                    []AdminSystemRuntimeItemDTO `json:"runtime"`
	UpdatedBy                  string                      `json:"updatedBy,omitempty"`
	UpdatedAt                  string                      `json:"updatedAt,omitempty"`
}

type UpdateAdminSystemSettingsReq struct {
	ReportReviewTimeoutMinutes *int   `json:"reportReviewTimeoutMinutes"`
	DefaultSiteMuteMinutes     *int   `json:"defaultSiteMuteMinutes"`
	Note                       string `json:"note"`
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

func (s *ModerationService) AdminSystemSettings(ctx context.Context, adminID string) (*AdminSystemSettingsResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	policy, err := s.systemPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return s.adminSystemSettingsResp(policy), nil
}

func (s *ModerationService) UpdateAdminSystemSettings(ctx context.Context, adminID string, req UpdateAdminSystemSettingsReq) (*AdminSystemSettingsResp, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	current, err := s.systemPolicy(ctx)
	if err != nil {
		return nil, err
	}
	updates := map[string]string{}
	changes := make([]string, 0, 2)
	if req.ReportReviewTimeoutMinutes != nil {
		minutes := *req.ReportReviewTimeoutMinutes
		if _, ok := allowedReviewTimeoutMinutes[minutes]; !ok {
			return nil, errcode.New(http.StatusBadRequest, "invalid review timeout").WithReason("invalid_review_timeout")
		}
		if minutes != current.ReportReviewTimeoutMinutes {
			updates[systemSettingReportReviewTimeout] = strconv.Itoa(minutes)
			changes = append(changes, "review timeout "+strconv.Itoa(current.ReportReviewTimeoutMinutes)+"m -> "+strconv.Itoa(minutes)+"m")
		}
	}
	if req.DefaultSiteMuteMinutes != nil {
		minutes := *req.DefaultSiteMuteMinutes
		if _, ok := allowedSiteMuteDurations[minutes]; !ok {
			return nil, errcode.New(http.StatusBadRequest, "invalid default mute duration").WithReason("invalid_default_mute_duration")
		}
		if minutes != current.DefaultSiteMuteMinutes {
			updates[systemSettingDefaultSiteMute] = strconv.Itoa(minutes)
			changes = append(changes, "default site mute "+strconv.Itoa(current.DefaultSiteMuteMinutes)+"m -> "+strconv.Itoa(minutes)+"m")
		}
	}
	now := s.now()
	if len(updates) > 0 {
		if err := s.moderation.UpsertSystemSettings(ctx, updates, adminID, now); err != nil {
			return nil, err
		}
		note := strings.Join(changes, "; ")
		if trimmed := strings.TrimSpace(req.Note); trimmed != "" {
			if note != "" {
				note += "; "
			}
			note += "note: " + trimmed
		}
		_ = s.logAdminAudit(ctx, model.AdminAuditCategorySystem, "system_settings_update", adminID, "system_settings", "moderation_policy", "Moderation policy", "", "", note, now)
	}
	next, err := s.systemPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return s.adminSystemSettingsResp(next), nil
}

type adminSystemPolicy struct {
	ReportReviewTimeoutMinutes int
	DefaultSiteMuteMinutes     int
	UpdatedBy                  string
	UpdatedAt                  *time.Time
}

func (s *ModerationService) systemPolicy(ctx context.Context) (adminSystemPolicy, error) {
	rows, err := s.moderation.ListSystemSettings(ctx)
	if err != nil {
		return adminSystemPolicy{}, err
	}
	settings := make(map[string]model.SystemSetting, len(rows))
	policy := adminSystemPolicy{
		ReportReviewTimeoutMinutes: defaultReportReviewTimeoutMinutes,
		DefaultSiteMuteMinutes:     defaultSiteMuteMinutes,
	}
	for _, row := range rows {
		settings[row.Key] = row
		if row.UpdatedBy != "" && (policy.UpdatedAt == nil || row.UpdatedAt.After(*policy.UpdatedAt)) {
			updatedAt := row.UpdatedAt
			policy.UpdatedAt = &updatedAt
			policy.UpdatedBy = row.UpdatedBy
		}
	}
	policy.ReportReviewTimeoutMinutes = settingInt(settings, systemSettingReportReviewTimeout, defaultReportReviewTimeoutMinutes, allowedReviewTimeoutMinutes)
	policy.DefaultSiteMuteMinutes = settingInt(settings, systemSettingDefaultSiteMute, defaultSiteMuteMinutes, allowedSiteMuteDurations)
	return policy, nil
}

func settingInt(settings map[string]model.SystemSetting, key string, fallback int, allowed map[int]struct{}) int {
	row, ok := settings[key]
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(row.Value))
	if err != nil {
		return fallback
	}
	if len(allowed) > 0 {
		if _, ok := allowed[value]; !ok {
			return fallback
		}
	}
	return value
}

func (s *ModerationService) adminSystemSettingsResp(policy adminSystemPolicy) *AdminSystemSettingsResp {
	resp := &AdminSystemSettingsResp{
		RegistrationPolicy:         "invite_only",
		LiveReviewEnabled:          true,
		ContentPolicyLevel:         "standard",
		ReportReviewTimeoutMinutes: policy.ReportReviewTimeoutMinutes,
		SiteMuteDurations:          []int{30, 120, 1440, 10080},
		DefaultSiteMuteMinutes:     policy.DefaultSiteMuteMinutes,
		Runtime:                    s.systemRuntimeItems(),
		UpdatedBy:                  policy.UpdatedBy,
	}
	if policy.UpdatedAt != nil && !policy.UpdatedAt.IsZero() {
		resp.UpdatedAt = policy.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return resp
}

func (s *ModerationService) systemRuntimeItems() []AdminSystemRuntimeItemDTO {
	cfg := s.runtime
	items := []AdminSystemRuntimeItemDTO{
		{Key: "service", Label: "room-service", Value: firstNonEmptyString(cfg.ServiceName, "room-service"), Description: "Service owning moderation and system policy."},
		{Key: "log_level", Label: "Log level", Value: firstNonEmptyString(cfg.LogLevel, "-"), Description: "Runtime logging level from config."},
		{Key: "flv_base", Label: "HTTP-FLV base", Value: firstNonEmptyString(cfg.LiveFLVBase, "-"), Description: "Public playback prefix for live streams."},
		{Key: "stream_key_ttl", Label: "Stream key TTL", Value: durationValue(cfg.StreamKeyTTL), Description: "Publisher stream key validity window."},
		{Key: "replay_record_dir", Label: "Replay record dir", Value: firstNonEmptyString(cfg.ReplayRecordDir, "-"), Description: "Server-side replay recording path."},
		{Key: "replay_upload_timeout", Label: "Replay upload timeout", Value: durationValue(cfg.ReplayUploadTimeout), Description: "Maximum upload time for replay assets."},
		{Key: "bunny_stream", Label: "Bunny Stream", Value: configuredValue(cfg.ReplayBunnyEnabled), Description: "External replay hosting credentials are masked."},
		{Key: "cover_public_url", Label: "Cover uploads", Value: firstNonEmptyString(cfg.CoverPublicURL, "-"), Description: "Public URL prefix for live cover images."},
		{Key: "post_public_url", Label: "Post images", Value: firstNonEmptyString(cfg.PostPublicURL, "-"), Description: "Public URL prefix for post images."},
	}
	return items
}

func durationValue(value time.Duration) string {
	if value <= 0 {
		return "-"
	}
	return value.String()
}

func configuredValue(ok bool) string {
	if ok {
		return "configured"
	}
	return "not configured"
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
