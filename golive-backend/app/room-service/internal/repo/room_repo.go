package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

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
	return r.db.AutoMigrate(&model.Room{}, &model.RoomWatchEvent{})
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

type CreatorRecommendationCandidate struct {
	ID          string
	Username    string
	DisplayName string
	Avatar      string
	Verified    bool
	UpdatedAt   time.Time
	ChannelID   string
	Channel     string
	LastLiveAt  *time.Time
	LastTitle   string
	StreamCount int64
	PeakViewers int64
}

type OwnerProfile struct {
	ID          string
	Username    string
	DisplayName string
	Avatar      string
	Verified    bool
}

func (r *RoomRepo) OwnerProfile(ctx context.Context, ownerID string) (OwnerProfile, error) {
	var row OwnerProfile
	err := r.db.WithContext(ctx).
		Table("users").
		Select("id, username, display_name, avatar, verified").
		Where("id = ?", ownerID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return OwnerProfile{}, ErrRoomNotFound
	}
	if isMissingTable(err) {
		return OwnerProfile{}, ErrRoomNotFound
	}
	return row, err
}

func (r *RoomRepo) CreatorRecommendationCandidates(ctx context.Context, limit int, category string) ([]CreatorRecommendationCandidate, error) {
	if limit < 1 {
		limit = 50
	}
	actualLiveStatuses := []string{model.StatusLive, model.StatusEnded}
	category = strings.TrimSpace(category)
	lowerCategory := strings.ToLower(category)
	categoryClause := ""
	if category != "" {
		categoryClause = " AND (LOWER(r.category) = ? OR r.category_ja = ?)"
	}
	selectArgs := make([]any, 0, 18)
	addSubqueryArgs := func() {
		selectArgs = append(selectArgs, actualLiveStatuses)
		if category != "" {
			selectArgs = append(selectArgs, lowerCategory, category)
		}
	}
	for i := 0; i < 6; i++ {
		addSubqueryArgs()
	}
	selectSQL := fmt.Sprintf(`
			u.id,
			u.username,
			u.display_name,
			u.avatar,
			u.verified,
			u.updated_at,
			COALESCE((
				SELECT r.channel_id FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?%s
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			), '') AS channel_id,
			COALESCE((
				SELECT r.channel FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?%s
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			), '') AS channel,
			(
				SELECT r.started_at FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?%s
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			) AS last_live_at,
			COALESCE((
				SELECT r.title FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?%s
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			), '') AS last_title,
			(SELECT COUNT(*) FROM rooms r WHERE r.owner_id = u.id AND r.status IN ?%s) AS stream_count,
			COALESCE((SELECT MAX(r.peak_viewers) FROM rooms r WHERE r.owner_id = u.id AND r.status IN ?%s), 0) AS peak_viewers
		`, categoryClause, categoryClause, categoryClause, categoryClause, categoryClause, categoryClause)
	var rows []CreatorRecommendationCandidate
	tx := r.db.WithContext(ctx).
		Table("users AS u").
		Select(selectSQL, selectArgs...).
		Where("u.live_permission_status = ?", "approved")
	if category != "" {
		tx = tx.Where(`
			EXISTS (
				SELECT 1 FROM rooms r
				WHERE r.owner_id = u.id
					AND r.status IN ?
					AND (LOWER(r.category) = ? OR r.category_ja = ?)
			)
		`, actualLiveStatuses, lowerCategory, category)
	}
	err := tx.Order("u.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

// Upsert inserts the room or updates the mutable fields if id already exists.
// Used by POST /rooms/live and admin/import flows.
func (r *RoomRepo) Upsert(ctx context.Context, room *model.Room) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "title_ja", "description", "category", "category_ja", "cover",
			"viewers", "peak_viewers", "started_at", "status", "ended_at", "updated_at",
			"stream_key", "owner_id", "channel", "channel_id", "avatar",
			"fan_club_only",
			"replay_upload_enabled", "replay_status", "replay_visibility",
			"replay_bunny_video_id", "replay_bunny_library_id", "replay_error",
			"replay_uploaded_at", "replay_deleted_at",
		}),
	}).Create(room).Error
}

