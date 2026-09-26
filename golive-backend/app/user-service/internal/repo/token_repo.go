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
	refreshKeyPrefix = "refresh:"
	// refreshUserKeyPrefix indexes each user's refresh tokens (a set) so all
	// of them can be revoked at once, e.g. when the user is banned.
	refreshUserKeyPrefix   = "refresh-user:"
	loginFailKeyPrefix     = "login:fail:"
	loginCooldownKeyPrefix = "login:cooldown:"
)

var rotateRefreshScript = redis.NewScript(`
local oldKey = KEYS[1]
local newKey = KEYS[2]
local userKey = KEYS[3]
local userID = ARGV[1]
local ttlMillis = tonumber(ARGV[2])
local oldToken = ARGV[3]
local newToken = ARGV[4]

local current = redis.call("GET", oldKey)
if not current then
	return 0
end
if current ~= userID then
	return 0
end

redis.call("DEL", oldKey)
redis.call("SREM", userKey, oldToken)
redis.call("SADD", userKey, newToken)
if ttlMillis and ttlMillis > 0 then
	redis.call("SET", newKey, userID, "PX", ttlMillis)
	redis.call("PEXPIRE", userKey, ttlMillis)
else
	redis.call("SET", newKey, userID)
end
return 1
`)

// revokeUserRefreshScript deletes every refresh token in a user's index and
// the index itself in one step, so no token recorded there survives.
var revokeUserRefreshScript = redis.NewScript(`
local userKey = KEYS[1]
local prefix = ARGV[1]
local tokens = redis.call("SMEMBERS", userKey)
for _, token in ipairs(tokens) do
	redis.call("DEL", prefix .. token)
end
redis.call("DEL", userKey)
return #tokens
`)

type TokenRepo struct {
	rdb *redis.Client
}

func NewTokenRepo(rdb *redis.Client) *TokenRepo { return &TokenRepo{rdb: rdb} }

// SaveRefresh stores token → userId with TTL and records the token in the
// user's index.
func (r *TokenRepo) SaveRefresh(ctx context.Context, token, userID string, ttl time.Duration) error {
	pipe := r.rdb.TxPipeline()
	pipe.Set(ctx, refreshKeyPrefix+token, userID, ttl)
	pipe.SAdd(ctx, refreshUserKeyPrefix+userID, token)
	if ttl > 0 {
		// Every token gets the same TTL, so the newest one outlives the rest.
		pipe.PExpire(ctx, refreshUserKeyPrefix+userID, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
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
	userID, err := r.rdb.Get(ctx, refreshKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, refreshKeyPrefix+token)
	pipe.SRem(ctx, refreshUserKeyPrefix+userID, token)
	_, err = pipe.Exec(ctx)
	return err
}

// RevokeUserRefresh revokes every refresh token issued to userID.
func (r *TokenRepo) RevokeUserRefresh(ctx context.Context, userID string) error {
	return revokeUserRefreshScript.Run(ctx, r.rdb, []string{refreshUserKeyPrefix + userID}, refreshKeyPrefix).Err()
}

// Rotate atomically revokes oldToken and stores newToken when oldToken still
// exists and belongs to userID. Redis runs the Lua script as one command, so
// clients never observe the old token deleted before the replacement is saved.
func (r *TokenRepo) Rotate(ctx context.Context, oldToken, newToken, userID string, ttl time.Duration) error {
	result, err := rotateRefreshScript.Run(
		ctx,
		r.rdb,
		[]string{refreshKeyPrefix + oldToken, refreshKeyPrefix + newToken, refreshUserKeyPrefix + userID},
		userID,
		ttl.Milliseconds(),
		oldToken,
		newToken,
	).Int()
	if err != nil {
		return err
	}
	if result != 1 {
		return ErrRefreshNotFound
	}
	return nil
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
