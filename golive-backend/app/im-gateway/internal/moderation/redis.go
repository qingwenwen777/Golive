package moderation

import (
	"context"
	"time"

	"github.com/go-redis/redis/v9"

	"github.com/qingwenwen777/golive/pkg/contentpolicy"
)

type State struct {
	Muted       bool
	MuteTTL     time.Duration
	SiteBanned  bool
	SiteMuted   bool
	SiteMuteTTL time.Duration
	IsModerator bool
}

type Checker interface {
	State(ctx context.Context, roomID, userID string) (State, error)
	ContainsBlockedWord(ctx context.Context, text string) (bool, error)
}

type RedisChecker struct {
	rdb *redis.Client
}

func (c *RedisChecker) ContainsBlockedWord(ctx context.Context, text string) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	words, err := c.rdb.SMembers(ctx, contentpolicy.RedisBlockedWordsKey).Result()
	if err != nil && err != redis.Nil {
		return false, err
	}
	return contentpolicy.Contains(text, words), nil
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
	banCmd := pipe.Exists(ctx, contentpolicy.RedisSiteBanPrefix+userID)
	siteMuteCmd := pipe.TTL(ctx, contentpolicy.RedisSiteMutePrefix+userID)
	modCmd := pipe.SIsMember(ctx, roomModeratorsKey(roomID), userID)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return State{}, err
	}
	ttl, _ := ttlCmd.Result()
	banned, _ := banCmd.Result()
	siteMuteTTL, _ := siteMuteCmd.Result()
	isMod, _ := modCmd.Result()
	return State{
		Muted:       ttl > 0,
		MuteTTL:     ttl,
		SiteBanned:  banned > 0,
		SiteMuted:   siteMuteTTL > 0,
		SiteMuteTTL: siteMuteTTL,
		IsModerator: isMod,
	}, nil
}

func roomModeratorsKey(roomID string) string { return "room:moderators:" + roomID }
func roomMuteKey(roomID, userID string) string {
	return "room:mute:" + roomID + ":" + userID
}