func (r *RoomRepo) IsFanClubMember(ctx context.Context, userID, creatorID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	creatorID = strings.TrimSpace(creatorID)
	if userID == "" || creatorID == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table("fan_badges").
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Count(&count).Error
	if isMissingTable(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RoomRepo) ActiveByOwner(ctx context.Context, ownerID string) (*model.Room, error) {
	var room model.Room
	err := r.db.WithContext(ctx).
		Where("owner_id = ? AND status IN ?", ownerID, []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Order("started_at DESC, created_at DESC").
		Take(&room).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) ActiveRoomsByOwner(ctx context.Context, ownerID string) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.WithContext(ctx).
		Where("owner_id = ? AND status IN ?", ownerID, []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Order("started_at DESC, created_at DESC").
		Find(&rooms).Error
	return rooms, err
}

func (r *RoomRepo) ActiveRooms(ctx context.Context) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.WithContext(ctx).
		Where("status IN ?", []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Order("started_at DESC, created_at DESC").
		Find(&rooms).Error
	return rooms, err
}

func (r *RoomRepo) UpdateMetadata(ctx context.Context, id, title, description, cover string) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(map[string]any{
		"title":       title,
		"description": description,
		"cover":       cover,
	}).Error
}

func (r *RoomRepo) EndActiveByOwner(ctx context.Context, ownerID string, endedAt time.Time) ([]model.Room, error) {
	var rooms []model.Room
	if err := r.db.WithContext(ctx).
		Where("owner_id = ? AND status IN ?", ownerID, []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	if len(rooms) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.ID)
	}
	if err := r.db.WithContext(ctx).Model(&model.Room{}).Where("id IN ?", ids).Updates(map[string]any{
		"status":   model.StatusEnded,
		"ended_at": endedAt,
	}).Error; err != nil {
		return nil, err
	}
	return rooms, nil
}

func (r *RoomRepo) ResolveOwnerID(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrRoomNotFound
	}
	if strings.HasPrefix(key, "ch-") {
		key = strings.TrimPrefix(key, "ch-")
	}
	if IsUUIDLike(key) {
		return key, nil
	}

	var ownerID string
	err := r.db.WithContext(ctx).Raw(`
SELECT id FROM users
WHERE username = ? OR display_name = ?
ORDER BY updated_at DESC
LIMIT 1
`, key, key).Row().Scan(&ownerID)
	if err == nil && strings.TrimSpace(ownerID) != "" {
		return ownerID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !isMissingTable(err) {
		return "", err
	}

	err = r.db.WithContext(ctx).Raw(`
SELECT owner_id FROM rooms
WHERE channel_id = ? OR owner_id = ? OR channel = ?
ORDER BY updated_at DESC
LIMIT 1
`, key, key, key).Row().Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(ownerID) == "" {
		return "", ErrRoomNotFound
	}
	if err != nil {
		return "", err
	}
	return ownerID, nil
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

func (r *RoomRepo) HistoryByOwner(ctx context.Context, ownerID string, page, size int) ([]model.Room, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 24
	}
	tx := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("owner_id = ? AND status = ?", ownerID, model.StatusEnded)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rooms []model.Room
	err := tx.
		Order("COALESCE(ended_at, updated_at) DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rooms).Error
	return rooms, total, err
}

func (r *RoomRepo) ReplayRoomsByOwner(ctx context.Context, ownerID string, page, size int) ([]model.Room, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 24
	}
	tx := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("owner_id = ? AND status = ? AND replay_status <> ?", ownerID, model.StatusEnded, model.ReplayStatusNone)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rooms []model.Room
	err := tx.
		Order("COALESCE(replay_uploaded_at, ended_at, updated_at) DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rooms).Error
	return rooms, total, err
}

func (r *RoomRepo) ReplayCandidateRoomsByOwner(ctx context.Context, ownerID string) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("owner_id = ? AND status = ? AND replay_status = ? AND replay_bunny_video_id <> ?", ownerID, model.StatusEnded, model.ReplayStatusReady, "").
		Order("COALESCE(replay_uploaded_at, ended_at, updated_at) DESC").
		Find(&rooms).Error
	return rooms, err
}

func (r *RoomRepo) HotReplayCandidates(ctx context.Context, since time.Time, limit int, category string) ([]model.Room, error) {
	if limit < 1 {
		limit = 100
	}
	tx := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("status = ? AND replay_status = ? AND replay_bunny_video_id <> ?", model.StatusEnded, model.ReplayStatusReady, "").
		Where("replay_visibility IN ?", []string{model.PostVisibilityPublic, model.PostVisibilityFollowers}).
		Where("COALESCE(ended_at, updated_at) >= ?", since)
	if category = strings.TrimSpace(category); category != "" {
		tx = tx.Where("LOWER(category) = ? OR category_ja = ?", strings.ToLower(category), category)
	}
	var rooms []model.Room
	err := tx.
		Order("COALESCE(ended_at, updated_at) DESC").
		Limit(limit).
		Find(&rooms).Error
	return rooms, err
}

type WatchCategoryRow struct {
	Category string
	Count    int64
}

type UserRevenuePreferenceRow struct {
	RoomID    string
	OwnerID   string
	ChannelID string
	Category  string
	Amount    int64
}

