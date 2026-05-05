package repo

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
)

var ErrRefreshNotFound = errors.New("refresh token not found")

const (
	refreshKeyPrefix       = "refresh:"
	loginFailKeyPrefix     = "login:fail:"
	loginCooldownKeyPrefix = "login:cooldown:"
)

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

func (r *TokenRepo) LoginCooldown(ctx context.Context, username string) (time.Duration, bool, error) {
	ttl, err := r.rdb.TTL(ctx, loginCooldownKey(username)).Result()
	if err != nil {
		return 0, false, err
	}
	if ttl > 0 {
		return ttl, true, nil
	}
	return 0, false, nil
}

func (r *TokenRepo) RecordLoginFailure(ctx context.Context, username string, window, cooldown time.Duration, maxAttempts int) (time.Duration, bool, error) {
	if window <= 0 {
		window = 5 * time.Minute
	}
	if cooldown <= 0 {
		cooldown = time.Minute
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	key := loginFailKey(username)
	count, err := r.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, false, err
	}
	if count == 1 {
		if err := r.rdb.Expire(ctx, key, window).Err(); err != nil {
			return 0, false, err
		}
	} else if ttl, err := r.rdb.TTL(ctx, key).Result(); err != nil {
		return 0, false, err
	} else if ttl < 0 {
		if err := r.rdb.Expire(ctx, key, window).Err(); err != nil {
			return 0, false, err
		}
	}
	if count < int64(maxAttempts) {
		return 0, false, nil
	}
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.Set(ctx, loginCooldownKey(username), "1", cooldown)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, false, err
	}
	return cooldown, true, nil
}

func (r *TokenRepo) ClearLoginFailures(ctx context.Context, username string) error {
	return r.rdb.Del(ctx, loginFailKey(username), loginCooldownKey(username)).Err()
}

func loginFailKey(username string) string {
	return loginFailKeyPrefix + loginAttemptIdentity(username)
}

func loginCooldownKey(username string) string {
	return loginCooldownKeyPrefix + loginAttemptIdentity(username)
}

func loginAttemptIdentity(username string) string {
	clean := strings.ToLower(strings.TrimSpace(username))
	if clean == "" {
		clean = "_"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(clean))
}
