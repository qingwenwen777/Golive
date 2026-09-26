package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// blockFixture wires the social, post, replay-comment and message services
// on one database, with MessageService as the real block checker.
type blockFixture struct {
	db       *gorm.DB
	rooms    *repo.RoomRepo
	social   *repo.SocialRepo
	messages *repo.MessageRepo
	msgSvc   *service.MessageService
	socialS  *service.SocialService
	posts    *service.PostService
	replays  *service.ReplayCommentService
}

func newBlockFixture(t *testing.T) blockFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	postRepo := repo.NewPostRepo(db)
	require.NoError(t, postRepo.AutoMigrate())
	replayRepo := repo.NewReplayCommentRepo(db)
	require.NoError(t, replayRepo.AutoMigrate())
	messages := repo.NewMessageRepo(db)
	require.NoError(t, messages.AutoMigrate())
	require.NoError(t, repo.NewAppointmentRepo(db).AutoMigrate())
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
	for _, u := range []struct{ id, username, status string }{
		{"creator-1", "creator_one", "approved"},
		{"creator-2", "creator_two", "approved"},
		{"fan-1", "fan", "none"},
		{"commenter-1", "commenter", "none"},
	} {
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, '', false, ?, ?)`,
			u.id, u.username, u.username, u.status, time.Now(),
		).Error)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	social := repo.NewSocialRepo(rdb)

	msgSvc := service.NewMessageService(messages, rooms, social)
	socialSvc := service.NewSocialService(social, rooms)
	socialSvc.SetBlockChecker(msgSvc)
	postSvc := service.NewPostService(postRepo, rooms, social, allowPostPermission{})
	postSvc.SetBlockChecker(msgSvc)
	postSvc.SetNotificationWriter(messages)
	replaySvc := service.NewReplayCommentService(replayRepo, rooms, nil)
	replaySvc.SetBlockChecker(msgSvc)
	replaySvc.SetNotificationWriter(messages)
	return blockFixture{
		db:       db,
		rooms:    rooms,
		social:   social,
		messages: messages,
		msgSvc:   msgSvc,
		socialS:  socialSvc,
		posts:    postSvc,
		replays:  replaySvc,
	}
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr), "expected %s error, got %v", reason, err)
	require.Equal(t, reason, appErr.Reason)
}

func (fx blockFixture) notificationCount(t *testing.T, userID, kind string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, fx.db.Model(&model.Notification{}).Where("user_id = ? AND type = ?", userID, kind).Count(&count).Error)
	return count
}

func TestBlockUserRemovesFollowsBothWays(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "ch-creator-1"))
	require.NoError(t, fx.social.Follow(ctx, "creator-1", "ch-fan-1"))

	require.NoError(t, fx.msgSvc.BlockUser(ctx, "creator-1", "fan-1", service.BlockUserReq{}))

	following, err := fx.social.IsFollowing(ctx, "fan-1", "ch-creator-1")
	require.NoError(t, err)
	require.False(t, following)
	following, err = fx.social.IsFollowing(ctx, "creator-1", "ch-fan-1")
	require.NoError(t, err)
	require.False(t, following)
	count, err := fx.social.FollowerCount(ctx, "ch-creator-1")
	require.NoError(t, err)
	require.Zero(t, count)
}

// Follows that predate a block (or survive it under a legacy key) must not
// keep delivering the creator's followers-only posts or channel card.
func TestSubscriptionFeedsHideBlockedCreators(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	enabled := true
	_, err := fx.posts.CreatePost(ctx, "creator-1", service.CreatePostReq{Content: "fans only", Visibility: "followers", CommentsEnabled: &enabled})
	require.NoError(t, err)
	_, err = fx.posts.CreatePost(ctx, "creator-2", service.CreatePostReq{Content: "other", Visibility: "public", CommentsEnabled: &enabled})
	require.NoError(t, err)
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "ch-creator-1"))
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "creator_one"))
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "ch-creator-2"))

	require.NoError(t, fx.messages.UpsertBlock(ctx, "creator-1", "fan-1", "user", "", time.Now()))

	latest, err := fx.posts.ListSubscriptionLatest(ctx, "fan-1", 10)
	require.NoError(t, err)
	require.Len(t, latest.Items, 1)
	require.Equal(t, "creator-2", latest.Items[0].OwnerID)

	subs, err := fx.socialS.ListSubscriptions(ctx, "fan-1")
	require.NoError(t, err)
	require.Len(t, subs.Items, 1)
	require.Equal(t, "ch-creator-2", subs.Items[0].ChannelID)
}

func TestFollowNormalisesKeyAndChecksBlockForUsernames(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()

	st, err := fx.socialS.Follow(ctx, "fan-1", "creator_one")
	require.NoError(t, err)
	require.Equal(t, "ch-creator-1", st.ChannelID)
	require.True(t, st.Following)
	following, err := fx.social.IsFollowing(ctx, "fan-1", "ch-creator-1")
	require.NoError(t, err)
	require.True(t, following, "username follows must be stored under the canonical channel key")
	keys, err := fx.social.Following(ctx, "fan-1")
	require.NoError(t, err)
	require.Equal(t, []string{"ch-creator-1"}, keys)

	st, err = fx.socialS.GetFollow(ctx, "fan-1", "ch-creator_one")
	require.NoError(t, err)
	require.True(t, st.Following)
	require.EqualValues(t, 1, st.SubscriberCount)

	_, err = fx.socialS.Unfollow(ctx, "fan-1", "creator_one")
	require.NoError(t, err)
	require.NoError(t, fx.messages.UpsertBlock(ctx, "creator-1", "fan-1", "user", "", time.Now()))
	for _, key := range []string{"creator_one", "ch-creator_one", "ch-creator-1", "creator-1"} {
		_, err = fx.socialS.Follow(ctx, "fan-1", key)
		requireReason(t, err, "channel_blocked")
	}
	_, err = fx.socialS.Follow(ctx, "creator-1", "creator_one")
	requireReason(t, err, "self_follow")
	_, err = fx.socialS.Follow(ctx, "fan-1", "nobody_here")
	requireReason(t, err, "channel_not_found")
}

// C blocks B: B must not reply to or like C's comment on A's post, and C
// must not be notified by B.
func TestPostCommentRepliesAndLikesRespectCommentAuthorBlocks(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	enabled := true
	post, err := fx.posts.CreatePost(ctx, "creator-1", service.CreatePostReq{Content: "open", Visibility: "public", CommentsEnabled: &enabled})
	require.NoError(t, err)
	comment, err := fx.posts.CreateComment(ctx, "commenter-1", post.ID, service.CreateCommentReq{Content: "hello"})
	require.NoError(t, err)
	require.NoError(t, fx.messages.UpsertBlock(ctx, "commenter-1", "fan-1", "user", "", time.Now()))

	_, err = fx.posts.CreateComment(ctx, "fan-1", post.ID, service.CreateCommentReq{Content: "reply", ParentID: comment.ID})
	requireReason(t, err, "user_blocked")
	_, err = fx.posts.LikeComment(ctx, "fan-1", post.ID, comment.ID)
	requireReason(t, err, "user_blocked")
	require.Zero(t, fx.notificationCount(t, "commenter-1", "post_comment_reply"))
	require.Zero(t, fx.notificationCount(t, "commenter-1", "post_comment_liked"))

	// Unrelated users are unaffected.
	_, err = fx.posts.CreateComment(ctx, "creator-2", post.ID, service.CreateCommentReq{Content: "reply", ParentID: comment.ID})
	require.NoError(t, err)
	_, err = fx.posts.LikeComment(ctx, "creator-2", post.ID, comment.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, fx.notificationCount(t, "commenter-1", "post_comment_reply"))
	require.EqualValues(t, 1, fx.notificationCount(t, "commenter-1", "post_comment_liked"))
}

func TestReplayCommentRepliesAndLikesRespectCommentAuthorBlocks(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	require.NoError(t, fx.rooms.Upsert(ctx, &model.Room{
		ID:                 "replay-1",
		Title:              "Replay",
		ChannelID:          "ch-creator-1",
		OwnerID:            "creator-1",
		Status:             model.StatusEnded,
		StartedAt:          time.Now().Add(-time.Hour),
		ReplayStatus:       model.ReplayStatusReady,
		ReplayVisibility:   model.PostVisibilityPublic,
		ReplayBunnyVideoID: "video-1",
	}))
	comment, err := fx.replays.Create(ctx, "commenter-1", "replay-1", service.CreateCommentReq{Content: "hello"})
	require.NoError(t, err)
	require.NoError(t, fx.messages.UpsertBlock(ctx, "commenter-1", "fan-1", "user", "", time.Now()))

	_, err = fx.replays.Create(ctx, "fan-1", "replay-1", service.CreateCommentReq{Content: "reply", ParentID: comment.ID})
	requireReason(t, err, "user_blocked")
	_, err = fx.replays.Like(ctx, "fan-1", "replay-1", comment.ID)
	requireReason(t, err, "user_blocked")
	require.Zero(t, fx.notificationCount(t, "commenter-1", "replay_comment_reply"))
	require.Zero(t, fx.notificationCount(t, "commenter-1", "replay_comment_liked"))

	// A block with the replay owner also stops likes, as it already stops comments.
	require.NoError(t, fx.messages.UpsertBlock(ctx, "creator-1", "creator-2", "user", "", time.Now()))
	_, err = fx.replays.Like(ctx, "creator-2", "replay-1", comment.ID)
	requireReason(t, err, "user_blocked")
}
