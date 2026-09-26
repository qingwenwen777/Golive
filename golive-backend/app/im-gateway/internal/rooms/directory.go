// Package rooms validates the roomId a client asks to join. Every joined room
// costs a Redis subscription, presence and viewer-metric keys, so arbitrary
// ids must not be accepted.
//
// Existence is resolved from the cheapest reliable signal first:
//
//  1. Redis "room:owner:<id>" — room-service writes it (no TTL) whenever a
//     room goes live and re-syncs it for active rooms at boot, and it also
//     yields the trusted owner id.
//  2. room-service GET /rooms/<id> (anonymous) for rooms that are viewable
//     but have never been live, e.g. a scheduled appointment's waiting room.
//     These lookups are rate limited and cached, positive and negative.
package rooms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
	"golang.org/x/time/rate"
)

// ErrUnavailable means existence could not be determined right now.
var ErrUnavailable = errors.New("room lookup unavailable")

// idPattern matches room-service ids ("live-<uuid>-<ts36>", "appt-...") and
// the varchar(64) column they live in.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidID reports whether id is a syntactically valid room id.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Info is what the gateway learns about a room.
type Info struct {
	OwnerID string
}

// Directory resolves room existence.
type Directory interface {
	// Lookup returns found=false for rooms that do not exist.
	Lookup(ctx context.Context, roomID string) (info Info, found bool, err error)
}

// Config tunes a RedisDirectory. Zero values get defaults.
type Config struct {
	// RoomServiceURL enables the HTTP fallback, e.g. "http://room-service:8091".
	RoomServiceURL string
	CacheTTL       time.Duration // positive results, default 5m
	NegativeTTL    time.Duration // unknown rooms, default 30s
	Timeout        time.Duration // HTTP fallback timeout, default 2s
	FallbackRate   float64       // HTTP fallback lookups/sec, default 20
	MaxEntries     int           // cache bound, default 10000
}

type cacheEntry struct {
	info    Info
	found   bool
	expires time.Time
}

// RedisDirectory implements Directory; see the package comment.
type RedisDirectory struct {
	rdb      *redis.Client
	baseURL  string
	client   *http.Client
	fallback *rate.Limiter
	cfg      Config
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

func NewRedisDirectory(rdb *redis.Client, cfg Config) *RedisDirectory {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.NegativeTTL <= 0 {
		cfg.NegativeTTL = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.FallbackRate <= 0 {
		cfg.FallbackRate = 20
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 10000
	}
	return &RedisDirectory{
		rdb:      rdb,
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.RoomServiceURL), "/"),
		client:   &http.Client{Timeout: cfg.Timeout},
		fallback: rate.NewLimiter(rate.Limit(cfg.FallbackRate), int(cfg.FallbackRate*2)+1),
		cfg:      cfg,
		now:      time.Now,
		cache:    make(map[string]cacheEntry),
	}
}

func ownerKey(roomID string) string { return "room:owner:" + roomID }

func (d *RedisDirectory) Lookup(ctx context.Context, roomID string) (Info, bool, error) {
	if !ValidID(roomID) {
		return Info{}, false, nil
	}
	if e, ok := d.cached(roomID); ok {
		return e.info, e.found, nil
	}

	owner, err := d.rdb.Get(ctx, ownerKey(roomID)).Result()
	switch {
	case err == nil && owner != "":
		info := Info{OwnerID: owner}
		d.store(roomID, info, true)
		return info, true, nil
	case err != nil && !errors.Is(err, redis.Nil):
		return Info{}, false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	if d.baseURL == "" {
		d.store(roomID, Info{}, false)
		return Info{}, false, nil
	}
	if !d.fallback.Allow() {
		return Info{}, false, ErrUnavailable
	}
	info, found, err := d.fetch(ctx, roomID)
	if err != nil {
		return Info{}, false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	d.store(roomID, info, found)
	return info, found, nil
}

// fetch asks room-service whether the room is publicly viewable.
func (d *RedisDirectory) fetch(ctx context.Context, roomID string) (Info, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/rooms/"+url.PathEscape(roomID), nil)
	if err != nil {
		return Info{}, false, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return Info{}, false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Info{}, false, nil
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return Info{}, false, fmt.Errorf("room-service returned %d", resp.StatusCode)
	}
	var body struct {
		ID      string `json:"id"`
		OwnerID string `json:"ownerId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
		return Info{}, false, err
	}
	return Info{OwnerID: body.OwnerID}, true, nil
}

func (d *RedisDirectory) cached(roomID string) (cacheEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.cache[roomID]
	if !ok || d.now().After(e.expires) {
		return cacheEntry{}, false
	}
	return e, true
}

func (d *RedisDirectory) store(roomID string, info Info, found bool) {
	ttl := d.cfg.NegativeTTL
	if found {
		ttl = d.cfg.CacheTTL
	}
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.cache) >= d.cfg.MaxEntries {
		for id, e := range d.cache {
			if now.After(e.expires) {
				delete(d.cache, id)
			}
		}
		if len(d.cache) >= d.cfg.MaxEntries {
			// Still full of live entries: start over rather than grow.
			d.cache = make(map[string]cacheEntry)
		}
	}
	d.cache[roomID] = cacheEntry{info: info, found: found, expires: now.Add(ttl)}
}
