package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var ErrModeratorNotFound = errors.New("moderator not found")

var (
	ErrReportDuplicate     = errors.New("report duplicate within 24h")
	ErrReportDailyLimit    = errors.New("daily report limit reached")
	ErrReportAlreadyClosed = errors.New("report group already handled")
	ErrReportClaimed       = errors.New("report group claimed by another reviewer")
	ErrBlockedWordExists   = errors.New("blocked word exists")
	ErrBlockedWordNotFound = errors.New("blocked word not found")
)

type ModerationRepo struct {
	db  *gorm.DB
	rdb *redis.Client
	// legacyTargetsGone holds ids of legacy reports whose content was gone
	// when VerifyLegacyReportTargets looked; deleted content does not return.
	legacyTargetsGone sync.Map
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
		&model.SystemSetting{},
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

func (r *ModerationRepo) CreateNotification(ctx context.Context, n model.Notification) error {
	if strings.TrimSpace(n.UserID) == "" {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error
}

// Platform roles, owned by user-service. A platform moderator only reviews
// content (reports, blocked words); everything else needs RoleAdmin.
const (
	RoleUser      = "user"
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
)

// UserRole returns the user's platform role, or "" for unknown users.
func (r *ModerationRepo) UserRole(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", nil
	}
	var role string
	err := r.db.WithContext(ctx).
		Table("users").
		Select("COALESCE(role, 'user')").
		Where("id = ?", userID).
		Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return role, err
}

func (r *ModerationRepo) IsAdmin(ctx context.Context, userID string) (bool, error) {
	role, err := r.UserRole(ctx, userID)
	return role == RoleAdmin, err
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil && !value.IsZero() {
			return value
		}
	}
	return nil
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
