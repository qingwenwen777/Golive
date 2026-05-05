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
	"strings"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
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
	Username          string
	Name              string
	CreatorID         string
	TotalContribution int64
}

type FanBadgeLookup struct {
	ByUserID map[string]*model.FanBadgePayload
	ByName   map[string]*model.FanBadgePayload
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

// FanBadgesForRoomUsers returns each chat user's current fan badge for the
// room owner. History rendering uses the current membership state so viewers
// entering later still see badges earned while they were away.
func (r *DanmuRepo) FanBadgesForRoomUsers(ctx context.Context, roomID string, userIDs []string, names []string) (FanBadgeLookup, error) {
	empty := FanBadgeLookup{
		ByUserID: map[string]*model.FanBadgePayload{},
		ByName:   map[string]*model.FanBadgePayload{},
	}
	unique := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		unique = append(unique, userID)
	}
	uniqueNames := make([]string, 0, len(names))
	seenNames := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := normalizeBadgeName(name)
		if _, ok := seenNames[key]; ok {
			continue
		}
		seenNames[key] = struct{}{}
		uniqueNames = append(uniqueNames, name)
	}
	if roomID == "" || (len(unique) == 0 && len(uniqueNames) == 0) {
		return empty, nil
	}

	var rows []fanBadgeHistoryRow
	q := r.db.WithContext(ctx).
		Table("fan_badges AS fb").
		Select(`
fb.user_id AS user_id,
COALESCE(u.username, '') AS username,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), fb.user_id) AS name,
fb.creator_id AS creator_id,
fb.total_contribution AS total_contribution
`).
		Joins("JOIN rooms AS r ON r.owner_id = fb.creator_id").
		Joins("LEFT JOIN users AS u ON u.id = fb.user_id").
		Where("r.id = ? AND fb.total_contribution > 0", roomID)
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 3)
	if len(unique) > 0 {
		clauses = append(clauses, "fb.user_id IN ?")
		args = append(args, unique)
	}
	if len(uniqueNames) > 0 {
		clauses = append(clauses, "u.username IN ? OR u.display_name IN ?")
		args = append(args, uniqueNames, uniqueNames)
	}
	err := q.Where("("+strings.Join(clauses, " OR ")+")", args...).Scan(&rows).Error
	if err != nil {
		// Older local/dev databases may not have gift fan-badge tables yet.
		// History should still load; it will simply omit badge decoration.
		msg := err.Error()
		if strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "no such table") {
			return empty, nil
		}
		return empty, err
	}

	out := empty
	for _, row := range rows {
		level := fanBadgeLevel(row.TotalContribution)
		if row.UserID == "" || row.CreatorID == "" || level <= 0 {
			continue
		}
		badge := &model.FanBadgePayload{
			CreatorID: row.CreatorID,
			Level:     level,
		}
		out.ByUserID[row.UserID] = badge
		if row.Username != "" {
			out.ByName[normalizeBadgeName(row.Username)] = badge
		}
		if row.Name != "" {
			out.ByName[normalizeBadgeName(row.Name)] = badge
		}
	}
	return out, nil
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

func normalizeBadgeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// SuperChatHistory returns successful SuperChats for the same room so the
// public history endpoint can restore paid messages when a viewer enters.
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
		Where("sc.room_id = ? AND sc.status = ?", roomID, "success")
	if before > 0 {
		q = q.Where("sc.created_at < FROM_UNIXTIME(?)", float64(before)/1000)
	}
	var out []SuperChatHistoryRow
	if err := q.Order("sc.created_at DESC").Limit(limit).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
