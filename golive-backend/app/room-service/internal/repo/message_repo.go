package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var (
	ErrDirectThreadNotFound   = errors.New("direct thread not found")
	ErrAwaitingCreatorReply   = errors.New("awaiting creator reply")
	ErrFanGroupNotFound       = errors.New("fan group not found")
	ErrFanGroupMemberNotFound = errors.New("fan group member not found")
	ErrFanGroupMuted          = errors.New("fan group member muted")
)

type MessageRepo struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) *MessageRepo { return &MessageRepo{db: db} }

func (r *MessageRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.DirectThread{},
		&model.DirectMessage{},
		&model.UserBlock{},
		&model.MessagePreference{},
		&model.FanGroupChat{},
		&model.FanGroupMember{},
		&model.FanGroupMessage{},
	)
}

type MessageUserProfile struct {
	ID                   string
	Username             string
	DisplayName          string
	Name                 string
	Avatar               string
	Verified             bool
	LivePermissionStatus string
}

type DirectSendInput struct {
	ViewerID   string
	CreatorID  string
	ChannelID  string
	SenderID   string
	ReceiverID string
	Body       string
	Now        time.Time
}

type ThreadOptionUpdate struct {
	Pinned       *bool
	Muted        *bool
	PushDisabled *bool
}

type FanGroupMemberRow struct {
	GroupID     string
	UserID      string
	Role        string
	MutedUntil  *time.Time
	KickedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Username    string
	DisplayName string
	Name        string
	Avatar      string
	Verified    bool
}

type FanGroupWithMembers struct {
	Group   model.FanGroupChat
	Members []FanGroupMemberRow
}

type FanGroupMessageRow struct {
	ID          string
	GroupID     string
	SenderID    string
	Body        string
	CreatedAt   time.Time
	Username    string
	DisplayName string
	Name        string
	Avatar      string
	Verified    bool
}

