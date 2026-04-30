// Package repo (social) holds Redis-backed follow + like/dislike state.
//
// Layout:
//
//	follow:
//	  user:<uid>:follows         ZSET  member=channelId  score=time
//	  channel:<cid>:followers    ZSET  member=userId     score=time   (reverse index)
//
//	like / dislike:
//	  like:<sid>:<uid>           HASH  fields liked=0|1, disliked=0|1
//	  like:<sid>:count           STRING  the displayed `likes` counter
//
// like/dislike mutations are done via Lua so the counter and the per-user
// state stay in sync regardless of concurrent writes from the same user.
package repo

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v9"
)

type SocialRepo struct {
	rdb *redis.Client
}

func NewSocialRepo(rdb *redis.Client) *SocialRepo { return &SocialRepo{rdb: rdb} }

// follow ----------------------------------------------------------------

func userFollowsKey(uid string) string  { return "user:" + uid + ":follows" }
func channelFansKey(cid string) string  { return "channel:" + cid + ":followers" }

func (s *SocialRepo) IsFollowing(ctx context.Context, uid, channelID string) (bool, error) {
	score, err := s.rdb.ZScore(ctx, userFollowsKey(uid), channelID).Result()
	_ = score
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SocialRepo) Follow(ctx context.Context, uid, channelID string) error {
	now := float64(time.Now().UnixMilli())
	pipe := s.rdb.TxPipeline()
	pipe.ZAdd(ctx, userFollowsKey(uid), redis.Z{Score: now, Member: channelID})
	pipe.ZAdd(ctx, channelFansKey(channelID), redis.Z{Score: now, Member: uid})
	_, err := pipe.Exec(ctx)
	return err
}

func (s *SocialRepo) Unfollow(ctx context.Context, uid, channelID string) error {
	pipe := s.rdb.TxPipeline()
	pipe.ZRem(ctx, userFollowsKey(uid), channelID)
	pipe.ZRem(ctx, channelFansKey(channelID), uid)
	_, err := pipe.Exec(ctx)
	return err
}

// like / dislike --------------------------------------------------------

func likeStateKey(sid, uid string) string { return "like:" + sid + ":" + uid }
func likeCountKey(sid string) string      { return "like:" + sid + ":count" }

type LikeState struct {
	Liked    bool
	Disliked bool
	Likes    int64
}

// SeedLikeCount sets the counter only if it doesn't already exist. Used at
// bootstrap to give each room a starting `likes` number similar to MSW's
// 1000 + rand*9000 — but deterministic across restarts.
func (s *SocialRepo) SeedLikeCount(ctx context.Context, sid string, count int64) error {
	return s.rdb.SetNX(ctx, likeCountKey(sid), count, 0).Err()
}

// GetLike reads the per-user state and counter without mutating anything.
func (s *SocialRepo) GetLike(ctx context.Context, sid, uid string) (*LikeState, error) {
	pipe := s.rdb.Pipeline()
	hget := pipe.HMGet(ctx, likeStateKey(sid, uid), "liked", "disliked")
	gcnt := pipe.Get(ctx, likeCountKey(sid))
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	vals, _ := hget.Result()
	cntStr, _ := gcnt.Result()
	cnt, _ := strconv.ParseInt(cntStr, 10, 64)
	return &LikeState{
		Liked:    isTrue(vals[0]),
		Disliked: isTrue(vals[1]),
		Likes:    cnt,
	}, nil
}

// Lua scripts. KEYS[1]=stateHash, KEYS[2]=countKey. Each returns {liked, disliked, count}.

var luaLike = redis.NewScript(`
local liked = redis.call('HGET', KEYS[1], 'liked')
if liked ~= '1' then redis.call('INCR', KEYS[2]) end
redis.call('HSET', KEYS[1], 'liked', '1', 'disliked', '0')
local cnt = tonumber(redis.call('GET', KEYS[2]) or '0')
return {1, 0, cnt}
`)

var luaUnlike = redis.NewScript(`
local liked = redis.call('HGET', KEYS[1], 'liked')
local disliked = redis.call('HGET', KEYS[1], 'disliked') or '0'
if liked == '1' then redis.call('DECR', KEYS[2]) end
redis.call('HSET', KEYS[1], 'liked', '0')
local cnt = tonumber(redis.call('GET', KEYS[2]) or '0')
if cnt < 0 then redis.call('SET', KEYS[2], '0'); cnt = 0 end
local d = 0
if disliked == '1' then d = 1 end
return {0, d, cnt}
`)

var luaDislike = redis.NewScript(`
local liked = redis.call('HGET', KEYS[1], 'liked')
if liked == '1' then redis.call('DECR', KEYS[2]) end
redis.call('HSET', KEYS[1], 'liked', '0', 'disliked', '1')
local cnt = tonumber(redis.call('GET', KEYS[2]) or '0')
if cnt < 0 then redis.call('SET', KEYS[2], '0'); cnt = 0 end
return {0, 1, cnt}
`)

var luaUndislike = redis.NewScript(`
local liked = redis.call('HGET', KEYS[1], 'liked') or '0'
redis.call('HSET', KEYS[1], 'disliked', '0')
local cnt = tonumber(redis.call('GET', KEYS[2]) or '0')
local l = 0
if liked == '1' then l = 1 end
return {l, 0, cnt}
`)

func (s *SocialRepo) Like(ctx context.Context, sid, uid string) (*LikeState, error) {
	return s.runLuaState(ctx, luaLike, sid, uid)
}
func (s *SocialRepo) Unlike(ctx context.Context, sid, uid string) (*LikeState, error) {
	return s.runLuaState(ctx, luaUnlike, sid, uid)
}
func (s *SocialRepo) Dislike(ctx context.Context, sid, uid string) (*LikeState, error) {
	return s.runLuaState(ctx, luaDislike, sid, uid)
}
func (s *SocialRepo) Undislike(ctx context.Context, sid, uid string) (*LikeState, error) {
	return s.runLuaState(ctx, luaUndislike, sid, uid)
}

func (s *SocialRepo) runLuaState(ctx context.Context, script *redis.Script, sid, uid string) (*LikeState, error) {
	res, err := script.Run(ctx, s.rdb,
		[]string{likeStateKey(sid, uid), likeCountKey(sid)},
	).Result()
	if err != nil {
		return nil, err
	}
	arr, _ := res.([]interface{})
	if len(arr) < 3 {
		return &LikeState{}, nil
	}
	return &LikeState{
		Liked:    toInt64(arr[0]) == 1,
		Disliked: toInt64(arr[1]) == 1,
		Likes:    toInt64(arr[2]),
	}, nil
}

func isTrue(v interface{}) bool {
	switch x := v.(type) {
	case string:
		return x == "1"
	case nil:
		return false
	}
	return false
}

func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
