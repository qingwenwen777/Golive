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
	UserID    string
	CreatorID string
	Level     int
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
		Where("room_id = ?", roomID)
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
func (r *DanmuRepo) FanBadgesForRoomUsers(ctx context.Context, roomID string, userIDs []string) (map[string]*model.FanBadgePayload, error) {
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
	if roomID == "" || len(unique) == 0 {
		return map[string]*model.FanBadgePayload{}, nil
	}

	var rows []fanBadgeHistoryRow
	err := r.db.WithContext(ctx).
		Table("fan_badges AS fb").
		Select("fb.user_id AS user_id, fb.creator_id AS creator_id, fb.level AS level").
		Joins("JOIN rooms AS r ON r.owner_id = fb.creator_id").
		Where("r.id = ? AND fb.user_id IN ? AND fb.level > 0", roomID, unique).
		Scan(&rows).Error
	if err != nil {
		// Older local/dev databases may not have gift fan-badge tables yet.
		// History should still load; it will simply omit badge decoration.
		msg := err.Error()
		if strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "no such table") {
			return map[string]*model.FanBadgePayload{}, nil
		}
		return nil, err
	}

	out := make(map[string]*model.FanBadgePayload, len(rows))
	for _, row := range rows {
		if row.UserID == "" || row.CreatorID == "" || row.Level <= 0 {
			continue
		}
		out[row.UserID] = &model.FanBadgePayload{
			CreatorID: row.CreatorID,
			Level:     row.Level,
		}
	}
	return out, nil
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
