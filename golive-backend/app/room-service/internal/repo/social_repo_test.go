package repo_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func TestFollowerCountsReturnsCountsForUniqueChannels(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	social := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	require.NoError(t, social.Follow(ctx, "u-1", "ch-a"))
	require.NoError(t, social.Follow(ctx, "u-2", "ch-a"))
	require.NoError(t, social.Follow(ctx, "u-3", "ch-b"))

	got, err := social.FollowerCounts(ctx, []string{"ch-a", "ch-b", "ch-a", ""})
	require.NoError(t, err)
	require.Equal(t, int64(2), got["ch-a"])
	require.Equal(t, int64(1), got["ch-b"])
	require.NotContains(t, got, "")
}

func TestFollowWithLimitRefusesNewFollowsPastTheCap(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	social := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	for _, channelID := range []string{"ch-a", "ch-b"} {
		ok, err := social.FollowWithLimit(ctx, "u-1", channelID, 2)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err := social.FollowWithLimit(ctx, "u-1", "ch-c", 2)
	require.NoError(t, err)
	require.False(t, ok)
	count, err := social.FollowerCount(ctx, "ch-c")
	require.NoError(t, err)
	require.Zero(t, count)

	ok, err = social.FollowWithLimit(ctx, "u-1", "ch-a", 2)
	require.NoError(t, err)
	require.True(t, ok, "re-following an existing channel is always allowed")
	ok, err = social.FollowWithLimit(ctx, "u-1", "ch-c", 0)
	require.NoError(t, err)
	require.True(t, ok, "a non-positive limit disables the cap")
}

func TestReplaceFollowKeyMovesOrDropsAFollow(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	social := repo.NewSocialRepo(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	require.NoError(t, social.Follow(ctx, "u-1", "legacy_name"))
	require.NoError(t, social.Follow(ctx, "u-1", "gone_name"))

	require.NoError(t, social.ReplaceFollowKey(ctx, "u-1", "legacy_name", "ch-owner"))
	require.NoError(t, social.ReplaceFollowKey(ctx, "u-1", "gone_name", ""))
	require.NoError(t, social.ReplaceFollowKey(ctx, "u-1", "never_followed", "ch-other"))

	keys, err := social.Following(ctx, "u-1")
	require.NoError(t, err)
	require.Equal(t, []string{"ch-owner"}, keys)
	for channelID, want := range map[string]int64{"ch-owner": 1, "legacy_name": 0, "gone_name": 0, "ch-other": 0} {
		count, err := social.FollowerCount(ctx, channelID)
		require.NoError(t, err)
		require.Equal(t, want, count, channelID)
	}
}