func (r *RoomRepo) RecordWatchEvent(ctx context.Context, event *model.RoomWatchEvent) error {
	if event == nil || event.ID == "" || event.UserID == "" || event.RoomID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"channel_id":      event.ChannelID,
			"owner_id":        event.OwnerID,
			"category":        event.Category,
			"last_watched_at": event.LastWatchedAt,
			"watch_count":     gorm.Expr("watch_count + 1"),
			"updated_at":      event.UpdatedAt,
		}),
	}).Create(event).Error
}

func (r *RoomRepo) WatchCategoryRows(ctx context.Context, userID string, since time.Time) ([]WatchCategoryRow, error) {
	if userID == "" {
		return nil, nil
	}
	var rows []WatchCategoryRow
	err := r.db.WithContext(ctx).
		Model(&model.RoomWatchEvent{}).
		Select("category, COALESCE(SUM(watch_count), 0) AS count").
		Where("user_id = ? AND last_watched_at >= ? AND category <> ?", userID, since, "").
		Group("category").
		Scan(&rows).Error
	return rows, err
}

func (r *RoomRepo) UserRevenuePreferenceRows(ctx context.Context, userID string, since time.Time) ([]UserRevenuePreferenceRow, error) {
	if userID == "" {
		return nil, nil
	}
	var rows []UserRevenuePreferenceRow
	err := r.db.WithContext(ctx).Raw(`
SELECT
  r.id AS room_id,
  r.owner_id,
  r.channel_id,
  r.category,
  COALESCE(SUM(x.amount), 0) AS amount
FROM (
  SELECT room_id, total_coin AS amount, created_at FROM gift_orders
  WHERE status = 'success' AND user_id = ? AND created_at >= ?
  UNION ALL
  SELECT room_id, amount AS amount, created_at FROM super_chat_orders
  WHERE status = 'success' AND user_id = ? AND created_at >= ?
) x
JOIN rooms r ON r.id = x.room_id
GROUP BY r.id, r.owner_id, r.channel_id, r.category
`, userID, since, userID, since).Scan(&rows).Error
	return rows, err
}

func (r *RoomRepo) EndedRoomByOwner(ctx context.Context, ownerID, roomID string) (*model.Room, error) {
	var room model.Room
	err := r.db.WithContext(ctx).
		Where("owner_id = ? AND id = ? AND status = ?", ownerID, roomID, model.StatusEnded).
		Take(&room).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) UpdateReplaySettings(ctx context.Context, id string, uploadAfterEnd bool, visibility, status string) (*model.Room, error) {
	updates := map[string]any{
		"replay_upload_enabled": uploadAfterEnd,
		"replay_visibility":     visibility,
		"replay_status":         status,
	}
	var room model.Room
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Room{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&room).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRoomNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) UpdateReplayVisibility(ctx context.Context, ownerID, roomID, visibility string) (*model.Room, error) {
	var room model.Room
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Room{}).
			Where("owner_id = ? AND id = ? AND status = ?", ownerID, roomID, model.StatusEnded).
			Update("replay_visibility", visibility)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrRoomNotFound
		}
		if err := tx.Where("owner_id = ? AND id = ? AND status = ?", ownerID, roomID, model.StatusEnded).Take(&room).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRoomNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) SetReplayStatus(ctx context.Context, roomID, status, message string) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", roomID).Updates(map[string]any{
		"replay_status": status,
		"replay_error":  message,
	}).Error
}

func (r *RoomRepo) SetReplayUploaded(ctx context.Context, roomID, libraryID, videoID, status string, uploadedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", roomID).Updates(map[string]any{
		"replay_status":           status,
		"replay_bunny_library_id": libraryID,
		"replay_bunny_video_id":   videoID,
		"replay_uploaded_at":      uploadedAt,
		"replay_deleted_at":       nil,
		"replay_error":            "",
	}).Error
}

func (r *RoomRepo) MarkReplayDeleted(ctx context.Context, roomID string, deletedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", roomID).Updates(map[string]any{
		"replay_upload_enabled":   false,
		"replay_status":           model.ReplayStatusDeleted,
		"replay_bunny_library_id": "",
		"replay_bunny_video_id":   "",
		"replay_error":            "",
		"replay_deleted_at":       deletedAt,
	}).Error
}

type RevenueRow struct {
	RoomID    string
	UserID    string
	UserName  string
	Avatar    string
	Amount    int64
	CreatedAt time.Time
	Kind      string
}

type FanBadgeDistributionRow struct {
	Bucket            string `gorm:"column:bucket"`
	FanCount          int64  `gorm:"column:fan_count"`
	TotalContribution int64  `gorm:"column:total_contribution"`
}

