// Package profile resolves what other viewers see about a chatter — display
// name, avatar, user level and fan badge — from server-side sources, keyed by
// the authenticated user id. Nothing here is ever taken from the client.
//
// Sources:
//
//   - user-service GET /users/profile/<id> (the public profile endpoint the
//     frontend itself uses): display name, avatar, level. Its gRPC API only
//     exposes permissions, and the JWT carries nothing but the user id.
//   - chat-service GET /internal/rooms/<room>/fan-badges/<user>: the user's
//     fan badge for the room owner, from gift-service's fan_badges table,
//     which chat-service can read. Sent with the shared internal token.
//
// Results are cached per gateway for a short TTL, so a busy room costs one
// lookup per chatter per TTL. When a source is down the resolver serves the
// last known value, else a neutral fallback (id-derived name, no avatar,
// level or badge) — never whatever the client claimed.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/pkg/internalauth"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// Profile is the public identity shown next to a user's chat and in the
// viewer list.
type Profile struct {
	UserID string
	Name   string
	Avatar string
	Level  int
}

// FanBadge is a user's badge in the room owner's fan club.
type FanBadge struct {
	CreatorID string `json:"creatorId"`
	Level     int    `json:"level"`
}

// Resolver never fails: on error it degrades to cached or neutral values.
type Resolver interface {
	Profile(ctx context.Context, userID string) Profile
	FanBadge(ctx context.Context, roomID, userID string) *FanBadge
}

// Config configures an HTTPResolver. Zero durations get defaults.
type Config struct {
	UserServiceURL string        // e.g. http://user-service:8090
	ChatServiceURL string        // e.g. http://chat-service:8093
	InternalToken  string        // X-Internal-Token for chat-service /internal
	TTL            time.Duration // fresh lifetime of a cached value, default 30s
	ErrorTTL       time.Duration // back-off after a failed lookup, default 5s
	Timeout        time.Duration // per request, default 1s
	MaxEntries     int           // per cache, default 50000
}

// HTTPResolver implements Resolver over the services' HTTP APIs.
type HTTPResolver struct {
	userURL string
	chatURL string
	token   string
	client  *http.Client
	cfg     Config
	now     func() time.Time

	profiles *ttlCache[Profile]
	badges   *ttlCache[*FanBadge]
}

func NewHTTPResolver(cfg Config) *HTTPResolver {
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * time.Second
	}
	if cfg.ErrorTTL <= 0 {
		cfg.ErrorTTL = 5 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = time.Second
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 50000
	}
	return &HTTPResolver{
		userURL:  strings.TrimRight(strings.TrimSpace(cfg.UserServiceURL), "/"),
		chatURL:  strings.TrimRight(strings.TrimSpace(cfg.ChatServiceURL), "/"),
		token:    cfg.InternalToken,
		client:   &http.Client{Timeout: cfg.Timeout},
		cfg:      cfg,
		now:      time.Now,
		profiles: newTTLCache[Profile](cfg.MaxEntries),
		badges:   newTTLCache[*FanBadge](cfg.MaxEntries),
	}
}

func (r *HTTPResolver) Profile(ctx context.Context, userID string) Profile {
	fallback := Profile{UserID: userID, Name: FallbackName(userID)}
	if userID == "" || r.userURL == "" {
		return fallback
	}
	now := r.now()
	cached, fresh, ok := r.profiles.get(userID, now)
	if fresh {
		return cached
	}
	p, err := r.fetchProfile(ctx, userID)
	if err != nil {
		logger.L().Warn("resolve chat profile", zap.String("user", userID), zap.Error(err))
		if !ok {
			cached = fallback
		}
		// Serve the stale/fallback value and back off before retrying.
		r.profiles.put(userID, cached, now.Add(r.cfg.ErrorTTL))
		return cached
	}
	r.profiles.put(userID, p, now.Add(r.cfg.TTL))
	return p
}

func (r *HTTPResolver) FanBadge(ctx context.Context, roomID, userID string) *FanBadge {
	if roomID == "" || userID == "" || r.chatURL == "" {
		return nil
	}
	key := roomID + "\x00" + userID
	now := r.now()
	cached, fresh, ok := r.badges.get(key, now)
	if fresh {
		return cached
	}
	b, err := r.fetchBadge(ctx, roomID, userID)
	if err != nil {
		logger.L().Warn("resolve fan badge", zap.String("room", roomID), zap.String("user", userID), zap.Error(err))
		if !ok {
			cached = nil
		}
		r.badges.put(key, cached, now.Add(r.cfg.ErrorTTL))
		return cached
	}
	r.badges.put(key, b, now.Add(r.cfg.TTL))
	return b
}

