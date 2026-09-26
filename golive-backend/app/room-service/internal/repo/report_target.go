package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

// ErrReportTargetNotFound is returned when the reported content does not exist
// (or no longer exists) in the store we would moderate it in.
var ErrReportTargetNotFound = errors.New("report target not found")

// ReportTarget is the server-side view of a reported item: who wrote it, which
// room/channel it belongs to and a snapshot of its text. Report actions
// (sanctions, force-ending a room, moderation notifications) are driven only by
// these fields, never by what the reporter's client claimed.
type ReportTarget struct {
	// TargetID is the canonical id for the target type; channel keys are
	// normalized to ch-<ownerID> so reports on one channel group together.
	TargetID  string
	RoomID    string
	ChannelID string
	OwnerID   string
	OwnerName string
	UserID    string
	UserName  string
	Title     string
	Text      string
	// Link is a relative in-app path to the target.
	Link string
}

// ResolveReportTarget looks the reported item up by type and id. roomID is only
// used to locate danmu, which are keyed by (room_id, id); the returned RoomID is
// always the one stored with the content.
func (r *ModerationRepo) ResolveReportTarget(ctx context.Context, targetType, targetID, roomID string) (*ReportTarget, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil, ErrReportTargetNotFound
	}
	switch targetType {
	case model.ReportTargetRoom:
		return r.resolveRoomTarget(ctx, targetID)
	case model.ReportTargetChannel:
		return r.resolveChannelTarget(ctx, targetID)
	case model.ReportTargetPost:
		return r.resolvePostTarget(ctx, targetID)
	case model.ReportTargetPostComment:
		return r.resolveCommentTarget(ctx, targetID)
	case model.ReportTargetDanmu:
		return r.resolveDanmuTarget(ctx, strings.TrimSpace(roomID), targetID)
	case model.ReportTargetSuperChat:
		return r.resolveSuperChatTarget(ctx, targetID)
	default:
		return nil, ErrReportTargetNotFound
	}
}

func (r *ModerationRepo) resolveRoomTarget(ctx context.Context, roomID string) (*ReportTarget, error) {
	var room model.Room
	if err := r.db.WithContext(ctx).Where("id = ?", roomID).Take(&room).Error; err != nil {
		return nil, notFoundAsTargetMissing(err)
	}
	return &ReportTarget{
		TargetID:  room.ID,
		RoomID:    room.ID,
		ChannelID: reportChannelID(room.OwnerID),
		OwnerID:   room.OwnerID,
		OwnerName: r.reportUserName(ctx, room.OwnerID, room.Channel),
		Title:     room.Title,
		Text:      room.Description,
		Link:      "/live/" + room.ID,
	}, nil
}

func (r *ModerationRepo) resolveChannelTarget(ctx context.Context, key string) (*ReportTarget, error) {
	key = strings.TrimPrefix(key, "ch-")
	var ownerID string
	for _, column := range []string{"id", "username", "display_name"} {
		var ids []string
		err := r.db.WithContext(ctx).Table("users").
			Where(column+" = ?", key).
			Limit(1).
			Pluck("id", &ids).Error
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 && strings.TrimSpace(ids[0]) != "" {
			ownerID = ids[0]
			break
		}
	}
	if ownerID == "" {
		return nil, ErrReportTargetNotFound
	}
	name := r.reportUserName(ctx, ownerID, "")
	return &ReportTarget{
		TargetID:  reportChannelID(ownerID),
		ChannelID: reportChannelID(ownerID),
		OwnerID:   ownerID,
		OwnerName: name,
		Title:     name,
		Link:      "/channel/" + ownerID,
	}, nil
}

func (r *ModerationRepo) resolvePostTarget(ctx context.Context, postID string) (*ReportTarget, error) {
	var post model.ChannelPost
	if err := r.db.WithContext(ctx).Where("id = ?", postID).Take(&post).Error; err != nil {
		return nil, notFoundAsTargetMissing(err)
	}
	ownerName := r.reportUserName(ctx, post.OwnerID, "")
	return &ReportTarget{
		TargetID:  post.ID,
		ChannelID: reportChannelID(post.OwnerID),
		OwnerID:   post.OwnerID,
		OwnerName: ownerName,
		UserID:    post.OwnerID,
		UserName:  ownerName,
		Title:     ownerName,
		Text:      post.Content,
		Link:      "/channel/" + post.OwnerID + "#post-" + post.ID,
	}, nil
}