func (r *RoomRepo) RevenueRowsByRooms(ctx context.Context, roomIDs []string) ([]RevenueRow, error) {
	if len(roomIDs) == 0 {
		return nil, nil
	}
	var rows []RevenueRow
	err := r.db.WithContext(ctx).Raw(`
SELECT
  o.room_id,
  o.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), o.user_id) AS user_name,
  COALESCE(u.avatar, '') AS avatar,
  o.total_coin AS amount,
  o.created_at,
  'gift' AS kind
FROM gift_orders o
LEFT JOIN users u ON u.id = o.user_id
WHERE o.status = 'success' AND o.room_id IN ?
UNION ALL
SELECT
  s.room_id,
  s.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), s.user_id) AS user_name,
  COALESCE(u.avatar, '') AS avatar,
  s.amount AS amount,
  s.created_at,
  'super_chat' AS kind
FROM super_chat_orders s
LEFT JOIN users u ON u.id = s.user_id
WHERE s.status = 'success' AND s.room_id IN ?
`, roomIDs, roomIDs).Scan(&rows).Error
	return rows, err
}

func (r *RoomRepo) FanBadgeDistribution(ctx context.Context, creatorID string) ([]FanBadgeDistributionRow, error) {
	if creatorID == "" {
		return nil, nil
	}
	var rows []FanBadgeDistributionRow
	err := r.db.WithContext(ctx).Raw(`
SELECT
  CASE
    WHEN total_contribution >= 60000 THEN 'level60Plus'
    WHEN total_contribution >= 40000 THEN 'level40To59'
    WHEN total_contribution >= 20000 THEN 'level20To39'
    WHEN total_contribution > 0 THEN 'under20'
    ELSE 'under20'
  END AS bucket,
  COUNT(*) AS fan_count,
  COALESCE(SUM(total_contribution), 0) AS total_contribution
FROM fan_badges
WHERE creator_id = ? AND total_contribution > 0
GROUP BY bucket
`, creatorID).Scan(&rows).Error
	return rows, err
}

func (r *RoomRepo) RevenueRowsByOwnerSince(ctx context.Context, ownerID string, since time.Time) ([]RevenueRow, error) {
	var rows []RevenueRow
	err := r.db.WithContext(ctx).Raw(`
SELECT
  o.room_id,
  o.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), o.user_id) AS user_name,
  COALESCE(u.avatar, '') AS avatar,
  o.total_coin AS amount,
  o.created_at,
  'gift' AS kind
FROM gift_orders o
JOIN rooms r ON r.id = o.room_id
LEFT JOIN users u ON u.id = o.user_id
WHERE o.status = 'success' AND r.owner_id = ? AND o.created_at >= ?
UNION ALL
SELECT
  s.room_id,
  s.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), s.user_id) AS user_name,
  COALESCE(u.avatar, '') AS avatar,
  s.amount AS amount,
  s.created_at,
  'super_chat' AS kind
FROM super_chat_orders s
JOIN rooms r ON r.id = s.room_id
LEFT JOIN users u ON u.id = s.user_id
WHERE s.status = 'success' AND r.owner_id = ? AND s.created_at >= ?
`, ownerID, since, ownerID, since).Scan(&rows).Error
	return rows, err
}

const danmuShardCount = 8

func (r *RoomRepo) DanmuCountsByRooms(ctx context.Context, roomIDs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(roomIDs))
	if len(roomIDs) == 0 {
		return out, nil
	}
	type row struct {
		RoomID string
		Count  int64
	}
	for i := 0; i < danmuShardCount; i++ {
		table := fmt.Sprintf("danmus_%d", i)
		var rows []row
		err := r.db.WithContext(ctx).
			Table(table).
			Select("room_id, COUNT(*) AS count").
			Where("room_id IN ?", roomIDs).
			Group("room_id").
			Scan(&rows).Error
		if err != nil {
			if isMissingTable(err) {
				continue
			}
			return nil, err
		}
		for _, item := range rows {
			out[item.RoomID] += item.Count
		}
	}
	return out, nil
}

const uuidPattern = "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$"

var uuidPatternRe = regexp.MustCompile(uuidPattern)

func IsUUIDLike(s string) bool {
	return uuidPatternRe.MatchString(strings.TrimSpace(s))
}

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

func (r *RoomRepo) SetEndedWithMetrics(ctx context.Context, id string, endedAt any, viewers, peakViewers int64) error {
	updates := map[string]any{
		"status":   model.StatusEnded,
		"ended_at": endedAt,
	}
	if viewers >= 0 {
		updates["viewers"] = viewers
	}
	if peakViewers >= 0 {
		updates["peak_viewers"] = peakViewers
	}
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(updates).Error
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
