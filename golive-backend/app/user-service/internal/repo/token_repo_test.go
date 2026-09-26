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

// Refresh tokens saved before the per-user index existed are indexed by a
// one-off scan, so a ban revokes them too.
func TestTokenRepoIndexLegacyRefreshTokensOnce(t *testing.T) {
	tokens, mr := newTokenRepo(t)
	ctx := context.Background()
	legacy := func(token, userID string, ttl time.Duration) {
		t.Helper()
		require.NoError(t, mr.Set("refresh:"+token, userID))
		if ttl > 0 {
			mr.SetTTL("refresh:"+token, ttl)
		}
	}
	legacy("legacy-a", "u-1", 2*time.Hour)
	legacy("legacy-b", "u-1", 3*time.Hour)
	legacy("legacy-other", "u-2", time.Hour)
	legacy("legacy-forever", "u-3", 0)
	require.NoError(t, tokens.SaveRefresh(ctx, "current", "u-1", time.Hour))

	indexed, err := tokens.IndexLegacyRefreshTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, indexed)
	members, err := mr.SMembers("refresh-user:u-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"legacy-a", "legacy-b", "current"}, members)
	// Each index outlives every token in it.
	require.Equal(t, 3*time.Hour, mr.TTL("refresh-user:u-1"))
	require.Equal(t, time.Hour, mr.TTL("refresh-user:u-2"))
	require.True(t, mr.Exists("refresh-user:u-3"))
	require.Zero(t, mr.TTL("refresh-user:u-3"), "a token without expiry keeps its index")

	// It runs once: tokens saved later are indexed by SaveRefresh / Rotate.
	legacy("late", "u-2", time.Hour)
	indexed, err = tokens.IndexLegacyRefreshTokens(ctx)
	require.NoError(t, err)
	require.Zero(t, indexed)
	isMember, err := mr.SIsMember("refresh-user:u-2", "late")
	require.NoError(t, err)
	require.False(t, isMember)

	require.NoError(t, tokens.RevokeUserRefresh(ctx, "u-1"))
	for _, token := range []string{"legacy-a", "legacy-b", "current"} {
		_, err := tokens.LookupRefresh(ctx, token)
		require.ErrorIs(t, err, repo.ErrRefreshNotFound, token)
	}
	got, err := tokens.LookupRefresh(ctx, "legacy-other")
	require.NoError(t, err)
	require.Equal(t, "u-2", got)
}
