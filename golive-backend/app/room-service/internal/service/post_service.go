package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	maxPostImages       = 6
	maxPostContentLen   = 2000
	maxCommentTextLen   = 500
	maxCommentTreeDepth = 2
)

type PostService struct {
	posts      *repo.PostRepo
	rooms      *repo.RoomRepo
	social     *repo.SocialRepo
	permission LivePermissionChecker
	textPolicy TextPolicy
	blocks     ChannelBlockChecker
	notify     NotificationWriter
}

func NewPostService(posts *repo.PostRepo, rooms *repo.RoomRepo, social *repo.SocialRepo, permission LivePermissionChecker) *PostService {
	return &PostService{posts: posts, rooms: rooms, social: social, permission: permission}
}

type TextPolicy interface {
	EnsureTextAllowed(ctx context.Context, texts ...string) error
	EnsureUserCanInteract(ctx context.Context, userID string) error
}

type NotificationWriter interface {
	CreateNotifications(ctx context.Context, notifications []model.Notification) error
}

func (s *PostService) SetTextPolicy(policy TextPolicy) {
	s.textPolicy = policy
}

func (s *PostService) SetBlockChecker(blocks ChannelBlockChecker) {
	s.blocks = blocks
}

func (s *PostService) SetNotificationWriter(writer NotificationWriter) {
	s.notify = writer
}

type CreatePostReq struct {
	Content         string   `json:"content"`
	Images          []string `json:"images"`
	Visibility      string   `json:"visibility"`
	CommentsEnabled *bool    `json:"commentsEnabled"`
	CommentMode     string   `json:"commentMode"`
}

type UpdatePostVisibilityReq struct {
	Visibility string `json:"visibility"`
}

type CreateCommentReq struct {
	Content  string `json:"content"`
	ParentID string `json:"parentId"`
}

type PostAuthor struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	Verified    bool   `json:"verified"`
}

type ChannelPostDTO struct {
	ID              string     `json:"id"`
	OwnerID         string     `json:"ownerId"`
	ChannelID       string     `json:"channelId"`
	Content         string     `json:"content"`
	Images          []string   `json:"images"`
	Visibility      string     `json:"visibility"`
	CommentsEnabled bool       `json:"commentsEnabled"`
	CommentMode     string     `json:"commentMode"`
	LikeCount       int64      `json:"likeCount"`
	CommentCount    int64      `json:"commentCount"`
	Liked           bool       `json:"liked"`
	CanComment      bool       `json:"canComment"`
	CanDelete       bool       `json:"canDelete"`
	Author          PostAuthor `json:"author"`
	CreatedAt       string     `json:"createdAt"`
	UpdatedAt       string     `json:"updatedAt"`
}

