package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const maxReplayCommentLen = 500

// ReplayViewChecker abstracts the replay visibility check so the comment
// service can reuse the exact watch rules enforced by ReplayService.
type ReplayViewChecker interface {
	CanView(ctx context.Context, room model.Room, viewerID string) (bool, error)
}

type ReplayCommentService struct {
	comments   *repo.ReplayCommentRepo
	rooms      *repo.RoomRepo
	viewer     ReplayViewChecker
	textPolicy TextPolicy
	blocks     ChannelBlockChecker
	notify     NotificationWriter
}

func NewReplayCommentService(comments *repo.ReplayCommentRepo, rooms *repo.RoomRepo, viewer ReplayViewChecker) *ReplayCommentService {
	return &ReplayCommentService{comments: comments, rooms: rooms, viewer: viewer}
}

func (s *ReplayCommentService) SetTextPolicy(policy TextPolicy) { s.textPolicy = policy }

func (s *ReplayCommentService) SetBlockChecker(blocks ChannelBlockChecker) { s.blocks = blocks }

func (s *ReplayCommentService) SetNotificationWriter(writer NotificationWriter) { s.notify = writer }

type ReplayCommentDTO struct {
	ID         string     `json:"id"`
	RoomID     string     `json:"roomId"`
	UserID     string     `json:"userId"`
	ParentID   string     `json:"parentId,omitempty"`
	RootID     string     `json:"rootId,omitempty"`
	Depth      int        `json:"depth"`
	Content    string     `json:"content"`
	LikeCount  int64      `json:"likeCount"`
	ReplyCount int64      `json:"replyCount"`
	Liked      bool       `json:"liked"`
	CanDelete  bool       `json:"canDelete"`
	Author     PostAuthor `json:"author"`
	CreatedAt  string     `json:"createdAt"`
	UpdatedAt  string     `json:"updatedAt"`
}

type ReplayCommentListResp struct {
	Items      []ReplayCommentDTO `json:"items"`
	Total      int64              `json:"total"`
	CanComment bool               `json:"canComment"`
}

type ReplayCommentLikeState struct {
	CommentID string `json:"commentId"`
	Liked     bool   `json:"liked"`
	Likes     int64  `json:"likes"`
}

func (s *ReplayCommentService) List(ctx context.Context, viewerID, roomID string) (*ReplayCommentListResp, error) {
	room, err := s.watchableRoom(ctx, viewerID, roomID)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.comments.List(ctx, room.ID, 200)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	liked, err := s.comments.LikedIDs(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	items := make([]ReplayCommentDTO, 0, len(rows))
	for _, row := range rows {
		canDelete := viewerID != "" && (viewerID == room.OwnerID || viewerID == row.UserID)
		items = append(items, replayCommentRowDTO(row, liked[row.ID], canDelete))
	}
	canComment := viewerID != "" && !s.isBlocked(ctx, viewerID, room.OwnerID)
	return &ReplayCommentListResp{Items: items, Total: total, CanComment: canComment}, nil
}

func (s *ReplayCommentService) Create(ctx context.Context, userID, roomID string, req CreateCommentReq) (*ReplayCommentDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	room, err := s.watchableRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	if userID != room.OwnerID && s.isBlocked(ctx, userID, room.OwnerID) {
		return nil, errcode.New(http.StatusForbidden, "comments are restricted").WithReason("comments_restricted")
	}
	content := cleanPostText(req.Content, maxReplayCommentLen)
	if content == "" {
		return nil, errcode.New(http.StatusBadRequest, "comment content is required").WithReason("content_required")
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureTextAllowed(ctx, content); err != nil {
			return nil, err
		}
	}

	commentID := uuid.NewString()
	parentID := strings.TrimSpace(req.ParentID)
	rootID := commentID
	depth := 0
	var parent *model.ReplayComment
	if parentID != "" {
		parent, err = s.comments.Get(ctx, room.ID, parentID)
		if err != nil {
			return nil, replayCommentError(err)
		}
		if parent.Depth >= maxCommentTreeDepth {
			return nil, errcode.New(http.StatusConflict, "reply depth limit reached").WithReason("reply_depth_limit")
		}
		depth = parent.Depth + 1
		rootID = parent.RootID
		if rootID == "" {
			rootID = parent.ID
		}
	}

	comment := &model.ReplayComment{
		ID:       commentID,
		RoomID:   room.ID,
		UserID:   userID,
		ParentID: parentID,
		RootID:   rootID,
		Depth:    depth,
		Content:  content,
	}
	if err := s.comments.Create(ctx, comment); err != nil {
		return nil, err
	}
	_ = s.notifyComment(ctx, *room, *comment, parent)
	author := s.authorForUser(ctx, userID)
	dto := replayCommentDTO(*comment, author, false, true)
	return &dto, nil
}

func (s *ReplayCommentService) Delete(ctx context.Context, userID, roomID, commentID string) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return replayRoomError(err)
	}
	comment, err := s.comments.Get(ctx, roomID, commentID)
	if err != nil {
		return replayCommentError(err)
	}
	if userID != room.OwnerID && userID != comment.UserID {
		return errcode.New(http.StatusForbidden, "comment cannot be deleted").WithReason("comment_delete_forbidden")
	}
	if _, err := s.comments.DeleteTree(ctx, roomID, commentID); err != nil {
		return replayCommentError(err)
	}
	return nil
}