func (r *MessageRepo) UserProfile(ctx context.Context, userID string) (MessageUserProfile, error) {
	var row MessageUserProfile
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified,
COALESCE(u.live_permission_status, 'none') AS live_permission_status
`).
		Where("u.id = ?", userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableName(err) {
		return MessageUserProfile{}, ErrRoomNotFound
	}
	return row, err
}

func (r *MessageRepo) IsApprovedCreator(ctx context.Context, userID string) (bool, error) {
	profile, err := r.UserProfile(ctx, userID)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(profile.LivePermissionStatus, "approved"), nil
}

func (r *MessageRepo) CreateNotification(ctx context.Context, n model.Notification) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error
}

func (r *MessageRepo) CreateNotifications(ctx context.Context, notifications []model.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&notifications).Error
}

func (r *MessageRepo) ThreadByViewerCreator(ctx context.Context, viewerID, creatorID string) (*model.DirectThread, error) {
	var thread model.DirectThread
	err := r.db.WithContext(ctx).
		Where("viewer_id = ? AND creator_id = ?", viewerID, creatorID).
		Take(&thread).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDirectThreadNotFound
	}
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

func (r *MessageRepo) DirectThreadForUser(ctx context.Context, threadID, userID string) (*model.DirectThread, error) {
	var thread model.DirectThread
	err := r.db.WithContext(ctx).
		Where("id = ? AND (viewer_id = ? OR creator_id = ?)", threadID, userID, userID).
		Take(&thread).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDirectThreadNotFound
	}
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

func (r *MessageRepo) ListDirectThreads(ctx context.Context, userID string, page, size int) ([]model.DirectThread, int64, error) {
	page, size = normalizeMessagePage(page, size)
	tx := r.db.WithContext(ctx).Model(&model.DirectThread{}).
		Where("viewer_id = ? OR creator_id = ?", userID, userID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.DirectThread
	err := tx.Order("last_message_at DESC, updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *MessageRepo) SendDirectMessage(ctx context.Context, input DirectSendInput) (*model.DirectThread, *model.DirectMessage, error) {
	var thread model.DirectThread
	var message model.DirectMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("viewer_id = ? AND creator_id = ?", input.ViewerID, input.CreatorID).
			Take(&thread).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			thread = model.DirectThread{
				ID:        uuid.NewString(),
				ViewerID:  input.ViewerID,
				CreatorID: input.CreatorID,
				ChannelID: input.ChannelID,
				CreatedAt: input.Now,
				UpdatedAt: input.Now,
			}
			if err := tx.Create(&thread).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if input.SenderID == input.ViewerID && thread.LastSenderID == input.ViewerID {
			return ErrAwaitingCreatorReply
		}

		message = model.DirectMessage{
			ID:         uuid.NewString(),
			ThreadID:   thread.ID,
			SenderID:   input.SenderID,
			ReceiverID: input.ReceiverID,
			Body:       input.Body,
			CreatedAt:  input.Now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}

		updates := map[string]any{
			"channel_id":           input.ChannelID,
			"last_message_id":      message.ID,
			"last_message_preview": trimDBPreview(input.Body),
			"last_sender_id":       input.SenderID,
			"last_message_at":      input.Now,
			"updated_at":           input.Now,
		}
		if input.SenderID == thread.ViewerID {
			updates["creator_unread"] = gorm.Expr("creator_unread + 1")
			updates["viewer_archived_at"] = nil
		} else {
			updates["viewer_unread"] = gorm.Expr("viewer_unread + 1")
			updates["creator_archived_at"] = nil
		}
		if err := tx.Model(&model.DirectThread{}).Where("id = ?", thread.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", thread.ID).Take(&thread).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return &thread, &message, nil
}

func (r *MessageRepo) DirectMessages(ctx context.Context, threadID, userID string, page, size int) ([]model.DirectMessage, int64, error) {
	page, size = normalizeMessagePage(page, size)
	if _, err := r.DirectThreadForUser(ctx, threadID, userID); err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Model(&model.DirectMessage{}).Where("thread_id = ?", threadID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.DirectMessage
	err := tx.Order("created_at ASC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *MessageRepo) MarkDirectThreadRead(ctx context.Context, threadID, userID string) error {
	thread, err := r.DirectThreadForUser(ctx, threadID, userID)
	if err != nil {
		return err
	}
	updates := map[string]any{}
	if userID == thread.ViewerID {
		updates["viewer_unread"] = 0
	} else {
		updates["creator_unread"] = 0
	}
	return r.db.WithContext(ctx).Model(&model.DirectThread{}).Where("id = ?", threadID).Updates(updates).Error
}

func (r *MessageRepo) UpdateThreadOptions(ctx context.Context, threadID, userID string, input ThreadOptionUpdate, now time.Time) (*model.DirectThread, error) {
	thread, err := r.DirectThreadForUser(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"updated_at": now}
	prefix := "viewer"
	if userID == thread.CreatorID {
		prefix = "creator"
	}
	if input.Pinned != nil {
		key := prefix + "_pinned_at"
		if *input.Pinned {
			updates[key] = now
		} else {
			updates[key] = nil
		}
	}
	if input.Muted != nil {
		updates[prefix+"_muted"] = *input.Muted
	}
	if input.PushDisabled != nil {
		updates[prefix+"_push_disabled"] = *input.PushDisabled
	}
	if err := r.db.WithContext(ctx).Model(&model.DirectThread{}).Where("id = ?", threadID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return r.DirectThreadForUser(ctx, threadID, userID)
}

func (r *MessageRepo) UpsertBlock(ctx context.Context, blockerID, targetUserID, targetRole, reason string, now time.Time) error {
	block := model.UserBlock{
		BlockerID:    blockerID,
		TargetUserID: targetUserID,
		TargetRole:   targetRole,
		Reason:       strings.TrimSpace(reason),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if block.TargetRole == "" {
		block.TargetRole = "user"
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "blocker_id"}, {Name: "target_user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"target_role": block.TargetRole,
			"reason":      block.Reason,
			"updated_at":  now,
		}),
	}).Create(&block).Error
}

func (r *MessageRepo) DeleteBlock(ctx context.Context, blockerID, targetUserID string) error {
	return r.db.WithContext(ctx).
		Where("blocker_id = ? AND target_user_id = ?", blockerID, targetUserID).
		Delete(&model.UserBlock{}).Error
}

func (r *MessageRepo) BlockExists(ctx context.Context, blockerID, targetUserID string) (bool, error) {
	if blockerID == "" || targetUserID == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserBlock{}).
		Where("blocker_id = ? AND target_user_id = ?", blockerID, targetUserID).
		Count(&count).Error
	return count > 0, err
}

func (r *MessageRepo) BlocksEitherWay(ctx context.Context, userA, userB string) (bool, error) {
	if userA == "" || userB == "" || userA == userB {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserBlock{}).
		Where("(blocker_id = ? AND target_user_id = ?) OR (blocker_id = ? AND target_user_id = ?)", userA, userB, userB, userA).
		Count(&count).Error
	return count > 0, err
}

func (r *MessageRepo) ListBlocks(ctx context.Context, blockerID string, page, size int) ([]model.UserBlock, int64, error) {
	page, size = normalizeMessagePage(page, size)
	tx := r.db.WithContext(ctx).Model(&model.UserBlock{}).Where("blocker_id = ?", blockerID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.UserBlock
	err := tx.Order("updated_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *MessageRepo) Preference(ctx context.Context, userID string) (*model.MessagePreference, error) {
	var pref model.MessagePreference
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Take(&pref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultPreference(userID), nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (r *MessageRepo) UpsertPreference(ctx context.Context, pref *model.MessagePreference) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"message_reminder_enabled": pref.MessageReminderEnabled,
			"reply_reminder_scope":     pref.ReplyReminderScope,
			"mention_reminder_scope":   pref.MentionReminderScope,
			"like_reminder_enabled":    pref.LikeReminderEnabled,
			"fold_unfollowed_messages": pref.FoldUnfollowedMessages,
			"updated_at":               pref.UpdatedAt,
		}),
	}).Create(pref).Error
}

func (r *MessageRepo) FanClubMemberIDs(ctx context.Context, creatorID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Table("fan_badges").
		Select("user_id").
		Where("creator_id = ? AND user_id <> ?", creatorID, creatorID).
		Order("updated_at ASC").
		Pluck("user_id", &ids).Error
	if isMissingTableName(err) {
		return []string{}, nil
	}
	return ids, err
}

func (r *MessageRepo) SyncFanGroups(ctx context.Context, creatorID, creatorName string, now time.Time) ([]model.FanGroupChat, error) {
	memberIDs, err := r.FanClubMemberIDs(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	chunkSize := 199
	groupCount := 1
	if len(memberIDs) > 0 {
		groupCount = (len(memberIDs) + chunkSize - 1) / chunkSize
	}
	groups := make([]model.FanGroupChat, 0, groupCount)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.FanGroupMember{}).
			Where("group_id IN (SELECT id FROM fan_group_chats WHERE creator_id = ?) AND user_id <> ?", creatorID, creatorID).
			Update("kicked_at", now).Error; err != nil {
			return err
		}
		for i := 0; i < groupCount; i++ {
			groupNo := i + 1
			groupName := creatorName
			if groupName == "" {
				groupName = "粉丝团"
			}
			if groupCount > 1 {
				groupName = groupName + " 粉丝群 " + strconv.Itoa(groupNo)
			} else {
				groupName = groupName + " 粉丝群"
			}
			group := model.FanGroupChat{
				ID:        uuid.NewString(),
				CreatorID: creatorID,
				GroupNo:   groupNo,
				Name:      groupName,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "creator_id"}, {Name: "group_no"}},
				DoUpdates: clause.Assignments(map[string]any{
					"name":       group.Name,
					"updated_at": now,
				}),
			}).Create(&group).Error; err != nil {
				return err
			}
			if err := tx.Where("creator_id = ? AND group_no = ?", creatorID, groupNo).Take(&group).Error; err != nil {
				return err
			}
			groups = append(groups, group)
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "group_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"role":        model.FanGroupRoleOwner,
					"kicked_at":   nil,
					"muted_until": nil,
					"updated_at":  now,
				}),
			}).Create(&model.FanGroupMember{
				GroupID:   group.ID,
				UserID:    creatorID,
				Role:      model.FanGroupRoleOwner,
				CreatedAt: now,
				UpdatedAt: now,
			}).Error; err != nil {
				return err
			}
			start := i * chunkSize
			end := start + chunkSize
			if end > len(memberIDs) {
				end = len(memberIDs)
			}
			for _, userID := range memberIDs[start:end] {
				if err := tx.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "group_id"}, {Name: "user_id"}},
					DoUpdates: clause.Assignments(map[string]any{
						"kicked_at":  nil,
						"updated_at": now,
					}),
				}).Create(&model.FanGroupMember{
					GroupID:   group.ID,
					UserID:    userID,
					Role:      model.FanGroupRoleMember,
					CreatedAt: now,
					UpdatedAt: now,
				}).Error; err != nil {
					return err
				}
			}
			var count int64
			if err := tx.Model(&model.FanGroupMember{}).
				Where("group_id = ? AND kicked_at IS NULL", group.ID).
				Count(&count).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.FanGroupChat{}).Where("id = ?", group.ID).
				Updates(map[string]any{"member_count": count, "updated_at": now}).Error; err != nil {
				return err
			}
			group.MemberCount = int(count)
		}
		return nil
	})
	return groups, err
}

func (r *MessageRepo) ListFanGroups(ctx context.Context, creatorID string) ([]FanGroupWithMembers, error) {
	var groups []model.FanGroupChat
	if err := r.db.WithContext(ctx).Where("creator_id = ?", creatorID).Order("group_no ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return r.fanGroupsWithMembers(ctx, groups)
}

func (r *MessageRepo) ListJoinedFanGroups(ctx context.Context, userID string) ([]FanGroupWithMembers, error) {
	var groups []model.FanGroupChat
	err := r.db.WithContext(ctx).
		Table("fan_group_chats AS fg").
		Select("fg.*").
		Joins("JOIN fan_group_members AS mine ON mine.group_id = fg.id AND mine.user_id = ? AND mine.kicked_at IS NULL", userID).
		Order("fg.updated_at DESC, fg.group_no ASC").
		Find(&groups).Error
	if err != nil {
		return nil, err
	}
	return r.fanGroupsWithMembers(ctx, groups)
}

func (r *MessageRepo) fanGroupsWithMembers(ctx context.Context, groups []model.FanGroupChat) ([]FanGroupWithMembers, error) {
	out := make([]FanGroupWithMembers, 0, len(groups))
	for _, group := range groups {
		var members []FanGroupMemberRow
		err := r.db.WithContext(ctx).
			Table("fan_group_members AS gm").
			Select(`
gm.group_id,
gm.user_id,
gm.role,
gm.muted_until,
gm.kicked_at,
gm.created_at,
gm.updated_at,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), gm.user_id) AS name,
COALESCE(u.avatar, '') AS avatar,
COALESCE(u.verified, false) AS verified
`).
			Joins("LEFT JOIN users AS u ON u.id = gm.user_id").
			Where("gm.group_id = ? AND gm.kicked_at IS NULL", group.ID).
			Order("CASE gm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, gm.created_at ASC").
			Limit(220).
			Scan(&members).Error
		if err != nil {
			return nil, err
		}
		out = append(out, FanGroupWithMembers{Group: group, Members: members})
	}
	return out, nil
}

func (r *MessageRepo) FanGroupForCreator(ctx context.Context, creatorID, groupID string) (*model.FanGroupChat, error) {
	var group model.FanGroupChat
	err := r.db.WithContext(ctx).Where("id = ? AND creator_id = ?", groupID, creatorID).Take(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFanGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *MessageRepo) UpdateFanGroupMember(ctx context.Context, creatorID, groupID, userID, role string, mutedUntil *time.Time, clearMute bool, kick bool, now time.Time) error {
	group, err := r.FanGroupForCreator(ctx, creatorID, groupID)
	if err != nil {
		return err
	}
	if userID == creatorID {
		return ErrFanGroupMemberNotFound
	}
	updates := map[string]any{"updated_at": now}
	if role != "" {
		updates["role"] = role
	}
	if mutedUntil != nil {
		updates["muted_until"] = mutedUntil
	} else if clearMute {
		updates["muted_until"] = nil
	}
	if kick {
		updates["kicked_at"] = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.FanGroupMember{}).
			Where("group_id = ? AND user_id = ? AND kicked_at IS NULL", group.ID, userID).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrFanGroupMemberNotFound
		}
		var count int64
		if err := tx.Model(&model.FanGroupMember{}).
			Where("group_id = ? AND kicked_at IS NULL", group.ID).
			Count(&count).Error; err != nil {
			return err
		}
		return tx.Model(&model.FanGroupChat{}).
			Where("id = ?", group.ID).
			Updates(map[string]any{"member_count": count, "updated_at": now}).Error
	})
}

func (r *MessageRepo) FanGroupMemberForUser(ctx context.Context, groupID, userID string) (*model.FanGroupMember, error) {
	var member model.FanGroupMember
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ? AND kicked_at IS NULL", groupID, userID).
		Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFanGroupMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *MessageRepo) FanGroupMessages(ctx context.Context, groupID, userID string, page, size int) ([]FanGroupMessageRow, int64, error) {
	page, size = normalizeMessagePage(page, size)
	if _, err := r.FanGroupMemberForUser(ctx, groupID, userID); err != nil {
		return nil, 0, err
	}
	tx := r.db.WithContext(ctx).Model(&model.FanGroupMessage{}).Where("group_id = ?", groupID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []FanGroupMessageRow
	err := r.db.WithContext(ctx).
		Table("fan_group_messages AS msg").
		Select(`
