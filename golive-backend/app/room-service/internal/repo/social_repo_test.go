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
