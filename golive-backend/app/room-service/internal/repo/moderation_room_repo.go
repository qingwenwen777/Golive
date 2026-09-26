package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ModerationRepo) ListFanClubMembers(ctx context.Context, ownerID, query string, page, size int) ([]ModerationUser, int64, error) {
	page, size = normalizeModerationPage(page, size)
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return []ModerationUser{}, 0, nil
	}
	q := r.db.WithContext(ctx).
		Table("fan_badges AS fb").
		Joins("JOIN users AS u ON u.id = fb.user_id").
		Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified,
CASE WHEN rm.user_id IS NULL THEN false ELSE true END AS moderator,
rm.created_at AS created_at
`).
		Joins("LEFT JOIN room_moderators AS rm ON rm.owner_id = ? AND rm.user_id = u.id AND rm.revoked_at IS NULL", ownerID).
		Where("fb.creator_id = ? AND fb.user_id <> ?", ownerID, ownerID)
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("LOWER(COALESCE(u.username, '')) LIKE ? OR LOWER(COALESCE(u.display_name, '')) LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		if isMissingTableName(err) {
			return []ModerationUser{}, 0, nil
		}
		return nil, 0, err
	}
	var rows []ModerationUser
	err := q.Order("moderator DESC, fb.updated_at DESC, u.updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if isMissingTableName(err) {
		return []ModerationUser{}, 0, nil
	}
	return rows, total, err
}

func (r *ModerationRepo) IsFanClubMember(ctx context.Context, userID, creatorID string) (bool, error) {
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
	if isMissingTableName(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ModerationRepo) ListModerators(ctx context.Context, ownerID string, page, size int) ([]ModerationUser, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).
		Table("room_moderators AS rm").
		Joins("JOIN users AS u ON u.id = rm.user_id").
		Where("rm.owner_id = ? AND rm.revoked_at IS NULL", ownerID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModerationUser
	err := q.Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified,
true AS moderator,
rm.created_at AS created_at
`).
		Order("rm.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) AddModerator(ctx context.Context, ownerID, actorID, targetUserID string, now time.Time) error {
	actor, err := r.UserProfile(ctx, actorID)
	if err != nil {
		return err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"created_by": actorID,
				"created_at": now,
				"updated_at": now,
				"revoked_at": nil,
			}),
		}).Create(&model.RoomModerator{
			OwnerID:   ownerID,
			UserID:    targetUserID,
			CreatedBy: actorID,
			CreatedAt: now,
			UpdatedAt: now,
			RevokedAt: nil,
		}).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, "", actor, target, model.ModeratorActionAdd, 0, now)).Error
	})
}

func (r *ModerationRepo) RemoveModerator(ctx context.Context, ownerID, actorID, targetUserID string, now time.Time) error {
	actor, err := r.UserProfile(ctx, actorID)
	if err != nil {
		return err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.RoomModerator{}).
			Where("owner_id = ? AND user_id = ? AND revoked_at IS NULL", ownerID, targetUserID).
			Updates(map[string]any{
				"revoked_at": now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrModeratorNotFound
		}
		return tx.Create(actionLog(ownerID, "", actor, target, model.ModeratorActionRemove, 0, now)).Error
	})
}

func (r *ModerationRepo) IsModerator(ctx context.Context, ownerID, userID string) (bool, error) {
	if ownerID == "" || userID == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.RoomModerator{}).
		Where("owner_id = ? AND user_id = ? AND revoked_at IS NULL", ownerID, userID).
		Count(&count).Error
	return count > 0, err
}

func (r *ModerationRepo) ActiveModeratorIDs(ctx context.Context, ownerID string) ([]string, error) {
	if ownerID == "" {
		return nil, nil
	}
	var ids []string
	err := r.db.WithContext(ctx).Model(&model.RoomModerator{}).
		Where("owner_id = ? AND revoked_at IS NULL", ownerID).
		Pluck("user_id", &ids).Error
	return ids, err
}

func (r *ModerationRepo) MuteUser(ctx context.Context, ownerID, roomID, operatorID, operatorRole, targetUserID, targetName, targetAvatar string, durationMinutes int, now time.Time) (*model.RoomMute, error) {
	actor, err := r.UserProfile(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if err == nil {
		targetName = target.Name
		targetAvatar = target.Avatar
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		target = ModerationUser{ID: targetUserID, Name: strings.TrimSpace(targetName), Avatar: strings.TrimSpace(targetAvatar)}
		if target.Name == "" {
			target.Name = targetUserID
		}
	} else {
		return nil, err
	}

	expiresAt := now.Add(time.Duration(durationMinutes) * time.Minute)
	mute := &model.RoomMute{
		ID:              uuid.NewString(),
		RoomID:          roomID,
		OwnerID:         ownerID,
		TargetUserID:    targetUserID,
		TargetName:      targetName,
		TargetAvatar:    targetAvatar,
		OperatorID:      operatorID,
		OperatorRole:    operatorRole,
		DurationMinutes: durationMinutes,
		StartedAt:       now,
		ExpiresAt:       expiresAt,
		CreatedAt:       now,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(mute).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, roomID, actor, target, model.ModeratorActionMute, durationMinutes, now)).Error
	}); err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if err := r.rdb.Set(ctx, roomMuteKey(roomID, targetUserID), expiresAt.UTC().Format(time.RFC3339), expiresAt.Sub(now)).Err(); err != nil {
			return nil, err
		}
	}
	return mute, nil
}

