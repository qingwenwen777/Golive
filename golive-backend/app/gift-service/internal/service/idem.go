// Package service / idem implements the Redis layer of idempotency.
//
// Layered with the DB-level UNIQUE(request_id), this gives us:
//
//  1. Hot replay: 99% of duplicate requests served from Redis without
//     hitting MySQL. TTL = 10 minutes (matches the frontend axios contract).
//  2. Cold replay: if the Redis entry is evicted but a row still exists in
//     gift_orders, the DB-level unique key catches it and the repo returns
//     the existing order. The caller marks the response as replayed.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-redis/redis/v9"
)

// CachedResp is what we store per requestId.
type CachedResp struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

type IdemCache struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewIdemCache(rdb *redis.Client, ttl time.Duration) *IdemCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &IdemCache{rdb: rdb, ttl: ttl}
}

func key(userID, reqID string) string { return "idem:" + userID + ":" + reqID }

// Lookup returns nil if the requestId is unseen. Returns a non-nil
// CachedResp when there's a final result to replay.
func (c *IdemCache) Lookup(ctx context.Context, userID, reqID string) (*CachedResp, error) {
	v, err := c.rdb.Get(ctx, key(userID, reqID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cr CachedResp
	if err := json.Unmarshal(v, &cr); err != nil {
		return nil, nil // corrupt entry — treat as a miss
	}
	return &cr, nil
}

// Save persists the final response. We use unconditional SET (not SETNX) so
// that the path "DB-level unique violation → look up existing order →
// cache the replay" overwrites any earlier in-flight marker cleanly.
func (c *IdemCache) Save(ctx context.Context, userID, reqID string, status int, body []byte) error {
	payload, err := json.Marshal(CachedResp{Status: status, Body: body})
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key(userID, reqID), payload, c.ttl).Err()
}
