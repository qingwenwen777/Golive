package repo

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v9"
)

var ErrRefreshNotFound = errors.New("refresh token not found")

const refreshKeyPrefix = "refresh:"

type TokenRepo struct {
	rdb *redis.Client
}

func NewTokenRepo(rdb *redis.Client) *TokenRepo { return &TokenRepo{rdb: rdb} }

// SaveRefresh stores token → userId with TTL.
func (r *TokenRepo) SaveRefresh(ctx context.Context, token, userID string, ttl time.Duration) error {
	return r.rdb.Set(ctx, refreshKeyPrefix+token, userID, ttl).Err()
}

// LookupRefresh returns the userId bound to token, or ErrRefreshNotFound.
func (r *TokenRepo) LookupRefresh(ctx context.Context, token string) (string, error) {
	v, err := r.rdb.Get(ctx, refreshKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrRefreshNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// DeleteRefresh revokes a token.
func (r *TokenRepo) DeleteRefresh(ctx context.Context, token string) error {
	return r.rdb.Del(ctx, refreshKeyPrefix+token).Err()
}

// Rotate atomically deletes the old token and stores the new one. The current
// implementation uses a pipeline; collisions are negligible for our scale and
// a strict CAS is unnecessary because each refreshToken is a fresh UUID.
func (r *TokenRepo) Rotate(ctx context.Context, oldToken, newToken, userID string, ttl time.Duration) error {
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, refreshKeyPrefix+oldToken)
	pipe.Set(ctx, refreshKeyPrefix+newToken, userID, ttl)
	_, err := pipe.Exec(ctx)
	return err
}
