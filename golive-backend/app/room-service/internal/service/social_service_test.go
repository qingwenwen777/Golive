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

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

func newSocialSvc(t *testing.T) (*service.SocialService, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return service.NewSocialService(repo.NewSocialRepo(rdb)), mr
}

func TestFollow_PopulatesSubscriberCountAndListing(t *testing.T) {
	svc, _ := newSocialSvc(t)
	ctx := context.Background()

	// Two viewers follow the same channel.
	st, err := svc.Follow(ctx, "viewer-A", "ch-creator")
	require.NoError(t, err)
	require.True(t, st.Following)
	require.Equal(t, int64(1), st.SubscriberCount, "Follow should report fresh count")

	st, err = svc.Follow(ctx, "viewer-B", "ch-creator")
	require.NoError(t, err)
	require.Equal(t, int64(2), st.SubscriberCount)

	// GetFollow from a third viewer (not following) sees count=2 but following=false.
	st, err = svc.GetFollow(ctx, "viewer-C", "ch-creator")
	require.NoError(t, err)
	require.False(t, st.Following)
	require.Equal(t, int64(2), st.SubscriberCount)

	// ListSubscriptions for viewer-A returns ch-creator.
	resp, err := svc.ListSubscriptions(ctx, "viewer-A")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "ch-creator", resp.Items[0].ChannelID)
	require.Equal(t, int64(2), resp.Items[0].SubscriberCount)
	require.NotNil(t, resp.Items[0].Stream, "stream placeholder must be set so the grid can render the channel")
}

func TestFollow_Toggle(t *testing.T) {
	svc, _ := newSocialSvc(t)
	ctx := context.Background()

	st, err := svc.GetFollow(ctx, "u1", "ch1")
	require.NoError(t, err)
	require.False(t, st.Following)
	require.Equal(t, "ch1", st.ChannelID)

	st, _ = svc.Follow(ctx, "u1", "ch1")
	require.True(t, st.Following)

	st, _ = svc.GetFollow(ctx, "u1", "ch1")
	require.True(t, st.Following)

	st, _ = svc.Unfollow(ctx, "u1", "ch1")
	require.False(t, st.Following)
}

func TestRecommendedCreatorsRanksByFollowersAndRecency(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
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

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"creator-a", "luna", "Luna", "luna.png", true, "approved", now.Add(-time.Hour),
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"creator-b", "mika", "Mika", "", false, "approved", now.Add(-2*time.Hour),
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, live_permission_status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"viewer-1", "viewer", "Viewer", "", false, "approved", now,
	).Error)

	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:          "live-a-old",
		Title:       "Luna comeback",
		Channel:     "Luna Space",
		ChannelID:   "ch-creator-a",
		Avatar:      "luna-room.png",
		Category:    "Music",
		StartedAt:   now.Add(-24 * time.Hour),
		Status:      model.StatusEnded,
		OwnerID:     "creator-a",
		PeakViewers: 100,
	}))
	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:          "live-b-old",
		Title:       "Mika archive",
		Channel:     "Mika Lab",
		ChannelID:   "ch-creator-b",
		Category:    "Gaming",
		StartedAt:   now.Add(-60 * 24 * time.Hour),
		Status:      model.StatusEnded,
		OwnerID:     "creator-b",
		PeakViewers: 10,
	}))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	svc := service.NewSocialService(repo.NewSocialRepo(rdb), rooms)
	_, err = svc.Follow(ctx, "fan-1", "ch-creator-a")
	require.NoError(t, err)
	_, err = svc.Follow(ctx, "viewer-1", "ch-creator-a")
	require.NoError(t, err)

	resp, err := svc.RecommendedCreators(ctx, "viewer-1", 4)
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	require.Equal(t, "creator-a", resp.Items[0].ID)
	require.Equal(t, "ch-creator-a", resp.Items[0].ChannelID)
	require.Equal(t, "Luna Space", resp.Items[0].Name)
	require.EqualValues(t, 2, resp.Items[0].SubscriberCount)
	require.True(t, resp.Items[0].Following)
	require.NotEmpty(t, resp.Items[0].LastLiveAt)
	require.Equal(t, "Luna comeback", resp.Items[0].LastTitle)
	require.Equal(t, "creator-b", resp.Items[1].ID)
}

// The big one: like / dislike state machine. We replay the same sequence the
// frontend optimistic logic in src/api/room.ts predicts and assert each step.
func TestLikeDislike_StateMachine(t *testing.T) {
	svc, mr := newSocialSvc(t)
	ctx := context.Background()
	const sid, uid = "rm-x", "u1"

	// Pre-seed counter at 100 to mirror the bootstrap path.
	require.NoError(t, mr.Set("like:rm-x:count", "100"))

	// 1) Initial GET → liked=false, disliked=false, likes=100
	got, err := svc.GetLike(ctx, uid, sid)
	require.NoError(t, err)
	require.Equal(t, int64(100), got.Likes)
	require.False(t, got.Liked)
	require.False(t, got.Disliked)

	// 2) Like → likes=101, liked=true
	got, _ = svc.Like(ctx, uid, sid)
	require.Equal(t, int64(101), got.Likes)
	require.True(t, got.Liked)
	require.False(t, got.Disliked)

	// 3) Like again (idempotent on counter — already liked) → likes still 101
	got, _ = svc.Like(ctx, uid, sid)
	require.Equal(t, int64(101), got.Likes)
	require.True(t, got.Liked)

	// 4) Dislike → liked=false, likes=100, disliked=true
	got, _ = svc.Dislike(ctx, uid, sid)
	require.Equal(t, int64(100), got.Likes)
	require.False(t, got.Liked)
	require.True(t, got.Disliked)

	// 5) Like (transitions from disliked) → likes=101, liked=true, disliked=false
	got, _ = svc.Like(ctx, uid, sid)
	require.Equal(t, int64(101), got.Likes)
	require.True(t, got.Liked)
	require.False(t, got.Disliked)

	// 6) Unlike → likes=100, liked=false (disliked stays false)
	got, _ = svc.Unlike(ctx, uid, sid)
	require.Equal(t, int64(100), got.Likes)
	require.False(t, got.Liked)
	require.False(t, got.Disliked)

	// 7) Unlike again → no underflow, stays at 100
	got, _ = svc.Unlike(ctx, uid, sid)
	require.Equal(t, int64(100), got.Likes)

	// 8) Dislike → counter unchanged (was already not liked)
	got, _ = svc.Dislike(ctx, uid, sid)
	require.Equal(t, int64(100), got.Likes)
	require.True(t, got.Disliked)

	// 9) Undislike → disliked=false
	got, _ = svc.Undislike(ctx, uid, sid)
	require.False(t, got.Disliked)
	require.Equal(t, int64(100), got.Likes)
}

func TestLikeDislike_PerUserIsolation(t *testing.T) {
	svc, mr := newSocialSvc(t)
	ctx := context.Background()
	require.NoError(t, mr.Set("like:rm-y:count", "10"))

	_, _ = svc.Like(ctx, "alice", "rm-y")
	_, _ = svc.Like(ctx, "bob", "rm-y")
	got, _ := svc.GetLike(ctx, "alice", "rm-y")
	require.Equal(t, int64(12), got.Likes)
	require.True(t, got.Liked)

	gotBob, _ := svc.GetLike(ctx, "bob", "rm-y")
	require.True(t, gotBob.Liked)

	gotCarol, _ := svc.GetLike(ctx, "carol", "rm-y") // never interacted
	require.False(t, gotCarol.Liked)
	require.False(t, gotCarol.Disliked)
	require.Equal(t, int64(12), gotCarol.Likes)
}
