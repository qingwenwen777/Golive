// Package chatlimit implements a Redis-backed fixed-window per-user limiter.
// Being in Redis, the limit holds across every connection and gateway
// instance a user has, unlike a per-connection token bucket.
//
// Algorithm: INCR on a key keyed by (userId, currentSecond). On the first
// increment in a window, EXPIRE marks the bucket TTL. The current count is
// compared against the configured limit. Both ops fit in a single round-trip
// via a tiny Lua script for atomicity.
//
// Trade-offs:
//
//   - Pro: O(1) per request, no GC sweep.
//   - Con: fixed-window has a 2x burst at boundaries vs. sliding window.
//     Acceptable for chat (we just want to keep one user from flooding).
package chatlimit

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v9"
)

type Limiter struct {
	rdb    *redis.Client
	prefix string
	limit  int
	window time.Duration
}

// New returns a limiter allowing limit calls per window per user. Keys are
// prefix+userID+":"+bucket; callers enforcing independent limits must use
// different prefixes so they don't consume each other's budget.
func New(rdb *redis.Client, prefix string, limit int, window time.Duration) *Limiter {
	if limit <= 0 {
		limit = 1
	}
	// Sub-millisecond windows cannot work: every call lands in its own bucket
	// and PEXPIRE rounds the TTL to 0. Treat them as unset.
	if window < time.Millisecond {
		window = time.Second
	}
	return &Limiter{rdb: rdb, prefix: prefix, limit: limit, window: window}
}

// luaCheckIncr returns 1 if allowed, 0 if denied.
var luaCheckIncr = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
if n > tonumber(ARGV[1]) then
  return 0
end
return 1
`)

// Allow returns true if the user is under the limit for the current window.
func (l *Limiter) Allow(ctx context.Context, userID string) (bool, error) {
	bucket := strconv.FormatInt(time.Now().UnixNano()/int64(l.window), 10)
	key := l.prefix + userID + ":" + bucket
	res, err := luaCheckIncr.Run(ctx, l.rdb,
		[]string{key},
		l.limit, l.window.Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