msg.id,
msg.group_id,
msg.sender_id,
msg.body,
msg.created_at,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), msg.sender_id) AS name,
COALESCE(u.avatar, '') AS avatar,
COALESCE(u.verified, false) AS verified
`).
		Joins("LEFT JOIN users AS u ON u.id = msg.sender_id").
		Where("msg.group_id = ?", groupID).
		Order("msg.created_at ASC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	return rows, total, err
}

func (r *MessageRepo) SendFanGroupMessage(ctx context.Context, groupID, senderID, body string, now time.Time) (*model.FanGroupMessage, error) {
	member, err := r.FanGroupMemberForUser(ctx, groupID, senderID)
	if err != nil {
		return nil, err
	}
	if member.MutedUntil != nil && member.MutedUntil.After(now) {
		return nil, ErrFanGroupMuted
	}
	message := model.FanGroupMessage{
		ID:        uuid.NewString(),
		GroupID:   groupID,
		SenderID:  senderID,
		Body:      body,
		CreatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

func defaultPreference(userID string) *model.MessagePreference {
	return &model.MessagePreference{
		UserID:                 userID,
		MessageReminderEnabled: true,
		ReplyReminderScope:     model.MessagePreferenceScopeAll,
		MentionReminderScope:   model.MessagePreferenceScopeAll,
		LikeReminderEnabled:    true,
		FoldUnfollowedMessages: false,
	}
}

func normalizeMessagePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func trimDBPreview(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 140 {
		return string(runes[:140])
	}
	return value
}
