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

func userFollowsKey(uid string) string { return "user:" + uid + ":follows" }
func channelFansKey(cid string) string { return "channel:" + cid + ":followers" }

func (s *SocialRepo) IsFollowing(ctx context.Context, uid, channelID string) (bool, error) {
	if uid == "" {
		return false, nil
	}
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

func (s *SocialRepo) FollowerCount(ctx context.Context, channelID string) (int64, error) {
	if channelID == "" {
		return 0, nil
	}
	return s.rdb.ZCard(ctx, channelFansKey(channelID)).Result()
}

func (s *SocialRepo) FollowerCounts(ctx context.Context, channelIDs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(channelIDs))
	if len(channelIDs) == 0 {
		return out, nil
	}
	pipe := s.rdb.Pipeline()
	cmds := make(map[string]*redis.IntCmd, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID == "" {
			continue
		}
		if _, ok := cmds[channelID]; ok {
			continue
		}
		cmds[channelID] = pipe.ZCard(ctx, channelFansKey(channelID))
	}
	if len(cmds) == 0 {
		return out, nil
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	for channelID, cmd := range cmds {
		count, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		out[channelID] = count
	}
	return out, nil
}

func (s *SocialRepo) FollowerCountBetween(ctx context.Context, channelID string, start, end time.Time) (int64, error) {
	if channelID == "" {
		return 0, nil
	}
	return s.rdb.ZCount(
		ctx,
		channelFansKey(channelID),
		strconv.FormatInt(start.UnixMilli(), 10),
		strconv.FormatInt(end.UnixMilli(), 10),
	).Result()
}

func (s *SocialRepo) Following(ctx context.Context, uid string) ([]string, error) {
	if uid == "" {
		return nil, nil
	}
	return s.rdb.ZRevRange(ctx, userFollowsKey(uid), 0, -1).Result()
}

func (s *SocialRepo) Followers(ctx context.Context, channelID string) ([]string, error) {
	if channelID == "" {
		return nil, nil
	}
	return s.rdb.ZRevRange(ctx, channelFansKey(channelID), 0, -1).Result()
}

func (s *SocialRepo) Follow(ctx context.Context, uid, channelID string) error {
	now := float64(time.Now().UnixMilli())
	pipe := s.rdb.TxPipeline()
	pipe.ZAdd(ctx, userFollowsKey(uid), redis.Z{Score: now, Member: channelID})
	pipe.ZAdd(ctx, channelFansKey(channelID), redis.Z{Score: now, Member: uid})
	_, err := pipe.Exec(ctx)
	return err
}

// KEYS[1]=user follows, KEYS[2]=channel followers.
// ARGV[1]=score, ARGV[2]=uid, ARGV[3]=channelID, ARGV[4]=limit (<=0: none).
// Returns 0 when a new follow would exceed the limit, else 1.
var luaFollowWithLimit = redis.NewScript(`
local limit = tonumber(ARGV[4])
if limit > 0 and not redis.call('ZSCORE', KEYS[1], ARGV[3]) and redis.call('ZCARD', KEYS[1]) >= limit then
  return 0
end
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[3])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[2])
return 1
`)

// FollowWithLimit follows like Follow but refuses a new follow once uid
// already follows limit channels. Re-following an existing channel is always
// allowed. It reports whether the follow was stored.
func (s *SocialRepo) FollowWithLimit(ctx context.Context, uid, channelID string, limit int) (bool, error) {
	now := time.Now().UnixMilli()
	res, err := luaFollowWithLimit.Run(ctx, s.rdb,
		[]string{userFollowsKey(uid), channelFansKey(channelID)},
		now, uid, channelID, limit,
	).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// KEYS[1]=user follows, KEYS[2]=old channel followers, KEYS[3]=new channel
// followers. ARGV[1]=uid, ARGV[2]=old key, ARGV[3]=new key ("" drops it).
var luaReplaceFollowKey = redis.NewScript(`
local score = redis.call('ZSCORE', KEYS[1], ARGV[2])
if not score then
  return 0
end
redis.call('ZREM', KEYS[1], ARGV[2])
redis.call('ZREM', KEYS[2], ARGV[1])
if ARGV[3] ~= '' then
  redis.call('ZADD', KEYS[1], 'NX', score, ARGV[3])
  redis.call('ZADD', KEYS[3], 'NX', score, ARGV[1])
end
return 1
`)

// ReplaceFollowKey moves uid's follow from oldKey to newKey, keeping the
// original follow time (or an existing follow of newKey). An empty newKey
// drops the follow.
func (s *SocialRepo) ReplaceFollowKey(ctx context.Context, uid, oldKey, newKey string) error {
	newFans := channelFansKey(oldKey)
	if newKey != "" {
		newFans = channelFansKey(newKey)
	}
	return luaReplaceFollowKey.Run(ctx, s.rdb,
		[]string{userFollowsKey(uid), channelFansKey(oldKey), newFans},
		uid, oldKey, newKey,
	).Err()
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

// SeedLikeCount sets the counter only if it doesn't already exist. Tests use
// it to set up like counts; rooms themselves start from zero.
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

func (s *SocialRepo) LikeCounts(ctx context.Context, sids []string) (map[string]int64, error) {
	out := make(map[string]int64, len(sids))
	if s == nil || s.rdb == nil || len(sids) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(sids))
	for _, sid := range sids {
		keys = append(keys, likeCountKey(sid))
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	for i, value := range values {
		out[sids[i]] = toInt64(value)
	}
	return out, nil
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