func (s *ReplayCommentService) Like(ctx context.Context, userID, roomID, commentID string) (*ReplayCommentLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	room, err := s.watchableRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	comment, err := s.comments.Get(ctx, room.ID, commentID)
	if err != nil {
		return nil, replayCommentError(err)
	}
	alreadyLiked, err := s.comments.LikedIDs(ctx, userID, []string{commentID})
	if err != nil {
		return nil, err
	}
	count, err := s.comments.Like(ctx, commentID, userID)
	if err != nil {
		return nil, replayCommentError(err)
	}
	if !alreadyLiked[commentID] {
		_ = s.notifyCommentLiked(ctx, *room, *comment, userID)
	}
	return &ReplayCommentLikeState{CommentID: commentID, Liked: true, Likes: count}, nil
}

func (s *ReplayCommentService) Unlike(ctx context.Context, userID, roomID, commentID string) (*ReplayCommentLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	room, err := s.watchableRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	if _, err := s.comments.Get(ctx, room.ID, commentID); err != nil {
		return nil, replayCommentError(err)
	}
	count, err := s.comments.Unlike(ctx, commentID, userID)
	if err != nil {
		return nil, replayCommentError(err)
	}
	return &ReplayCommentLikeState{CommentID: commentID, Liked: false, Likes: count}, nil
}

func (s *ReplayCommentService) watchableRoom(ctx context.Context, viewerID, roomID string) (*model.Room, error) {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return nil, replayRoomError(err)
	}
	if viewerID != "" && viewerID == room.OwnerID {
		return room, nil
	}
	if s.viewer != nil {
		canView, err := s.viewer.CanView(ctx, *room, viewerID)
		if err != nil {
			return nil, err
		}
		if !canView {
			return nil, errcode.New(http.StatusForbidden, "replay is not available").WithReason("replay_not_visible")
		}
	}
	return room, nil
}

func (s *ReplayCommentService) isBlocked(ctx context.Context, viewerID, ownerID string) bool {
	if s.blocks == nil || viewerID == "" || ownerID == "" || viewerID == ownerID {
		return false
	}
	blocked, err := s.blocks.BlocksInteraction(ctx, viewerID, ownerID)
	if err != nil {
		return false
	}
	return blocked
}

func (s *ReplayCommentService) authorForUser(ctx context.Context, userID string) PostAuthor {
	profile, err := s.comments.UserProfile(ctx, userID)
	if err != nil {
		return fallbackPostAuthor(userID)
	}
	return PostAuthor{
		ID:          profile.ID,
		Username:    profile.Username,
		DisplayName: profile.DisplayName,
		Name:        nonEmpty(profile.Name, profile.Username, profile.ID),
		Avatar:      profile.Avatar,
		Verified:    profile.Verified,
	}
}

