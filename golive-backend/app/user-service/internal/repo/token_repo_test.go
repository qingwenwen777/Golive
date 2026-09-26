package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
)

func newTokenRepo(t *testing.T) (*repo.TokenRepo, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })
	return repo.NewTokenRepo(rdb), mr
}

func TestTokenRepoRotateAtomicallyReplacesRefreshToken(t *testing.T) {
	tokens, mr := newTokenRepo(t)
	ctx := context.Background()
	require.NoError(t, mr.Set("refresh:old", "u-1"))
	mr.SetTTL("refresh:old", time.Hour)

	require.NoError(t, tokens.Rotate(ctx, "old", "new", "u-1", 30*time.Minute))

	require.False(t, mr.Exists("refresh:old"))
	got, err := mr.Get("refresh:new")
	require.NoError(t, err)
	require.Equal(t, "u-1", got)
	require.Greater(t, mr.TTL("refresh:new"), time.Duration(0))
}

func TestTokenRepoRotateMissingOldTokenDoesNotWriteNewToken(t *testing.T) {
	tokens, mr := newTokenRepo(t)

	err := tokens.Rotate(context.Background(), "old", "new", "u-1", time.Hour)

	require.True(t, errors.Is(err, repo.ErrRefreshNotFound))
	require.False(t, mr.Exists("refresh:new"))
}

func TestTokenRepoRotateUserMismatchDoesNotRevokeOldToken(t *testing.T) {
	tokens, mr := newTokenRepo(t)
	require.NoError(t, mr.Set("refresh:old", "u-2"))

	err := tokens.Rotate(context.Background(), "old", "new", "u-1", time.Hour)

	require.True(t, errors.Is(err, repo.ErrRefreshNotFound))
	oldUser, getErr := mr.Get("refresh:old")
	require.NoError(t, getErr)
	require.Equal(t, "u-2", oldUser)
	require.False(t, mr.Exists("refresh:new"))
}

func TestTokenRepoRevokeUserRefreshRevokesEveryTokenOfThatUser(t *testing.T) {
	tokens, mr := newTokenRepo(t)
	ctx := context.Background()
	require.NoError(t, tokens.SaveRefresh(ctx, "a", "u-1", time.Hour))
	require.NoError(t, tokens.SaveRefresh(ctx, "b", "u-1", time.Hour))
	require.NoError(t, tokens.SaveRefresh(ctx, "other", "u-2", time.Hour))
	require.NoError(t, tokens.Rotate(ctx, "b", "c", "u-1", time.Hour))
	require.NoError(t, tokens.SaveRefresh(ctx, "logged-out", "u-1", time.Hour))
	require.NoError(t, tokens.DeleteRefresh(ctx, "logged-out"))
	members, err := mr.SMembers("refresh-user:u-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "c"}, members)

	require.NoError(t, tokens.RevokeUserRefresh(ctx, "u-1"))

	for _, token := range []string{"a", "b", "c"} {
		_, err := tokens.LookupRefresh(ctx, token)
		require.ErrorIs(t, err, repo.ErrRefreshNotFound, token)
	}
	require.False(t, mr.Exists("refresh-user:u-1"))
	got, err := tokens.LookupRefresh(ctx, "other")
	require.NoError(t, err)
	require.Equal(t, "u-2", got)
}