// resolveCommentTarget handles post comments and, because the web client
// reports them with the same target type, replay comments.
func (r *ModerationRepo) resolveCommentTarget(ctx context.Context, commentID string) (*ReportTarget, error) {
	var comment model.PostComment
	err := r.db.WithContext(ctx).Where("id = ?", commentID).Take(&comment).Error
	if err == nil {
		var post model.ChannelPost
		if err := r.db.WithContext(ctx).Unscoped().Where("id = ?", comment.PostID).Take(&post).Error; err != nil {
			return nil, notFoundAsTargetMissing(err)
		}
		ownerName := r.reportUserName(ctx, post.OwnerID, "")
		return &ReportTarget{
			TargetID:  comment.ID,
			ChannelID: reportChannelID(post.OwnerID),
			OwnerID:   post.OwnerID,
			OwnerName: ownerName,
			UserID:    comment.UserID,
			UserName:  r.reportUserName(ctx, comment.UserID, ""),
			Title:     ownerName,
			Text:      comment.Content,
			Link:      "/channel/" + post.OwnerID + "#comment-" + comment.ID,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var replay model.ReplayComment
	if err := r.db.WithContext(ctx).Where("id = ?", commentID).Take(&replay).Error; err != nil {
		return nil, notFoundAsTargetMissing(err)
	}
	target := &ReportTarget{
		TargetID: replay.ID,
		RoomID:   replay.RoomID,
		UserID:   replay.UserID,
		UserName: r.reportUserName(ctx, replay.UserID, ""),
		Text:     replay.Content,
		Link:     "/live/" + replay.RoomID + "#replay-comment-" + replay.ID,
	}
	target.Title = target.UserName
	r.fillReportRoomOwner(ctx, target)
	return target, nil
}

func (r *ModerationRepo) resolveDanmuTarget(ctx context.Context, roomID, danmuID string) (*ReportTarget, error) {
	if roomID == "" {
		return nil, ErrReportTargetNotFound
	}
	type danmuRow struct {
		ID       string
		RoomID   string
		UserID   string
		Username string
		Text     string
	}
	for i := 0; i < danmuShardCount; i++ {
		var rows []danmuRow
		err := r.db.WithContext(ctx).Table(fmt.Sprintf("danmus_%d", i)).
			Select("id, room_id, user_id, username, text").
			Where("room_id = ? AND id = ? AND deleted_at IS NULL", roomID, danmuID).
			Limit(1).
			Scan(&rows).Error
		if err != nil {
			if isMissingTableName(err) || isMissingColumn(err) {
				continue
			}
			return nil, err
		}
		if len(rows) == 0 {
			continue
		}
		row := rows[0]
		target := &ReportTarget{
			TargetID: row.ID,
			RoomID:   row.RoomID,
			UserID:   row.UserID,
			UserName: r.reportUserName(ctx, row.UserID, row.Username),
			Text:     row.Text,
			Link:     "/live/" + row.RoomID,
		}
		r.fillReportRoomOwner(ctx, target)
		target.Title = target.OwnerName
		return target, nil
	}
	return nil, ErrReportTargetNotFound
}

func (r *ModerationRepo) resolveSuperChatTarget(ctx context.Context, orderID string) (*ReportTarget, error) {
	var rows []struct {
		OrderID string
		UserID  string
		RoomID  string
		Text    string
	}
	err := r.db.WithContext(ctx).Table("super_chat_orders").
		Select("order_id, user_id, room_id, text").
		Where("order_id = ? AND status = ?", orderID, "success").
		Limit(1).
		Scan(&rows).Error
	if err != nil {
		if isMissingTableName(err) {
			return nil, ErrReportTargetNotFound
		}
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrReportTargetNotFound
	}
	row := rows[0]
	target := &ReportTarget{
		TargetID: row.OrderID,
		RoomID:   row.RoomID,
		UserID:   row.UserID,
		UserName: r.reportUserName(ctx, row.UserID, ""),
		Text:     row.Text,
		Link:     "/live/" + row.RoomID,
	}
	r.fillReportRoomOwner(ctx, target)
	target.Title = target.OwnerName
	return target, nil
}

// fillReportRoomOwner sets the owner/channel of target.RoomID. A missing room
// leaves them empty rather than failing: the content itself was found.
func (r *ModerationRepo) fillReportRoomOwner(ctx context.Context, target *ReportTarget) {
	if target.RoomID == "" {
		return
	}
	var room model.Room
	if err := r.db.WithContext(ctx).Where("id = ?", target.RoomID).Take(&room).Error; err != nil {
		return
	}
	target.OwnerID = room.OwnerID
	target.OwnerName = r.reportUserName(ctx, room.OwnerID, room.Channel)
	target.ChannelID = reportChannelID(room.OwnerID)
}

func (r *ModerationRepo) reportUserName(ctx context.Context, userID, fallback string) string {
	if strings.TrimSpace(userID) == "" {
		return fallback
	}
	if profile, err := r.UserProfile(ctx, userID); err == nil && strings.TrimSpace(profile.Name) != "" {
		return profile.Name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return userID
}

func reportChannelID(ownerID string) string {
	if strings.TrimSpace(ownerID) == "" {
		return ""
	}
	return "ch-" + ownerID
}

func notFoundAsTargetMissing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableName(err) {
		return ErrReportTargetNotFound
	}
	return err
}