func (r *ModerationRepo) CurrentMute(ctx context.Context, roomID, targetUserID string, now time.Time) (*model.RoomMute, error) {
	if roomID == "" || targetUserID == "" {
		return nil, nil
	}
	var mute model.RoomMute
	err := r.db.WithContext(ctx).
		Where("room_id = ? AND target_user_id = ? AND expires_at > ?", roomID, targetUserID, now).
		Order("expires_at DESC").
		Take(&mute).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if ttl := mute.ExpiresAt.Sub(now); ttl > 0 {
			_ = r.rdb.Set(ctx, roomMuteKey(roomID, targetUserID), mute.ExpiresAt.UTC().Format(time.RFC3339), ttl).Err()
		}
	}
	return &mute, nil
}

func (r *ModerationRepo) UnmuteUser(ctx context.Context, ownerID, roomID, operatorID, targetUserID, targetName, targetAvatar string, now time.Time) (*model.RoomMute, error) {
	actor, err := r.UserProfile(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	current, err := r.CurrentMute(ctx, roomID, targetUserID, now)
	if err != nil {
		return nil, err
	}
	target, err := r.UserProfile(ctx, targetUserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		target = ModerationUser{ID: targetUserID, Name: strings.TrimSpace(targetName), Avatar: strings.TrimSpace(targetAvatar)}
		if target.Name == "" && current != nil {
			target.Name = current.TargetName
			target.Avatar = current.TargetAvatar
		}
		if target.Name == "" {
			target.Name = targetUserID
		}
	} else if err != nil {
		return nil, err
	}
	if current == nil {
		if r.rdb != nil {
			_ = r.rdb.Del(ctx, roomMuteKey(roomID, targetUserID)).Err()
		}
		return nil, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.RoomMute{}).
			Where("room_id = ? AND target_user_id = ? AND expires_at > ?", roomID, targetUserID, now).
			Update("expires_at", now).Error; err != nil {
			return err
		}
		return tx.Create(actionLog(ownerID, roomID, actor, target, model.ModeratorActionUnmute, 0, now)).Error
	})
	if err != nil {
		return nil, err
	}
	if r.rdb != nil {
		if err := r.rdb.Del(ctx, roomMuteKey(roomID, targetUserID)).Err(); err != nil {
			return nil, err
		}
	}
	current.ExpiresAt = now
	return current, nil
}

func (r *ModerationRepo) Logs(ctx context.Context, ownerID string, page, size int) ([]ModerationLogRow, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).Model(&model.ModeratorActionLog{}).Where("owner_id = ?", ownerID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ModerationLogRow
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) SyncRoomModerators(ctx context.Context, roomID, ownerID string) error {
	if r.rdb == nil || roomID == "" || ownerID == "" {
		return nil
	}
	ids, err := r.ActiveModeratorIDs(ctx, ownerID)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, roomOwnerKey(roomID), ownerID, 0)
	pipe.Del(ctx, roomModeratorsKey(roomID))
	if len(ids) > 0 {
		members := make([]any, 0, len(ids))
		for _, id := range ids {
			members = append(members, id)
		}
		pipe.SAdd(ctx, roomModeratorsKey(roomID), members...)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModerationRepo) SyncActiveRooms(ctx context.Context, rooms []model.Room) error {
	for _, room := range rooms {
		if err := r.SyncRoomModerators(ctx, room.ID, room.OwnerID); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModerationRepo) SyncActiveMutes(ctx context.Context, now time.Time) error {
	if r.rdb == nil {
		return nil
	}
	var mutes []model.RoomMute
	if err := r.db.WithContext(ctx).
		Where("expires_at > ?", now).
		Find(&mutes).Error; err != nil {
		return err
	}
	if len(mutes) == 0 {
		return nil
	}
	pipe := r.rdb.Pipeline()
	for _, mute := range mutes {
		ttl := mute.ExpiresAt.Sub(now)
		if ttl <= 0 {
			continue
		}
		pipe.Set(ctx, roomMuteKey(mute.RoomID, mute.TargetUserID), mute.ExpiresAt.UTC().Format(time.RFC3339), ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}
