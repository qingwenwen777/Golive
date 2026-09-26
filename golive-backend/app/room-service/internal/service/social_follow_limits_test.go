package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

// Only real channels can be followed: ResolveOwnerID alone accepts any
// UUID-like key, so arbitrary follows used to be stored and fanned out.
func TestFollowRejectsUnknownChannels(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()

	for _, key := range []string{"ch-6f1c2d4e-0000-4000-8000-000000000001", "6f1c2d4e-0000-4000-8000-000000000002", "no-such-user", "ch-"} {
		_, err := fx.socialS.Follow(ctx, "fan-1", key)
		requireReason(t, err, "channel_not_found")
	}
	keys, err := fx.social.Following(ctx, "fan-1")
	require.NoError(t, err)
	require.Empty(t, keys)

	// An owner known only from their rooms is still a channel.
	roomOwner := "6f1c2d4e-0000-4000-8000-000000000003"
	require.NoError(t, fx.rooms.Upsert(ctx, &model.Room{ID: "room-x", ChannelID: "ch-" + roomOwner, OwnerID: roomOwner, Status: model.StatusEnded, StartedAt: time.Now()}))
	st, err := fx.socialS.Follow(ctx, "fan-1", roomOwner)
	require.NoError(t, err)
	require.Equal(t, "ch-"+roomOwner, st.ChannelID)
}

func TestFollowIsCappedPerUser(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	fx.socialS.SetMaxFollows(2)

	_, err := fx.socialS.Follow(ctx, "fan-1", "ch-creator-1")
	require.NoError(t, err)
	_, err = fx.socialS.Follow(ctx, "fan-1", "ch-creator-2")
	require.NoError(t, err)
	_, err = fx.socialS.Follow(ctx, "fan-1", "ch-commenter-1")
	requireReason(t, err, "follow_limit")
	count, err := fx.social.FollowerCount(ctx, "ch-commenter-1")
	require.NoError(t, err)
	require.Zero(t, count)

	// Re-following an existing channel is not a new follow.
	_, err = fx.socialS.Follow(ctx, "fan-1", "creator_one")
	require.NoError(t, err)

	_, err = fx.socialS.Unfollow(ctx, "fan-1", "ch-creator-2")
	require.NoError(t, err)
	_, err = fx.socialS.Follow(ctx, "fan-1", "ch-commenter-1")
	require.NoError(t, err)
}

// Follows stored under usernames before keys were normalised are rewritten
// once, so later subscription pages do not scan users by display name.
func TestListSubscriptionsMigratesLegacyFollowKeys(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	var nameScans int
	countNameScans := func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "display_name = ?") {
			nameScans++
		}
	}
	require.NoError(t, fx.db.Callback().Row().After("gorm:row").Register("test:name_scans", countNameScans))
	require.NoError(t, fx.db.Callback().Query().After("gorm:query").Register("test:name_scans_query", countNameScans))
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "ch-creator-2"))
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "creator_one"))
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "renamed_user"))

	resp, err := fx.socialS.ListSubscriptions(ctx, "fan-1")
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	require.Equal(t, "ch-creator-1", resp.Items[0].ChannelID)
	require.EqualValues(t, 1, resp.Items[0].SubscriberCount)
	require.Equal(t, "ch-creator-2", resp.Items[1].ChannelID)
	require.Positive(t, nameScans)

	keys, err := fx.social.Following(ctx, "fan-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ch-creator-1", "ch-creator-2"}, keys)
	following, err := fx.socialS.GetFollow(ctx, "fan-1", "ch-creator-1")
	require.NoError(t, err)
	require.True(t, following.Following)

	nameScans = 0
	resp, err = fx.socialS.ListSubscriptions(ctx, "fan-1")
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	latest, err := fx.posts.ListSubscriptionLatest(ctx, "fan-1", 10)
	require.NoError(t, err)
	require.Empty(t, latest.Items)
	require.Zero(t, nameScans)
}