func (r *HTTPResolver) fetchProfile(ctx context.Context, userID string) (Profile, error) {
	var body struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Avatar      string `json:"avatar"`
		LevelInfo   struct {
			Level int `json:"level"`
		} `json:"levelInfo"`
	}
	found, err := r.getJSON(ctx, r.userURL+"/users/profile/"+url.PathEscape(userID), "", &body)
	if err != nil {
		return Profile{}, err
	}
	if !found || body.ID != userID {
		// The endpoint also resolves usernames; only an exact id match counts.
		return Profile{UserID: userID, Name: FallbackName(userID)}, nil
	}
	return Profile{
		UserID: userID,
		Name:   DisplayName(userID, body.DisplayName, body.Username),
		Avatar: cleanAvatar(body.Avatar),
		Level:  clampLevel(body.LevelInfo.Level),
	}, nil
}

func (r *HTTPResolver) fetchBadge(ctx context.Context, roomID, userID string) (*FanBadge, error) {
	var body struct {
		FanBadge *FanBadge `json:"fanBadge"`
	}
	found, err := r.getJSON(ctx, r.chatURL+"/internal/rooms/"+url.PathEscape(roomID)+"/fan-badges/"+url.PathEscape(userID), r.token, &body)
	if err != nil || !found || body.FanBadge == nil {
		return nil, err
	}
	b := body.FanBadge
	if b.CreatorID == "" || len(b.CreatorID) > 80 || b.Level < 1 {
		return nil, nil
	}
	return &FanBadge{CreatorID: b.CreatorID, Level: clampLevel(b.Level)}, nil
}

// getJSON decodes a 2xx body into out. found is false on 404. A non-empty
// internalToken is sent as X-Internal-Token (only for /internal calls).
func (r *HTTPResolver) getJSON(ctx context.Context, endpoint, internalToken string, out any) (found bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	internalauth.SetToken(req, internalToken)
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("%s returned %d", endpoint, resp.StatusCode)
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(out); err != nil {
		return false, err
	}
	return true, nil
}

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DisplayName mirrors the frontend's userDisplayName: display name, else a
// username that isn't a bare uuid, else an id-derived fallback.
func DisplayName(userID, displayName, username string) string {
	if name := cleanName(displayName); name != "" {
		return name
	}
	if name := cleanName(username); name != "" && !uuidLike.MatchString(name) {
		return name
	}
	return FallbackName(userID)
}

// FallbackName is shown when no profile is available.
func FallbackName(userID string) string {
	if userID == "" {
		return "Guest"
	}
	if len(userID) > 8 {
		userID = userID[:8]
	}
	return "Creator " + userID
}

func cleanName(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if utf8.RuneCountInString(s) > 64 {
		s = string([]rune(s)[:64])
	}
	return s
}

func cleanAvatar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 500 || strings.ContainsAny(s, "\r\n\t") {
		return ""
	}
	return s
}

func clampLevel(level int) int {
	if level < 1 {
		return 0
	}
	if level > 99 {
		return 99
	}
	return level
}

// ttlCache is a small mutex-guarded map with per-entry expiry. Stale entries
// are kept (until evicted) so they can be served while a source is down.
type ttlCache[V any] struct {
	max int

	mu sync.Mutex
	m  map[string]ttlEntry[V]
}

type ttlEntry[V any] struct {
	v       V
	expires time.Time
}

func newTTLCache[V any](max int) *ttlCache[V] {
	return &ttlCache[V]{max: max, m: make(map[string]ttlEntry[V])}
}

// get returns the cached value, whether it is still fresh, and whether any
// value (fresh or stale) was cached.
func (c *ttlCache[V]) get(key string, now time.Time) (v V, fresh, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return v, false, false
	}
	return e.v, now.Before(e.expires), true
}

func (c *ttlCache[V]) put(key string, v V, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.m[key]; !exists && len(c.m) >= c.max {
		now := time.Now()
		for k, e := range c.m {
			if now.After(e.expires) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= c.max {
			c.m = make(map[string]ttlEntry[V])
		}
	}
	c.m[key] = ttlEntry[V]{v: v, expires: expires}
}
