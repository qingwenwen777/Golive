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
