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

const watchHeartbeatInterval = 30 * time.Second

type RoomRepo struct {
	db *gorm.DB
}

func NewRoomRepo(db *gorm.DB) *RoomRepo { return &RoomRepo{db: db} }

func (r *RoomRepo) AutoMigrate() error {
	if err := r.db.AutoMigrate(&model.Room{}, &model.RoomWatchEvent{}, &model.UserLibraryItem{}); err != nil {
		return err
	}
	if err := ensureMySQLIndex(
		r.db,
		"rooms",
		"idx_rooms_live_list",
		"CREATE INDEX idx_rooms_live_list ON rooms (status, started_at DESC, viewers DESC)",
	); err != nil {
		return err
	}
	return ensureMySQLFullTextIndexes(r.db, sharedSearchFullTextIndexes()...)
}

// maxListSize bounds one List page. RecommendedLive reads the largest page
// (its candidate pool); public endpoints are capped lower by the service.
const maxListSize = 200

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
	if q.Size > maxListSize {
		q.Size = maxListSize
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

// OwnerProfiles is OwnerProfile for many owners in one query, keyed by id.
// Owners without a users row are absent from the result.
func (r *RoomRepo) OwnerProfiles(ctx context.Context, ownerIDs []string) (map[string]OwnerProfile, error) {
	out := make(map[string]OwnerProfile, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return out, nil
	}
	var rows []OwnerProfile
	err := r.db.WithContext(ctx).
		Table("users").
		Select("id, username, display_name, avatar, verified").
		Where("id IN ?", ownerIDs).
		Scan(&rows).Error
	if isMissingTable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
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

// FanClubCreators returns which of creatorIDs userID holds a fan badge for:
// IsFanClubMember for many creators in one query.
func (r *RoomRepo) FanClubCreators(ctx context.Context, userID string, creatorIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	userID = strings.TrimSpace(userID)
	if userID == "" || len(creatorIDs) == 0 {
		return out, nil
	}
	var ids []string
	err := r.db.WithContext(ctx).
		Table("fan_badges").
		Where("user_id = ? AND creator_id IN ?", userID, creatorIDs).
		Pluck("creator_id", &ids).Error
	if isMissingTable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
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

// ResolveOwnerID maps a channel key to the owner's id: "ch-<id>" or a UUID
// as is, else a user's username or display name (see userIDByName), else a
// room's channel id or owner id, else a room's channel label when only one
// owner uses it.
func (r *RoomRepo) ResolveOwnerID(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrRoomNotFound
	}
	key = strings.TrimPrefix(key, "ch-")
	if IsUUIDLike(key) {
		return key, nil
	}

	ownerID, err := userIDByName(ctx, r.db, key)
	switch {
	case err == nil:
		return ownerID, nil
	case errors.Is(err, errUserNameAmbiguous):
		return "", ErrRoomNotFound
	case !errors.Is(err, ErrRoomNotFound) && !isMissingTable(err):
		return "", err
	}

	err = r.db.WithContext(ctx).Raw(`
SELECT owner_id FROM rooms
WHERE channel_id = ? OR owner_id = ?
ORDER BY updated_at DESC
LIMIT 1
`, key, key).Row().Scan(&ownerID)
	if err == nil && strings.TrimSpace(ownerID) != "" {
		return ownerID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	// Whoever goes live picks the room's channel label, so a label counts
	// only while a single owner uses it.
	var owners []string
	if err := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("channel = ? AND owner_id <> ?", key, "").
		Distinct("owner_id").Limit(2).
		Pluck("owner_id", &owners).Error; err != nil {
		return "", err
	}
	if len(owners) != 1 {
		return "", ErrRoomNotFound
	}
	return owners[0], nil
}

// errUserNameAmbiguous means several users have the display name a key names.
var errUserNameAmbiguous = errors.New("display name belongs to several users")

// userIDByName resolves a channel name to a user id. A username match always
// wins. Display names are not unique and anyone can take one, so a display
// name resolves only when exactly one user has it (errUserNameAmbiguous
// otherwise), never to whoever changed their profile last. ErrRoomNotFound
// means no user has the name.
func userIDByName(ctx context.Context, db *gorm.DB, name string) (string, error) {
	var ids []string
	if err := db.WithContext(ctx).Table("users").
		Where("username = ?", name).Limit(1).
		Pluck("id", &ids).Error; err != nil {
		return "", err
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if err := db.WithContext(ctx).Table("users").
		Where("display_name = ?", name).Limit(2).
		Pluck("id", &ids).Error; err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", ErrRoomNotFound
	case 1:
		return ids[0], nil
	default:
		return "", errUserNameAmbiguous
	}
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

// ReplayRecoverableUploads returns ended rooms whose replay upload was
// interrupted. Failed uploads are retried separately, a limited number of
// times (see ReplayRetryableUploads).
func (r *RoomRepo) ReplayRecoverableUploads(ctx context.Context, limit int) ([]model.Room, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	statuses := []string{
		model.ReplayStatusPending,
		model.ReplayStatusUploading,
	}
	var rooms []model.Room
	err := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("status = ? AND replay_upload_enabled = ? AND replay_status IN ?", model.StatusEnded, true, statuses).
		Order("COALESCE(ended_at, updated_at) ASC").
		Limit(limit).
		Find(&rooms).Error
	return rooms, err
}

// ReplayRetryableUploads returns ended rooms whose failed replay upload has
// retries left: the upload is still on, and a retry is scheduled or the
// failure predates retry scheduling (no attempts counted). A non-zero dueBy
// limits it to the retries due by then; oldest first.
func (r *RoomRepo) ReplayRetryableUploads(ctx context.Context, dueBy time.Time, limit int) ([]model.Room, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	tx := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("status = ? AND replay_upload_enabled = ? AND replay_status = ?", model.StatusEnded, true, model.ReplayStatusFailed)
	if dueBy.IsZero() {
		tx = tx.Where("(replay_retry_at IS NOT NULL OR replay_attempts = ?)", 0)
	} else {
		tx = tx.Where("(replay_retry_at <= ? OR (replay_retry_at IS NULL AND replay_attempts = ?))", dueBy, 0)
	}
	var rooms []model.Room
	err := tx.Order("replay_retry_at ASC, COALESCE(ended_at, updated_at) ASC").
		Limit(limit).
		Find(&rooms).Error
	return rooms, err
}

// RecordingRooms returns the rooms whose DVR recording may still be needed:
// active rooms, and ended rooms with replay upload on whose upload is pending,
// in progress, or failed and either has retries left (see
// ReplayRetryableUploads) or gave up after keepFailedSince, as such a
// recording is kept a while for a manual retry.
func (r *RoomRepo) RecordingRooms(ctx context.Context, keepFailedSince time.Time) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("status IN ? OR (status = ? AND replay_upload_enabled = ? AND (replay_status IN ? OR "+
			"(replay_status = ? AND (replay_retry_at IS NOT NULL OR replay_attempts = ? OR replay_failed_at >= ?))))",
			[]string{model.StatusPublishing, model.StatusLive, model.StatusEnding},
			model.StatusEnded, true,
			[]string{model.ReplayStatusPending, model.ReplayStatusUploading},
			model.ReplayStatusFailed, 0, keepFailedSince).
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

type UserLibraryRoom struct {
	Item model.UserLibraryItem
	Room model.Room
}

func (r *RoomRepo) RecordWatchEvent(ctx context.Context, event *model.RoomWatchEvent) error {
	if event == nil || event.ID == "" || event.UserID == "" || event.RoomID == "" {
		return nil
	}
	if event.WatchDate == "" {
		event.WatchDate = event.LastWatchedAt.UTC().Format("2006-01-02")
	}
	if event.DailyWatchCount <= 0 {
		event.DailyWatchCount = 1
	}
	if event.WatchCount <= 0 {
		event.WatchCount = 1
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.RoomWatchEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND room_id = ?", event.UserID, event.RoomID).
			Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(event).Error
		}
		if err != nil {
			return err
		}
		if event.LastWatchedAt.Before(existing.LastWatchedAt.Add(watchHeartbeatInterval)) {
			return nil
		}
		dailyWatchCount := any(gorm.Expr("daily_watch_count + 1"))
		if existing.WatchDate != event.WatchDate {
			dailyWatchCount = event.DailyWatchCount
		}
		return tx.Model(&existing).Updates(map[string]any{
			"channel_id":        event.ChannelID,
			"owner_id":          event.OwnerID,
			"category":          event.Category,
			"watch_count":       gorm.Expr("watch_count + 1"),
			"watch_date":        event.WatchDate,
			"daily_watch_count": dailyWatchCount,
			"last_watched_at":   event.LastWatchedAt,
			"updated_at":        event.UpdatedAt,
		}).Error
	})
}

func (r *RoomRepo) RemoveWatchEvent(ctx context.Context, userID, roomID string) error {
	if userID == "" || roomID == "" {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND room_id = ?", userID, roomID).
		Delete(&model.RoomWatchEvent{}).Error
}

func (r *RoomRepo) ClearWatchEvents(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.RoomWatchEvent{}).Error
}

func (r *RoomRepo) UpsertUserLibraryItem(ctx context.Context, item *model.UserLibraryItem) error {
	if item == nil || item.ID == "" || item.UserID == "" || item.Type == "" || item.RoomID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "type"}, {Name: "room_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"saved_at":   item.SavedAt,
			"updated_at": item.UpdatedAt,
		}),
	}).Create(item).Error
}

