package repo

import (
	"context"
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var ErrRoomNotFound = errors.New("room not found")

type RoomRepo struct {
	db *gorm.DB
}

func NewRoomRepo(db *gorm.DB) *RoomRepo { return &RoomRepo{db: db} }

func (r *RoomRepo) AutoMigrate() error {
	return r.db.AutoMigrate(&model.Room{})
}

// ListQuery is what the service layer hands to the repo. Empty Category means
// "no filter". The repo applies a case-insensitive match on category OR an
// exact match on category_ja.
type ListQuery struct {
	Category string
	Page     int
	Size     int
}

// List returns (items, total). Only owner-created live rooms are shown.
func (r *RoomRepo) List(ctx context.Context, q ListQuery) ([]model.Room, int64, error) {
	tx := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("status = ? AND owner_id <> ?", model.StatusLive, "")

	if q.Category != "" {
		// case-insensitive on category, exact on category_ja
		tx = tx.Where("LOWER(category) = ? OR category_ja = ?", strings.ToLower(q.Category), q.Category)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 {
		q.Size = 24
	}
	offset := (q.Page - 1) * q.Size

	var rooms []model.Room
	// Live first (already filtered), then newest start, then highest viewers.
	if err := tx.Order("started_at DESC, viewers DESC").
		Offset(offset).Limit(q.Size).Find(&rooms).Error; err != nil {
		return nil, 0, err
	}
	return rooms, total, nil
}

func (r *RoomRepo) GetByID(ctx context.Context, id string) (*model.Room, error) {
	var room model.Room
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&room).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) LatestByChannelIDs(ctx context.Context, channelIDs []string) (map[string]model.Room, error) {
	out := make(map[string]model.Room, len(channelIDs))
	if len(channelIDs) == 0 {
		return out, nil
	}

	var rooms []model.Room
	if err := r.db.WithContext(ctx).
		Where("channel_id IN ?", channelIDs).
		Order("updated_at DESC").
		Find(&rooms).Error; err != nil {
		return nil, err
	}

	sort.SliceStable(rooms, func(i, j int) bool {
		return roomStatusRank(rooms[i].Status) < roomStatusRank(rooms[j].Status)
	})
	for _, room := range rooms {
		if _, ok := out[room.ChannelID]; !ok {
			out[room.ChannelID] = room
		}
	}
	return out, nil
}

// Upsert inserts the room or updates the mutable fields if id already exists.
// Used by POST /rooms/live and admin/import flows.
func (r *RoomRepo) Upsert(ctx context.Context, room *model.Room) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "title_ja", "description", "category", "category_ja", "cover",
			"viewers", "started_at", "status", "ended_at", "updated_at",
			"stream_key", "owner_id", "channel", "channel_id", "avatar",
		}),
	}).Create(room).Error
}

const uuidPattern = "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"

// FixUUIDChannels rewrites legacy rows where rooms.channel was stored as a
// raw user UUID. Prefer the real display name from users; fall back to a
// short Creator label if the matching user row is not available.
// Runs once on startup; safe to invoke repeatedly.
func (r *RoomRepo) FixUUIDChannels(ctx context.Context) (int64, error) {
	joined := r.db.WithContext(ctx).Exec(`
UPDATE rooms AS r
JOIN users AS u ON u.id = r.owner_id
SET r.channel = COALESCE(NULLIF(u.display_name, ''), u.username),
    r.avatar = COALESCE(NULLIF(u.avatar, ''), r.avatar)
WHERE r.channel REGEXP ?
  AND COALESCE(NULLIF(u.display_name, ''), u.username) <> ''
`, uuidPattern)
	if joined.Error != nil {
		return joined.RowsAffected, joined.Error
	}

	fallback := r.db.WithContext(ctx).
		Model(&model.Room{}).
		Where("channel REGEXP ?", uuidPattern).
		Update("channel", gorm.Expr("CONCAT('Creator ', SUBSTRING(channel, 1, 8))"))
	return joined.RowsAffected + fallback.RowsAffected, fallback.Error
}

// SetPublishing marks a room as waiting for SRS on_publish.
func (r *RoomRepo) SetPublishing(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(map[string]any{
		"status":   model.StatusPublishing,
		"ended_at": nil,
	}).Error
}

// SetLive marks a room live with the given start time.
func (r *RoomRepo) SetLive(ctx context.Context, id string, startedAt any) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(map[string]any{
		"status":     model.StatusLive,
		"started_at": startedAt,
		"ended_at":   nil,
	}).Error
}

// SetEnded marks a room ended.
func (r *RoomRepo) SetEnded(ctx context.Context, id string, endedAt any) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(map[string]any{
		"status":   model.StatusEnded,
		"ended_at": endedAt,
	}).Error
}

func roomStatusRank(status string) int {
	switch status {
	case model.StatusLive:
		return 0
	case model.StatusPublishing:
		return 1
	case model.StatusEnding:
		return 2
	default:
		return 3
	}
}
