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
// lookup per chatter per TTL, and concurrent lookups for one user share a
// single fetch. When a source is down the resolver serves the last known
// value, else a neutral fallback (id-derived name, no avatar, level or
// badge) — never whatever the client claimed — and says so, so callers
// don't hold on to the fallback.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// Profile returns the user's profile, cached for Config.TTL. ok is false
	// when user-service couldn't be reached and nothing was cached: p is
	// then only the neutral fallback, which callers must not hold on to.
	Profile(ctx context.Context, userID string) (p Profile, ok bool)
	// Refresh is Profile for when the user may just have changed their
	// profile: it refetches unless the cached value is younger than
	// Config.MinRefresh, so a client can't turn it into a flood.
	Refresh(ctx context.Context, userID string) (p Profile, ok bool)
	FanBadge(ctx context.Context, roomID, userID string) *FanBadge
}

// Config configures an HTTPResolver. Zero durations get defaults.
type Config struct {
	UserServiceURL string        // e.g. http://user-service:8090
	ChatServiceURL string        // e.g. http://chat-service:8093
	InternalToken  string        // X-Internal-Token for chat-service /internal
	TTL            time.Duration // fresh lifetime of a cached value, default 30s
	MinRefresh     time.Duration // youngest profile Refresh refetches, default 5s
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

	profiles *ttlCache[profileEntry]
	badges   *ttlCache[*FanBadge]
	// fetching de-duplicates concurrent profile fetches for one user.
	fetching flight[profileEntry]
}

// profileEntry is a cached profile lookup.
type profileEntry struct {
	p       Profile
	ok      bool      // p is user-service's answer, not the fallback
	failed  bool      // the last fetch failed: back off for ErrorTTL
	fetched time.Time // when the last fetch finished
}

func NewHTTPResolver(cfg Config) *HTTPResolver {
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * time.Second
	}
	if cfg.MinRefresh <= 0 {
		cfg.MinRefresh = 5 * time.Second
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
		client:   newClient(cfg.Timeout),
		cfg:      cfg,
		now:      time.Now,
		profiles: newTTLCache[profileEntry](cfg.MaxEntries),
		badges:   newTTLCache[*FanBadge](cfg.MaxEntries),
	}
}

// newClient returns the client for the lookups. The default transport keeps
// 2 idle connections per host, so a burst of lookups (every viewer
// reconnecting after a gateway deploy) opened a TCP connection per lookup
// and closed most of them again. The pool now stays warm, and
// MaxConnsPerHost bounds how many requests a burst puts on a service at once.
func newClient(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 512
	t.MaxIdleConnsPerHost = 256
	t.MaxConnsPerHost = 256
	t.IdleConnTimeout = 90 * time.Second
	return &http.Client{Timeout: timeout, Transport: t}
}

func (r *HTTPResolver) Profile(ctx context.Context, userID string) (Profile, bool) {
	return r.profile(ctx, userID, r.cfg.TTL)
}

func (r *HTTPResolver) Refresh(ctx context.Context, userID string) (Profile, bool) {
	return r.profile(ctx, userID, r.cfg.MinRefresh)
}

// profile serves the cached profile while it is younger than maxAge (or its
// failed fetch is still backing off) and otherwise fetches it, once for all
// concurrent callers.
func (r *HTTPResolver) profile(ctx context.Context, userID string, maxAge time.Duration) (Profile, bool) {
	if userID == "" || r.userURL == "" {
		return Profile{UserID: userID, Name: FallbackName(userID)}, true
	}
	if e, ok := r.cachedProfile(userID, maxAge); ok {
		return e.p, e.ok
	}
	e := r.fetching.do(userID, func() profileEntry {
		// A fetch that just finished may have made this one unnecessary.
		if e, ok := r.cachedProfile(userID, maxAge); ok {
			return e
		}
		// Callers share the result, so one caller's cancellation mustn't
		// fail it for the rest; getJSON still applies the timeout.
		return r.fetchProfileEntry(context.WithoutCancel(ctx), userID)
	})
	return e.p, e.ok
}

// cachedProfile returns the cached lookup unless it is due for a refetch.
func (r *HTTPResolver) cachedProfile(userID string, maxAge time.Duration) (profileEntry, bool) {
	now := r.now()
	e, _, ok := r.profiles.get(userID, now)
	if !ok {
		return e, false
	}
	if e.failed {
		maxAge = r.cfg.ErrorTTL
	}
	return e, now.Sub(e.fetched) < maxAge
}

// fetchProfileEntry fetches and caches the profile. On error it keeps
// serving the last known value, else the fallback, and backs off before the
// next attempt.
func (r *HTTPResolver) fetchProfileEntry(ctx context.Context, userID string) profileEntry {
	p, err := r.fetchProfile(ctx, userID)
	now := r.now()
	if err == nil {
		e := profileEntry{p: p, ok: true, fetched: now}
		r.profiles.put(userID, e, now.Add(r.cfg.TTL))
		return e
	}
	logger.L().Warn("resolve chat profile", zap.String("user", userID), zap.Error(err))
	e, _, cached := r.profiles.get(userID, now)
	if !cached {
		e = profileEntry{p: Profile{UserID: userID, Name: FallbackName(userID)}}
	}
	e.failed, e.fetched = true, now
	r.profiles.put(userID, e, now.Add(r.cfg.ErrorTTL))
	return e
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
	defer func() {
		// Read what's left so the connection goes back to the pool, which
		// matters most while a restarting service answers with errors.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()
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

// flight runs one fetch per key at a time: callers asking for a key that is
// already being fetched wait for that fetch and share its result, so a burst
// of lookups for one user (a reconnect storm, several tabs) costs one request.
type flight[V any] struct {
	mu    sync.Mutex
	calls map[string]*flightCall[V]
}

type flightCall[V any] struct {
	done chan struct{}
	v    V
}

func (g *flight[V]) do(key string, fetch func() V) V {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.v
	}
	if g.calls == nil {
		g.calls = make(map[string]*flightCall[V])
	}
	c := &flightCall[V]{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		close(c.done)
	}()
	c.v = fetch()
	return c.v
}