func (r *RoomRepo) MergeUserLibraryItem(ctx context.Context, item *model.UserLibraryItem) error {
	if item == nil || item.ID == "" || item.UserID == "" || item.Type == "" || item.RoomID == "" {
		return nil
	}
	var existing model.UserLibraryItem
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND room_id = ?", item.UserID, item.Type, item.RoomID).
		Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(item).Error
	}
	if err != nil {
		return err
	}
	if !item.SavedAt.After(existing.SavedAt) {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.UserLibraryItem{}).
		Where("id = ?", existing.ID).
		Updates(map[string]any{
			"saved_at":   item.SavedAt,
			"updated_at": item.UpdatedAt,
		}).Error
}

func (r *RoomRepo) ListUserLibraryRooms(ctx context.Context, userID, libraryType string, limit int) ([]UserLibraryRoom, error) {
	if userID == "" || libraryType == "" {
		return nil, nil
	}
	if limit < 1 || limit > 100 {
		limit = 60
	}
	var items []model.UserLibraryItem
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, libraryType).
		Order("saved_at DESC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RoomID)
	}
	var rooms []model.Room
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	roomsByID := make(map[string]model.Room, len(rooms))
	for _, room := range rooms {
		roomsByID[room.ID] = room
	}
	out := make([]UserLibraryRoom, 0, len(items))
	for _, item := range items {
		room, ok := roomsByID[item.RoomID]
		if !ok {
			continue
		}
		out = append(out, UserLibraryRoom{Item: item, Room: room})
	}
	return out, nil
}