type PostListResp struct {
	Items []ChannelPostDTO `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

type PostCommentDTO struct {
	ID         string     `json:"id"`
	PostID     string     `json:"postId"`
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

type PostCommentListResp struct {
	Items []PostCommentDTO `json:"items"`
	Total int64            `json:"total"`
}

type PostLikeState struct {
	PostID string `json:"postId"`
	Liked  bool   `json:"liked"`
	Likes  int64  `json:"likes"`
}

type CommentLikeState struct {
	CommentID string `json:"commentId"`
	Liked     bool   `json:"liked"`
	Likes     int64  `json:"likes"`
}

func (s *PostService) CreatePost(ctx context.Context, ownerID string, req CreatePostReq) (*ChannelPostDTO, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if err := s.requireCreatorPermission(ctx, ownerID); err != nil {
		return nil, err
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, ownerID); err != nil {
			return nil, err
		}
	}
	content := cleanPostText(req.Content, maxPostContentLen)
	images, err := cleanPostImages(req.Images)
	if err != nil {
		return nil, err
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureTextAllowed(ctx, content); err != nil {
			return nil, err
		}
	}
	if content == "" && len(images) == 0 {
		return nil, errcode.New(http.StatusBadRequest, "post content or image is required").WithReason("content_required")
	}
	commentsEnabled := true
	if req.CommentsEnabled != nil {
		commentsEnabled = *req.CommentsEnabled
	}
	post := &model.ChannelPost{
		ID:              uuid.NewString(),
		OwnerID:         ownerID,
		ChannelID:       channelIDForOwner(ownerID),
		Content:         content,
		ImagesJSON:      encodeImages(images),
		Visibility:      normalizePostVisibility(req.Visibility),
		CommentsEnabled: commentsEnabled,
		CommentMode:     normalizePostCommentMode(req.CommentMode),
		LikeCount:       0,
		CommentCount:    0,
	}
	if err := s.posts.CreatePost(ctx, post); err != nil {
		return nil, err
	}
	author := s.authorForUser(ctx, ownerID)
	dto, err := s.postDTO(ctx, *post, ownerID, author, false)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *PostService) ListMine(ctx context.Context, ownerID string, page, size int) (*PostListResp, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	page, size = normalizeListPage(page, size)
	posts, total, err := s.posts.ListByOwner(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	items, err := s.postsToDTO(ctx, posts, ownerID)
	if err != nil {
		return nil, err
	}
	return &PostListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *PostService) ListChannel(ctx context.Context, viewerID, channelKey string, page, size int) (*PostListResp, error) {
	page, size = normalizeListPage(page, size)
	ownerID, err := s.resolveOwnerID(ctx, channelKey)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return &PostListResp{Items: []ChannelPostDTO{}, Total: 0, Page: page, Size: size}, nil
	}
	if err != nil {
		return nil, err
	}
	isOwner := viewerID != "" && viewerID == ownerID
	if !isOwner && s.blocks != nil && viewerID != "" {
		blocked, err := s.blocks.BlocksInteraction(ctx, viewerID, ownerID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return &PostListResp{Items: []ChannelPostDTO{}, Total: 0, Page: page, Size: size}, nil
		}
	}
	var posts []model.ChannelPost
	var total int64
	if isOwner {
		posts, total, err = s.posts.ListByOwner(ctx, ownerID, page, size)
	} else {
		visibilities := []string{model.PostVisibilityPublic}
		var following bool
		following, err = s.isFollower(ctx, viewerID, channelIDForOwner(ownerID))
		if err != nil {
			return nil, err
		}
		if following {
			visibilities = append(visibilities, model.PostVisibilityFollowers)
		}
		posts, total, err = s.posts.ListVisibleByOwner(ctx, ownerID, visibilities, page, size)
	}
	if err != nil {
		return nil, err
	}
	items, err := s.postsToDTO(ctx, posts, viewerID)
	if err != nil {
		return nil, err
	}
	return &PostListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *PostService) ListSubscriptionLatest(ctx context.Context, viewerID string, size int) (*PostListResp, error) {
	if viewerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.social == nil {
		return &PostListResp{Items: []ChannelPostDTO{}, Total: 0, Page: 1, Size: size}, nil
	}
	_, size = normalizeListPage(1, size)
	// Blocked owners are dropped here, as ListChannel hides their posts too.
	targets, err := followedChannels(ctx, s.social, s.rooms, s.blocks, viewerID)
	if err != nil {
		return nil, err
	}
	ownerIDs := make([]string, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		ownerID := target.ownerID
		if ownerID == "" || seen[ownerID] {
			continue
		}
		seen[ownerID] = true
		ownerIDs = append(ownerIDs, ownerID)
	}
	if len(ownerIDs) == 0 {
		return &PostListResp{Items: []ChannelPostDTO{}, Total: 0, Page: 1, Size: size}, nil
	}
	posts, total, err := s.posts.ListLatestVisibleByOwners(ctx, ownerIDs, []string{
		model.PostVisibilityPublic,
		model.PostVisibilityFollowers,
	}, size)
	if err != nil {
		return nil, err
	}
	items, err := s.postsToDTO(ctx, posts, viewerID)
	if err != nil {
		return nil, err
	}
	return &PostListResp{Items: items, Total: total, Page: 1, Size: size}, nil
}

func (s *PostService) Search(ctx context.Context, viewerID, query string, limit int) ([]ChannelPostDTO, error) {
	limit = normalizeSearchSize(limit, 8, 24)
	if s.posts == nil {
		return []ChannelPostDTO{}, nil
	}
	phrase := repo.NewSearchPhrase(query)
	if phrase.Empty() {
		return []ChannelPostDTO{}, nil
	}
	candidates, err := s.posts.SearchVisible(ctx, phrase, limit*8)
	if err != nil {
		return nil, err
	}
	visible := make([]model.ChannelPost, 0, minInt(limit*2, len(candidates)))
	for _, post := range candidates {
		ok, err := s.canViewPost(ctx, viewerID, post)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		visible = append(visible, post)
	}
	items, err := s.postsToDTO(ctx, visible, viewerID)
	if err != nil {
		return nil, err
	}
	sortBySearchScore(items, func(item ChannelPostDTO) int {
		return searchScore(phrase, item.Content, item.Author.Name, item.Author.Username, item.Author.DisplayName, item.ChannelID)
	}, func(a, b ChannelPostDTO) bool {
		return a.CreatedAt > b.CreatedAt
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *PostService) UpdatePostVisibility(ctx context.Context, ownerID, postID string, req UpdatePostVisibilityReq) (*ChannelPostDTO, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	visibility := normalizePostVisibility(req.Visibility)
	post, err := s.posts.UpdatePostVisibility(ctx, ownerID, postID, visibility)
	if err != nil {
		return nil, postError(err)
	}
	author := s.authorForUser(ctx, ownerID)
	dto, err := s.postDTO(ctx, *post, ownerID, author, false)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *PostService) DeletePost(ctx context.Context, ownerID, postID string) error {
	if ownerID == "" {
		return errcode.ErrUnauthorized
	}
	if err := s.posts.DeletePost(ctx, ownerID, postID); err != nil {
		return postError(err)
	}
	return nil
}

func (s *PostService) ListComments(ctx context.Context, viewerID, postID string) (*PostCommentListResp, error) {
	post, err := s.visiblePost(ctx, viewerID, postID)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.posts.ListComments(ctx, post.ID, 200)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	liked, err := s.posts.CommentLikedIDs(ctx, viewerID, ids)
	if err != nil {
		return nil, err
	}
	items := make([]PostCommentDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, commentRowDTO(row, liked[row.ID], viewerID == post.OwnerID || viewerID == row.UserID))
	}
	return &PostCommentListResp{Items: items, Total: total}, nil
}

func (s *PostService) CreateComment(ctx context.Context, userID, postID string, req CreateCommentReq) (*PostCommentDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	post, err := s.visiblePost(ctx, userID, postID)
	if err != nil {
		return nil, err
	}
	canComment, err := s.canComment(ctx, userID, *post)
	if err != nil {
		return nil, err
	}
	if !canComment {
		return nil, errcode.New(http.StatusForbidden, "comments are restricted").WithReason("comments_restricted")
	}
	content := cleanPostText(req.Content, maxCommentTextLen)
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
	var parent *model.PostComment
	if parentID != "" {
		parent, err = s.posts.GetComment(ctx, postID, parentID)
		if err != nil {
			return nil, postError(err)
		}
		if parent.Depth >= maxCommentTreeDepth {
			return nil, errcode.New(http.StatusConflict, "reply depth limit reached").WithReason("reply_depth_limit")
		}
		blocked, err := blocksBetween(ctx, s.blocks, userID, parent.UserID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, userBlockedError()
		}
		depth = parent.Depth + 1
		rootID = parent.RootID
		if rootID == "" {
			rootID = parent.ID
		}
	}

	comment := &model.PostComment{
		ID:       commentID,
		PostID:   postID,
		UserID:   userID,
		ParentID: parentID,
		RootID:   rootID,
		Depth:    depth,
		Content:  content,
	}
	if err := s.posts.CreateComment(ctx, comment); err != nil {
		return nil, err
	}
	_ = s.notifyPostComment(ctx, *post, *comment, parent)
	author := s.authorForUser(ctx, userID)
	dto := commentDTO(*comment, author, false, true)
	return &dto, nil
}

func (s *PostService) DeleteComment(ctx context.Context, userID, postID, commentID string) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	post, err := s.posts.GetPost(ctx, postID)
	if err != nil {
		return postError(err)
	}
	comment, err := s.posts.GetComment(ctx, postID, commentID)
	if err != nil {
		return postError(err)
	}
	if userID != post.OwnerID && userID != comment.UserID {
		return errcode.New(http.StatusForbidden, "comment cannot be deleted").WithReason("comment_delete_forbidden")
	}
	if _, err := s.posts.DeleteCommentTree(ctx, postID, commentID); err != nil {
		return postError(err)
	}
	return nil
}

func (s *PostService) LikePost(ctx context.Context, userID, postID string) (*PostLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	post, err := s.visiblePost(ctx, userID, postID)
	if err != nil {
		return nil, err
	}
	alreadyLiked, err := s.posts.PostLikedIDs(ctx, userID, []string{postID})
	if err != nil {
		return nil, err
	}
	count, err := s.posts.LikePost(ctx, postID, userID)
	if err != nil {
		return nil, postError(err)
	}
	if !alreadyLiked[postID] {
		_ = s.notifyPostLiked(ctx, *post, userID)
	}
	return &PostLikeState{PostID: postID, Liked: true, Likes: count}, nil
}

func (s *PostService) UnlikePost(ctx context.Context, userID, postID string) (*PostLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	if _, err := s.visiblePost(ctx, userID, postID); err != nil {
		return nil, err
	}
	count, err := s.posts.UnlikePost(ctx, postID, userID)
	if err != nil {
		return nil, postError(err)
	}
	return &PostLikeState{PostID: postID, Liked: false, Likes: count}, nil
}

func (s *PostService) LikeComment(ctx context.Context, userID, postID, commentID string) (*CommentLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	post, err := s.visiblePost(ctx, userID, postID)
	if err != nil {
		return nil, err
	}
	comment, err := s.posts.GetComment(ctx, postID, commentID)
	if err != nil {
		return nil, postError(err)
	}
	blocked, err := blocksBetween(ctx, s.blocks, userID, comment.UserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, userBlockedError()
	}
	alreadyLiked, err := s.posts.CommentLikedIDs(ctx, userID, []string{commentID})
	if err != nil {
		return nil, err
	}
	count, err := s.posts.LikeComment(ctx, commentID, userID)
	if err != nil {
		return nil, postError(err)
	}
	if !alreadyLiked[commentID] {
		_ = s.notifyCommentLiked(ctx, *post, *comment, userID)
	}
	return &CommentLikeState{CommentID: commentID, Liked: true, Likes: count}, nil
}

func (s *PostService) UnlikeComment(ctx context.Context, userID, postID, commentID string) (*CommentLikeState, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureUserCanInteract(ctx, userID); err != nil {
			return nil, err
		}
	}
	if _, err := s.visiblePost(ctx, userID, postID); err != nil {
		return nil, err
	}
	if _, err := s.posts.GetComment(ctx, postID, commentID); err != nil {
		return nil, postError(err)
	}
	count, err := s.posts.UnlikeComment(ctx, commentID, userID)
	if err != nil {
		return nil, postError(err)
	}
	return &CommentLikeState{CommentID: commentID, Liked: false, Likes: count}, nil
}

func (s *PostService) requireCreatorPermission(ctx context.Context, userID string) error {
	if s.permission == nil {
		return nil
	}
	approved, err := s.permission.HasApprovedLivePermission(ctx, userID)
	if err != nil {
		return err
	}
	if !approved {
		return errcode.New(http.StatusForbidden, "creator permission is not approved").WithReason("creator_permission_required")
	}
	return nil
}

func (s *PostService) resolveOwnerID(ctx context.Context, channelKey string) (string, error) {
	key := strings.TrimSpace(channelKey)
	if key == "" {
		return "", repo.ErrRoomNotFound
	}
	if s.rooms != nil {
		return s.rooms.ResolveOwnerID(ctx, key)
	}
	if strings.HasPrefix(key, "ch-") {
		return strings.TrimPrefix(key, "ch-"), nil
	}
	return key, nil
}

func (s *PostService) visiblePost(ctx context.Context, viewerID, postID string) (*model.ChannelPost, error) {
	post, err := s.posts.GetPost(ctx, postID)
	if err != nil {
		return nil, postError(err)
	}
	visible, err := s.canViewPost(ctx, viewerID, *post)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, errcode.New(http.StatusForbidden, "post is not visible").WithReason("post_not_visible")
	}
	return post, nil
}

func (s *PostService) canViewPost(ctx context.Context, viewerID string, post model.ChannelPost) (bool, error) {
	if viewerID != "" && viewerID == post.OwnerID {
		return true, nil
	}
	if s.blocks != nil && viewerID != "" {
		blocked, err := s.blocks.BlocksInteraction(ctx, viewerID, post.OwnerID)
		if err != nil {
			return false, err
		}
		if blocked {
			return false, nil
		}
	}
	switch normalizePostVisibility(post.Visibility) {
	case model.PostVisibilityPublic:
		return true, nil
	case model.PostVisibilityFollowers:
		return s.isFollower(ctx, viewerID, post.ChannelID)
	case model.PostVisibilityPrivate:
		return false, nil
	default:
		return false, nil
	}
}

func (s *PostService) canComment(ctx context.Context, userID string, post model.ChannelPost) (bool, error) {
	if userID == "" || !post.CommentsEnabled {
		return false, nil
	}
	if userID == post.OwnerID {
		return true, nil
	}
	if normalizePostCommentMode(post.CommentMode) == model.PostCommentModeFollowers {
		return s.isFollower(ctx, userID, post.ChannelID)
	}
	return true, nil
}

func (s *PostService) isFollower(ctx context.Context, userID, channelID string) (bool, error) {
	if userID == "" || s.social == nil {
		return false, nil
	}
	return s.social.IsFollowing(ctx, userID, channelID)
}

func (s *PostService) postsToDTO(ctx context.Context, posts []model.ChannelPost, viewerID string) ([]ChannelPostDTO, error) {
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}
	liked, err := s.posts.PostLikedIDs(ctx, viewerID, postIDs)
	if err != nil {
		return nil, err
	}
	authorCache := map[string]PostAuthor{}
	items := make([]ChannelPostDTO, 0, len(posts))
	for _, post := range posts {
		author, ok := authorCache[post.OwnerID]
		if !ok {
			author = s.authorForUser(ctx, post.OwnerID)
			authorCache[post.OwnerID] = author
		}
		dto, err := s.postDTO(ctx, post, viewerID, author, liked[post.ID])
		if err != nil {
			return nil, err
		}
		items = append(items, dto)
	}
	return items, nil
}

func (s *PostService) postDTO(ctx context.Context, post model.ChannelPost, viewerID string, author PostAuthor, liked bool) (ChannelPostDTO, error) {
	canComment, err := s.canComment(ctx, viewerID, post)
	if err != nil {
		return ChannelPostDTO{}, err
	}
	return ChannelPostDTO{
		ID:              post.ID,
		OwnerID:         post.OwnerID,
		ChannelID:       post.ChannelID,
		Content:         post.Content,
		Images:          decodeImages(post.ImagesJSON),
		Visibility:      normalizePostVisibility(post.Visibility),
		CommentsEnabled: post.CommentsEnabled,
		CommentMode:     normalizePostCommentMode(post.CommentMode),
		LikeCount:       post.LikeCount,
		CommentCount:    post.CommentCount,
		Liked:           liked,
		CanComment:      canComment,
		CanDelete:       viewerID != "" && viewerID == post.OwnerID,
		Author:          author,
		CreatedAt:       post.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       post.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *PostService) authorForUser(ctx context.Context, userID string) PostAuthor {
	profile, err := s.posts.UserProfile(ctx, userID)
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

func commentRowDTO(row repo.PostCommentRow, liked, canDelete bool) PostCommentDTO {
	return PostCommentDTO{
		ID:         row.ID,
		PostID:     row.PostID,
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

func commentDTO(comment model.PostComment, author PostAuthor, liked, canDelete bool) PostCommentDTO {
	return PostCommentDTO{
		ID:         comment.ID,
		PostID:     comment.PostID,
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

func (s *PostService) notifyPostComment(ctx context.Context, post model.ChannelPost, comment model.PostComment, parent *model.PostComment) error {
	if s.notify == nil {
		return nil
	}
	targetID := post.OwnerID
	kind := "post_comment"
	title := "有人评论了你的帖子"
	if parent != nil {
		targetID = parent.UserID
		kind = "post_comment_reply"
		title = "有人回复了你的评论"
	}
	if targetID == "" || targetID == comment.UserID {
		return nil
	}
	if blocked, err := blocksBetween(ctx, s.blocks, comment.UserID, targetID); err != nil || blocked {
		return err
	}
	actor := s.authorForUser(ctx, comment.UserID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            kind + "-" + notificationHash(post.ID, comment.ID, targetID),
		UserID:        targetID,
		Type:          kind,
		Title:         title,
		Body:          trimRunes(comment.Content, 120),
		Link:          "/channel/" + post.OwnerID,
		ActorID:       actor.ID,
		ActorUsername: actor.Username,
		ActorName:     actor.Name,
		ActorAvatar:   actor.Avatar,
		ActorVerified: actor.Verified,
		CreatedAt:     time.Now(),
	}})
}

func (s *PostService) notifyPostLiked(ctx context.Context, post model.ChannelPost, actorID string) error {
	if s.notify == nil || post.OwnerID == "" || post.OwnerID == actorID {
		return nil
	}
	actor := s.authorForUser(ctx, actorID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            "post-liked-" + notificationHash(post.ID, actorID),
		UserID:        post.OwnerID,
		Type:          "post_liked",
		Title:         "你的帖子收到新的赞",
		Body:          trimRunes(post.Content, 120),
		Link:          "/channel/" + post.OwnerID,
		ActorID:       actor.ID,
		ActorUsername: actor.Username,
		ActorName:     actor.Name,
		ActorAvatar:   actor.Avatar,
		ActorVerified: actor.Verified,
		CreatedAt:     time.Now(),
	}})
}

func (s *PostService) notifyCommentLiked(ctx context.Context, post model.ChannelPost, comment model.PostComment, actorID string) error {
	if s.notify == nil || comment.UserID == "" || comment.UserID == actorID {
		return nil
	}
	if blocked, err := blocksBetween(ctx, s.blocks, actorID, comment.UserID); err != nil || blocked {
		return err
	}
	actor := s.authorForUser(ctx, actorID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            "comment-liked-" + notificationHash(comment.ID, actorID),
		UserID:        comment.UserID,
		Type:          "post_comment_liked",
		Title:         "你的评论收到新的赞",
		Body:          trimRunes(comment.Content, 120),
		Link:          "/channel/" + post.OwnerID,
		ActorID:       actor.ID,
		ActorUsername: actor.Username,
		ActorName:     actor.Name,
		ActorAvatar:   actor.Avatar,
		ActorVerified: actor.Verified,
		CreatedAt:     time.Now(),
	}})
}

func postError(err error) error {
	switch {
	case errors.Is(err, repo.ErrPostNotFound):
		return errcode.New(http.StatusNotFound, "post not found").WithReason("post_not_found")
	case errors.Is(err, repo.ErrCommentNotFound):
		return errcode.New(http.StatusNotFound, "comment not found").WithReason("comment_not_found")
	default:
		return err
	}
}

func normalizePostVisibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.PostVisibilityFollowers:
		return model.PostVisibilityFollowers
	case model.PostVisibilityPrivate:
		return model.PostVisibilityPrivate
	default:
		return model.PostVisibilityPublic
	}
}

func normalizePostCommentMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.PostCommentModeFollowers:
		return model.PostCommentModeFollowers
	default:
		return model.PostCommentModeEveryone
	}
}

func cleanPostText(value string, maxLen int) string {
	text := strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if maxLen > 0 && len([]rune(text)) > maxLen {
		runes := []rune(text)
		text = string(runes[:maxLen])
	}
	return text
}

func cleanPostImages(images []string) ([]string, error) {
	out := make([]string, 0, len(images))
	seen := map[string]bool{}
	for _, raw := range images {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		if len(value) > 1000 || (!strings.HasPrefix(value, "/") && !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://")) {
			return nil, errcode.New(http.StatusBadRequest, "invalid image url").WithReason("invalid_image")
		}
		out = append(out, value)
		seen[value] = true
		if len(out) > maxPostImages {
			return nil, errcode.New(http.StatusBadRequest, "too many post images").WithReason("too_many_images")
		}
	}
	return out, nil
}

func encodeImages(images []string) string {
	if len(images) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(images)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func decodeImages(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var images []string
	if err := json.Unmarshal([]byte(raw), &images); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(images))
	for _, image := range images {
		if trimmed := strings.TrimSpace(image); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func channelIDForOwner(ownerID string) string {
	if ownerID == "" {
		return ""
	}
	return "ch-" + ownerID
}

func fallbackPostAuthor(userID string) PostAuthor {
	name := userID
	if len(name) > 8 {
		name = "Creator " + name[:8]
	}
	if name == "" {
		name = "Creator"
	}
	return PostAuthor{ID: userID, Name: name}
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "Creator"
}

func normalizeListPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	return page, size
}
