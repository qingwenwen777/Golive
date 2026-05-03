package moderation

import (
	"context"
	"time"

	"github.com/go-redis/redis/v9"
)

type State struct {
	Muted       bool
	MuteTTL     time.Duration
	IsModerator bool
}

type Checker interface {
	State(ctx context.Context, roomID, userID string) (State, error)
}

type RedisChecker struct {
	rdb *redis.Client
}

func NewRedisChecker(rdb *redis.Client) *RedisChecker {
	return &RedisChecker{rdb: rdb}
}

func (c *RedisChecker) State(ctx context.Context, roomID, userID string) (State, error) {
	if c == nil || c.rdb == nil || roomID == "" || userID == "" {
		return State{}, nil
	}
	pipe := c.rdb.Pipeline()
	ttlCmd := pipe.TTL(ctx, roomMuteKey(roomID, userID))
	modCmd := pipe.SIsMember(ctx, roomModeratorsKey(roomID), userID)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return State{}, err
	}
	ttl, _ := ttlCmd.Result()
	isMod, _ := modCmd.Result()
	return State{
		Muted:       ttl > 0,
		MuteTTL:     ttl,
		IsModerator: isMod,
	}, nil
}

func roomModeratorsKey(roomID string) string { return "room:moderators:" + roomID }
func roomMuteKey(roomID, userID string) string {
	return "room:mute:" + roomID + ":" + userID
}