func (r *RoomRepo) RemoveUserLibraryItem(ctx context.Context, userID, libraryType, roomID string) error {
	if userID == "" || libraryType == "" || roomID == "" {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND room_id = ?", userID, libraryType, roomID).
		Delete(&model.UserLibraryItem{}).Error
}

func (r *RoomRepo) ClearUserLibrary(ctx context.Context, userID, libraryType string) error {
	if userID == "" || libraryType == "" {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, libraryType).
		Delete(&model.UserLibraryItem{}).Error
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

// SetReplayStatus updates the replay status. A deleted replay stays deleted,
// so an upload still running for it cannot bring it back.
func (r *RoomRepo) SetReplayStatus(ctx context.Context, roomID, status, message string) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND replay_status <> ?", roomID, model.ReplayStatusDeleted).
		Updates(map[string]any{
			"replay_status": status,
			"replay_error":  message,
		}).Error
}

// SetReplayFailed records a failed replay upload attempt: the reason shown
// to the creator, the attempts failed so far, and when to retry (nil for no
// retry). A deleted replay stays deleted; it reports whether the row changed.
func (r *RoomRepo) SetReplayFailed(ctx context.Context, roomID, message string, attempts int, failedAt time.Time, retryAt *time.Time) (bool, error) {
	var retry any
	if retryAt != nil {
		retry = *retryAt
	}
	res := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND replay_status <> ?", roomID, model.ReplayStatusDeleted).
		Updates(map[string]any{
			"replay_status":    model.ReplayStatusFailed,
			"replay_error":     message,
			"replay_attempts":  attempts,
			"replay_failed_at": failedAt,
			"replay_retry_at":  retry,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ClaimReplayRetry moves a failed replay upload back to pending for a retry.
// It reports false when the replay changed since attempts was read: deleted,
// its upload turned off, or retried already.
func (r *RoomRepo) ClaimReplayRetry(ctx context.Context, roomID string, attempts int) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND status = ? AND replay_upload_enabled = ? AND replay_status = ? AND replay_attempts = ?",
			roomID, model.StatusEnded, true, model.ReplayStatusFailed, attempts).
		Updates(map[string]any{
			"replay_status":   model.ReplayStatusPending,
			"replay_error":    "",
			"replay_retry_at": nil,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// SetReplayUploaded records the uploaded video, but only while the upload is
// still wanted: the replay is pending or uploading and was not deleted. It
// reports whether the row was updated; false means the creator deleted the
// replay mid-upload and the video must not be published.
func (r *RoomRepo) SetReplayUploaded(ctx context.Context, roomID, libraryID, videoID, status string, uploadedAt time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND replay_status IN ? AND replay_deleted_at IS NULL", roomID,
			[]string{model.ReplayStatusPending, model.ReplayStatusUploading}).
		Updates(map[string]any{
			"replay_status":           status,
			"replay_bunny_library_id": libraryID,
			"replay_bunny_video_id":   videoID,
			"replay_uploaded_at":      uploadedAt,
			"replay_error":            "",
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
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

// RevenueTotals is a room's successful gift and super chat income.
type RevenueTotals struct {
	Gift      int64
	SuperChat int64
}

func (t RevenueTotals) Total() int64 { return t.Gift + t.SuperChat }

// RevenueTotalsByRooms sums successful orders per room in SQL, so callers
// that only need totals don't load every order row.
func (r *RoomRepo) RevenueTotalsByRooms(ctx context.Context, roomIDs []string) (map[string]RevenueTotals, error) {
	out := make(map[string]RevenueTotals, len(roomIDs))
	if len(roomIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RoomID string
		Kind   string
		Amount int64
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT room_id, 'gift' AS kind, COALESCE(SUM(total_coin), 0) AS amount
FROM gift_orders
WHERE status = 'success' AND room_id IN ?
GROUP BY room_id
UNION ALL
SELECT room_id, 'super_chat' AS kind, COALESCE(SUM(amount), 0) AS amount
FROM super_chat_orders
WHERE status = 'success' AND room_id IN ?
GROUP BY room_id
`, roomIDs, roomIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		totals := out[row.RoomID]
		if row.Kind == "super_chat" {
			totals.SuperChat += row.Amount
		} else {
			totals.Gift += row.Amount
		}
		out[row.RoomID] = totals
	}
	return out, nil
}

// TopFanRow is one fan's successful gift and super chat total in a room.
type TopFanRow struct {
	RoomID   string
	UserID   string
	UserName string
	Avatar   string
	Amount   int64
}

// TopFansByRooms returns each room's top limit fans by total spend, ranked in
// SQL (ties broken by user id), ordered by room then rank.
func (r *RoomRepo) TopFansByRooms(ctx context.Context, roomIDs []string, limit int) ([]TopFanRow, error) {
	if len(roomIDs) == 0 || limit < 1 {
		return nil, nil
	}
	var rows []TopFanRow
	err := r.db.WithContext(ctx).Raw(`
SELECT room_id, user_id, user_name, avatar, amount
FROM (
  SELECT
    t.room_id,
    t.user_id,
    COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), t.user_id) AS user_name,
    COALESCE(u.avatar, '') AS avatar,
    t.amount,
    ROW_NUMBER() OVER (PARTITION BY t.room_id ORDER BY t.amount DESC, t.user_id ASC) AS fan_rank
  FROM (
    SELECT room_id, user_id, SUM(amount) AS amount
    FROM (
      SELECT room_id, user_id, total_coin AS amount FROM gift_orders
      WHERE status = 'success' AND room_id IN ? AND user_id <> ''
      UNION ALL
      SELECT room_id, user_id, amount FROM super_chat_orders
      WHERE status = 'success' AND room_id IN ? AND user_id <> ''
    ) x
    GROUP BY room_id, user_id
  ) t
  LEFT JOIN users u ON u.id = t.user_id
) ranked
WHERE fan_rank <= ?
ORDER BY room_id, fan_rank
`, roomIDs, roomIDs, limit).Scan(&rows).Error
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

// SetLive marks a publishing (or ending) room live with the given start time.
// It reports whether this call made the transition; false means the room is
// already live or was ended meanwhile, and an ended room must stay ended.
func (r *RoomRepo) SetLive(ctx context.Context, id string, startedAt any) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND status IN ?", id, []string{model.StatusPublishing, model.StatusEnding}).
		Updates(map[string]any{
			"status":     model.StatusLive,
			"started_at": startedAt,
			"ended_at":   nil,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// SetEnded marks a room ended.
func (r *RoomRepo) SetEnded(ctx context.Context, id string, endedAt any) error {
	return r.db.WithContext(ctx).Model(&model.Room{}).Where("id = ?", id).Updates(map[string]any{
		"status":   model.StatusEnded,
		"ended_at": endedAt,
	}).Error
}

// SetEndedWithMetrics ends a publishing, live or ending room. It reports
// whether this call ended it, so only one of several concurrent stop paths
// runs the follow-up work (broadcast, replay upload).
func (r *RoomRepo) SetEndedWithMetrics(ctx context.Context, id string, endedAt any, viewers, peakViewers int64) (bool, error) {
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
	res := r.db.WithContext(ctx).Model(&model.Room{}).
		Where("id = ? AND status IN ?", id, []string{model.StatusPublishing, model.StatusLive, model.StatusEnding}).
		Updates(updates)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
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