func (s *ReplayCommentService) notifyComment(ctx context.Context, room model.Room, comment model.ReplayComment, parent *model.ReplayComment) error {
	if s.notify == nil {
		return nil
	}
	targetID := room.OwnerID
	kind := "replay_comment"
	title := "有人评论了你的直播回放"
	if parent != nil {
		targetID = parent.UserID
		kind = "replay_comment_reply"
		title = "有人回复了你的评论"
	}
	if targetID == "" || targetID == comment.UserID {
		return nil
	}
	actor := s.authorForUser(ctx, comment.UserID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            kind + "-" + notificationHash(room.ID, comment.ID, targetID),
		UserID:        targetID,
		Type:          kind,
		Title:         title,
		Body:          trimRunes(comment.Content, 120),
		Link:          "/live/" + room.ID,
		ActorID:       actor.ID,
		ActorUsername: actor.Username,
		ActorName:     actor.Name,
		ActorAvatar:   actor.Avatar,
		ActorVerified: actor.Verified,
		CreatedAt:     time.Now(),
	}})
}

func (s *ReplayCommentService) notifyCommentLiked(ctx context.Context, room model.Room, comment model.ReplayComment, actorID string) error {
	if s.notify == nil || comment.UserID == "" || comment.UserID == actorID {
		return nil
	}
	actor := s.authorForUser(ctx, actorID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            "replay-comment-liked-" + notificationHash(comment.ID, actorID),
		UserID:        comment.UserID,
		Type:          "replay_comment_liked",
		Title:         "你的评论收到新的赞",
		Body:          trimRunes(comment.Content, 120),
		Link:          "/live/" + room.ID,
		ActorID:       actor.ID,
		ActorUsername: actor.Username,
		ActorName:     actor.Name,
		ActorAvatar:   actor.Avatar,
		ActorVerified: actor.Verified,
		CreatedAt:     time.Now(),
	}})
}

func replayCommentRowDTO(row repo.ReplayCommentRow, liked, canDelete bool) ReplayCommentDTO {
	return ReplayCommentDTO{
		ID:         row.ID,
		RoomID:     row.RoomID,
		UserID:     row.UserID,
		ParentID:   row.ParentID,
		RootID:     row.RootID,
		Depth:      row.Depth,
		Content:    row.Content,
		LikeCount:  row.LikeCount,
		ReplyCount: row.ReplyCount,
		Liked:      liked,
		CanDelete:  canDelete,
		Author: PostAuthor{
			ID:          row.UserID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Name:        nonEmpty(row.AuthorName, row.Username, row.UserID),
			Avatar:      row.Avatar,
			Verified:    row.Verified,
		},
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func replayCommentDTO(comment model.ReplayComment, author PostAuthor, liked, canDelete bool) ReplayCommentDTO {
	return ReplayCommentDTO{
		ID:         comment.ID,
		RoomID:     comment.RoomID,
		UserID:     comment.UserID,
		ParentID:   comment.ParentID,
		RootID:     comment.RootID,
		Depth:      comment.Depth,
		Content:    comment.Content,
		LikeCount:  comment.LikeCount,
		ReplyCount: comment.ReplyCount,
		Liked:      liked,
		CanDelete:  canDelete,
		Author:     author,
		CreatedAt:  comment.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  comment.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func replayCommentError(err error) error {
	switch {
	case errors.Is(err, repo.ErrReplayCommentNotFound):
		return errcode.New(http.StatusNotFound, "comment not found").WithReason("comment_not_found")
	case errors.Is(err, repo.ErrRoomNotFound):
		return errcode.New(http.StatusNotFound, "replay not found").WithReason("replay_not_found")
	default:
		return err
	}
}

func replayRoomError(err error) error {
	if errors.Is(err, repo.ErrRoomNotFound) {
		return errcode.New(http.StatusNotFound, "replay not found").WithReason("replay_not_found")
	}
	return err
}
