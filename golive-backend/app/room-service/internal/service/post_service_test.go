package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

type allowPostPermission struct{}

func (allowPostPermission) HasApprovedLivePermission(context.Context, string) (bool, error) {
	return true, nil
}

type postFixture struct {
	svc    *service.PostService
	social *repo.SocialRepo
	db     *gorm.DB
}

func newPostFixture(t *testing.T) postFixture {
	t.Helper()
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	posts := repo.NewPostRepo(db)
	require.NoError(t, posts.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE users (
	id varchar(36) primary key,
	username varchar(64),
	display_name varchar(64),
	avatar varchar(500),
	verified boolean,
	live_permission_status varchar(16),
	updated_at datetime
)`).Error)
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"creator-1", "creator", "Creator", "/avatar.png", true, "approved", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"fan-1", "fan", "Fan", "", false, "none", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"viewer-1", "viewer", "Viewer", "", false, "none", now,
	).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	social := repo.NewSocialRepo(rdb)
	require.NoError(t, social.Follow(ctx, "fan-1", "ch-creator-1"))
	return postFixture{
		svc:    service.NewPostService(posts, rooms, social, allowPostPermission{}),
		social: social,
		db:     db,
	}
}

// A failed post query for a non-owner used to be swallowed by a shadowed err
// and returned as an empty page.
func TestListChannelReturnsQueryErrorForViewers(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	require.NoError(t, fx.db.Exec("DROP TABLE channel_posts").Error)

	_, err := fx.svc.ListChannel(ctx, "fan-1", "creator", 1, 10)
	require.Error(t, err)
	_, err = fx.svc.ListChannel(ctx, "", "creator", 1, 10)
	require.Error(t, err)
}

func TestPosts_VisibilityAndFollowerComments(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	enabled := true
	post, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "fan note",
		Visibility:      "followers",
		CommentsEnabled: &enabled,
		CommentMode:     "followers",
	})
	require.NoError(t, err)
	require.Equal(t, "followers", post.Visibility)

	guest, err := fx.svc.ListChannel(ctx, "", "creator", 1, 10)
	require.NoError(t, err)
	require.Empty(t, guest.Items)

	fan, err := fx.svc.ListChannel(ctx, "fan-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Len(t, fan.Items, 1)
	require.True(t, fan.Items[0].CanComment)

	outsider, err := fx.svc.ListChannel(ctx, "viewer-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Empty(t, outsider.Items)

	_, err = fx.svc.CreateComment(ctx, "viewer-1", post.ID, service.CreateCommentReq{Content: "hi"})
	require.Error(t, err)

	comment, err := fx.svc.CreateComment(ctx, "fan-1", post.ID, service.CreateCommentReq{Content: "first"})
	require.NoError(t, err)
	require.Equal(t, 0, comment.Depth)

	owner, err := fx.svc.ListChannel(ctx, "creator-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Len(t, owner.Items, 1)
	require.True(t, owner.Items[0].CanDelete)
}

func TestPosts_DisabledCommentsStayDisabled(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	disabled := false
	post, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "quiet note",
		Visibility:      "public",
		CommentsEnabled: &disabled,
		CommentMode:     "everyone",
	})
	require.NoError(t, err)
	require.False(t, post.CommentsEnabled)
	require.False(t, post.CanComment)

	listed, err := fx.svc.ListChannel(ctx, "viewer-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	require.False(t, listed.Items[0].CommentsEnabled)
	require.False(t, listed.Items[0].CanComment)

	_, err = fx.svc.CreateComment(ctx, "viewer-1", post.ID, service.CreateCommentReq{Content: "should fail"})
	require.Error(t, err)
}

func TestPosts_OwnerCanUpdateVisibility(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	enabled := true
	post, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "visibility note",
		Visibility:      "public",
		CommentsEnabled: &enabled,
	})
	require.NoError(t, err)

	updated, err := fx.svc.UpdatePostVisibility(ctx, "creator-1", post.ID, service.UpdatePostVisibilityReq{Visibility: "private"})
	require.NoError(t, err)
	require.Equal(t, "private", updated.Visibility)

	viewer, err := fx.svc.ListChannel(ctx, "viewer-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Empty(t, viewer.Items)

	owner, err := fx.svc.ListChannel(ctx, "creator-1", "creator", 1, 10)
	require.NoError(t, err)
	require.Len(t, owner.Items, 1)
	require.Equal(t, "private", owner.Items[0].Visibility)

	_, err = fx.svc.UpdatePostVisibility(ctx, "viewer-1", post.ID, service.UpdatePostVisibilityReq{Visibility: "public"})
	require.Error(t, err)
}

func TestPosts_SubscriptionLatestReturnsLatestVisiblePerFollowedCreator(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	enabled := true
	_, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "old public",
		Visibility:      "public",
		CommentsEnabled: &enabled,
	})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	latestVisible, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "latest visible",
		Visibility:      "followers",
		CommentsEnabled: &enabled,
	})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	_, err = fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "private newer",
		Visibility:      "private",
		CommentsEnabled: &enabled,
	})
	require.NoError(t, err)

	resp, err := fx.svc.ListSubscriptionLatest(ctx, "fan-1", 8)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, latestVisible.ID, resp.Items[0].ID)
	require.Equal(t, "latest visible", resp.Items[0].Content)

	empty, err := fx.svc.ListSubscriptionLatest(ctx, "viewer-1", 8)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
}

func TestPosts_ReplyDepthAndOwnerDeletesThread(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	enabled := true
	post, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "open note",
		Visibility:      "public",
		CommentsEnabled: &enabled,
		CommentMode:     "everyone",
	})
	require.NoError(t, err)

	root, err := fx.svc.CreateComment(ctx, "viewer-1", post.ID, service.CreateCommentReq{Content: "root"})
	require.NoError(t, err)
	reply1, err := fx.svc.CreateComment(ctx, "fan-1", post.ID, service.CreateCommentReq{Content: "reply 1", ParentID: root.ID})
	require.NoError(t, err)
	require.Equal(t, 1, reply1.Depth)
	reply2, err := fx.svc.CreateComment(ctx, "creator-1", post.ID, service.CreateCommentReq{Content: "reply 2", ParentID: reply1.ID})
	require.NoError(t, err)
	require.Equal(t, 2, reply2.Depth)

	_, err = fx.svc.CreateComment(ctx, "viewer-1", post.ID, service.CreateCommentReq{Content: "too deep", ParentID: reply2.ID})
	require.Error(t, err)

	require.NoError(t, fx.svc.DeleteComment(ctx, "creator-1", post.ID, root.ID))
	comments, err := fx.svc.ListComments(ctx, "creator-1", post.ID)
	require.NoError(t, err)
	require.Empty(t, comments.Items)
}

func TestPosts_LikeIsIdempotent(t *testing.T) {
	fx := newPostFixture(t)
	ctx := context.Background()
	enabled := true
	post, err := fx.svc.CreatePost(ctx, "creator-1", service.CreatePostReq{
		Content:         "like me",
		Visibility:      "public",
		CommentsEnabled: &enabled,
	})
	require.NoError(t, err)

	state, err := fx.svc.LikePost(ctx, "viewer-1", post.ID)
	require.NoError(t, err)
	require.True(t, state.Liked)
	require.EqualValues(t, 1, state.Likes)

	state, err = fx.svc.LikePost(ctx, "viewer-1", post.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, state.Likes)

	state, err = fx.svc.UnlikePost(ctx, "viewer-1", post.ID)
	require.NoError(t, err)
	require.False(t, state.Liked)
	require.EqualValues(t, 0, state.Likes)

	state, err = fx.svc.UnlikePost(ctx, "viewer-1", post.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, state.Likes)
}
