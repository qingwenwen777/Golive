// Package repo persists chat events into sharded MySQL tables and reads them
// back for history.
//
// Sharding strategy:
//
//	tables   danmus_0 .. danmus_{N-1}
//	shard    fnv32(roomId) % N
//
// The shard count is fixed at boot from config (8 by default). Resharding
// later requires offline migration; we accept that for chat data which is
// effectively append-only and aggressively TTL'd.
package repo

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/userlevel"
)

type DanmuRepo struct {
	db     *gorm.DB
	shards int
}

type SuperChatHistoryRow struct {
	ID     string
	User   string
	Avatar string
	Amount int64
	Tier   int
	Text   string
	Ts     int64
}

type fanBadgeHistoryRow struct {
	UserID            string
	CreatorID         string
	TotalContribution int64
}

// UserProfile is a user's current public chat identity.
type UserProfile struct {
	Name   string
	Avatar string
	Level  int
}

func NewDanmuRepo(db *gorm.DB, shards int) *DanmuRepo {
	if shards <= 0 {
		shards = 8
	}
	return &DanmuRepo{db: db, shards: shards}
}

// Shards returns the configured shard count (so callers / tests can assert).
func (r *DanmuRepo) Shards() int { return r.shards }

// TableFor returns the shard table name for roomID.
func (r *DanmuRepo) TableFor(roomID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(roomID))
	return fmt.Sprintf("danmus_%d", int(h.Sum32())%r.shards)
}

// AutoMigrate creates every shard table.
func (r *DanmuRepo) AutoMigrate() error {
	for i := 0; i < r.shards; i++ {
		name := fmt.Sprintf("danmus_%d", i)
		if err := r.db.Table(name).AutoMigrate(&model.Danmu{}); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	return nil
}

func (r *DanmuRepo) Insert(ctx context.Context, d *model.Danmu) error {
	return r.db.WithContext(ctx).Table(r.TableFor(d.RoomID)).Create(d).Error
}

// History returns the most recent `limit` danmus before `before` (ms ts).
// Pass before=0 to mean "now".
func (r *DanmuRepo) History(ctx context.Context, roomID string, before int64, limit int) ([]model.Danmu, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Table(r.TableFor(roomID)).
		Where("room_id = ? AND deleted_at IS NULL", roomID)
	if before > 0 {
		q = q.Where("ts < ?", before)
	}
	var out []model.Danmu
	if err := q.Order("ts DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// FanBadgesForRoomUsers returns each user's current fan badge for the room
// owner, keyed by user id. History rendering uses the current membership
// state so viewers entering later still see badges earned while they were
// away. Badges are matched by user id only — display names are not unique
// and a name match let anyone inherit another user's badge.
func (r *DanmuRepo) FanBadgesForRoomUsers(ctx context.Context, roomID string, userIDs []string) (map[string]*model.FanBadgePayload, error) {
	out := map[string]*model.FanBadgePayload{}
	unique := uniqueIDs(userIDs)
	if roomID == "" || len(unique) == 0 {
		return out, nil
	}

	var rows []fanBadgeHistoryRow
	err := r.db.WithContext(ctx).
		Table("fan_badges AS fb").
		Select(`
fb.user_id AS user_id,
fb.creator_id AS creator_id,
fb.total_contribution AS total_contribution
`).
		Joins("JOIN rooms AS r ON r.owner_id = fb.creator_id").
		Where("r.id = ? AND fb.total_contribution > 0 AND fb.user_id IN ?", roomID, unique).
		Scan(&rows).Error
	if err != nil {
		// Older local/dev databases may not have gift fan-badge tables yet.
		// History should still load; it will simply omit badge decoration.
		if isMissingTable(err) {
			return out, nil
		}
		return out, err
	}

	for _, row := range rows {
		level := fanBadgeLevel(row.TotalContribution)
		if row.UserID == "" || row.CreatorID == "" || level <= 0 {
			continue
		}
		out[row.UserID] = &model.FanBadgePayload{
			CreatorID: row.CreatorID,
			Level:     level,
		}
	}
	return out, nil
}

// UserProfiles returns each user's current display name, avatar and user
// level (from top-ups, as user-service computes it), keyed by user id. Chat
// history renders these instead of the values stored with each message, so
// rows persisted before identity was derived server-side can't keep showing
// a client-chosen name or level.
func (r *DanmuRepo) UserProfiles(ctx context.Context, userIDs []string) (map[string]UserProfile, error) {
	out := map[string]UserProfile{}
	unique := uniqueIDs(userIDs)
	if len(unique) == 0 {
		return out, nil
	}
	var rows []struct {
		ID          string
		Username    string
		DisplayName string
		Avatar      string
		Topup       int64
	}
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
u.id AS id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(u.avatar, '') AS avatar,
COALESCE((SELECT SUM(ct.amount) FROM coin_transactions AS ct
  WHERE ct.user_id = u.id AND ct.type = 'topup' AND ct.amount > 0), 0) AS topup
`).
		Where("u.id IN ?", unique).
		Scan(&rows).Error
	if err != nil {
		if isMissingTable(err) {
			return out, nil
		}
		return out, err
	}
	for _, row := range rows {
		out[row.ID] = UserProfile{
			Name:   displayName(row.ID, row.DisplayName, row.Username),
			Avatar: row.Avatar,
			Level:  userlevel.LevelForTotalTopup(row.Topup),
		}
	}
	return out, nil
}

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// displayName mirrors the frontend's userDisplayName and im-gateway's live
// chat naming: display name, else a non-uuid username, else an id prefix.
func displayName(id, display, username string) string {
	if s := strings.TrimSpace(display); s != "" {
		return s
	}
	if s := strings.TrimSpace(username); s != "" && !uuidLike.MatchString(s) {
		return s
	}
	if len(id) > 8 {
		id = id[:8]
	}
	return "Creator " + id
}

func uniqueIDs(ids []string) []string {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func isMissingTable(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "no such table")
}

func fanBadgeLevel(totalContribution int64) int {
	if totalContribution <= 0 {
		return 1
	}
	level := int(totalContribution / 1000)
	if level < 1 {
		return 1
	}
	if level > 99 {
		return 99
	}
	return level
}

// SuperChatHistory returns successful SuperChats for the same room so the
// public history endpoint can restore paid messages when a viewer enters.
// Moderated ones (moderated_at set by gift-service) stay paid but hidden.
func (r *DanmuRepo) SuperChatHistory(ctx context.Context, roomID string, before int64, limit int) ([]SuperChatHistoryRow, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Table("super_chat_orders AS sc").
		Select(`
sc.order_id AS id,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), sc.user_id) AS user,
COALESCE(u.avatar, '') AS avatar,
sc.amount AS amount,
sc.tier AS tier,
sc.text AS text,
CAST(UNIX_TIMESTAMP(sc.created_at) * 1000 AS SIGNED) AS ts
`).
		Joins("LEFT JOIN users AS u ON u.id = sc.user_id").
		Where("sc.room_id = ? AND sc.status = ? AND sc.moderated_at IS NULL", roomID, "success")
	if before > 0 {
		q = q.Where("sc.created_at < FROM_UNIXTIME(?)", float64(before)/1000)
	}
	var out []SuperChatHistoryRow
	if err := q.Order("sc.created_at DESC").Limit(limit).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
