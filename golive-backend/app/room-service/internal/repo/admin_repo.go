package repo

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

func (r *ModerationRepo) ListSystemSettings(ctx context.Context) ([]model.SystemSetting, error) {
	var rows []model.SystemSetting
	err := r.db.WithContext(ctx).Order("updated_at DESC").Find(&rows).Error
	return rows, err
}

func (r *ModerationRepo) UpsertSystemSettings(ctx context.Context, values map[string]string, updatedBy string, now time.Time) error {
	if len(values) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			row := model.SystemSetting{
				Key:       strings.TrimSpace(key),
				Value:     strings.TrimSpace(value),
				UpdatedBy: strings.TrimSpace(updatedBy),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if row.Key == "" {
				continue
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "key"}},
				DoUpdates: clause.Assignments(map[string]any{
					"value":      row.Value,
					"updated_by": row.UpdatedBy,
					"updated_at": row.UpdatedAt,
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

func (r *ModerationRepo) AdminDashboardMetrics(ctx context.Context, now time.Time) (AdminDashboardMetrics, error) {
	metrics := AdminDashboardMetrics{
		Health: adminHTTPServiceHealth(ctx),
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
	metrics.Health = append(metrics.Health, adminTCPHealth(ctx, "kafka", "Kafka", "kafka:9092"))

	type roomRow struct {
		ID      string
		Viewers int64
	}
	var rooms []roomRow
	err := r.db.WithContext(ctx).
		Table("rooms").
		Select("id, viewers").
		Where("status = ?", model.StatusLive).
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

type adminHTTPHealthTarget struct {
	Key   string
	Label string
	URL   string
}

func adminHTTPServiceHealth(ctx context.Context) []AdminHealthItem {
	targets := []adminHTTPHealthTarget{
		{Key: "room-service", Label: "room-service", URL: "http://127.0.0.1:8091/healthz"},
		{Key: "api-gateway", Label: "api-gateway", URL: "http://api-gateway:8080/healthz"},
		{Key: "user-service", Label: "user-service", URL: "http://user-service:8090/healthz"},
		{Key: "gift-service", Label: "gift-service", URL: "http://gift-service:8092/healthz"},
		{Key: "chat-service", Label: "chat-service", URL: "http://chat-service:8093/healthz"},
		{Key: "im-gateway", Label: "im-gateway", URL: "http://im-gateway:8081/healthz"},
	}
	items := make([]AdminHealthItem, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		i, target := i, target
		wg.Add(1)
		go func() {
			defer wg.Done()
			items[i] = adminHTTPHealth(ctx, target)
		}()
	}
	wg.Wait()
	return items
}

func adminHTTPHealth(ctx context.Context, target adminHTTPHealthTarget) AdminHealthItem {
	checkCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		return AdminHealthItem{Key: target.Key, Label: target.Label, Status: "down", Detail: err.Error(), Checked: true}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return AdminHealthItem{Key: target.Key, Label: target.Label, Status: "down", Detail: compactAdminHealthError(err), Checked: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusInternalServerError {
		return AdminHealthItem{Key: target.Key, Label: target.Label, Status: "down", Detail: resp.Status, Checked: true}
	}
	return AdminHealthItem{Key: target.Key, Label: target.Label, Status: "ok", Detail: "HTTP /healthz responding", Checked: true}
}

func adminTCPHealth(ctx context.Context, key, label, addr string) AdminHealthItem {
	checkCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", addr)
	if err != nil {
		return AdminHealthItem{Key: key, Label: label, Status: "down", Detail: compactAdminHealthError(err), Checked: true}
	}
	_ = conn.Close()
	return AdminHealthItem{Key: key, Label: label, Status: "ok", Detail: "TCP port reachable", Checked: true}
}

func compactAdminHealthError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "Health check timed out"
	}
	msg := err.Error()
	const maxLen = 160
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}
