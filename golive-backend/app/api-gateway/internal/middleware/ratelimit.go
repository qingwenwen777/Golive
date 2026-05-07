package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	rateLimitBucketTTL        = 10 * time.Minute
	rateLimitCleanupInterval  = time.Minute
	rateLimitMaxBucketEntries = 100_000
	defaultAuthRatePerSec     = 0.3
	defaultAuthBurst          = 6
)

type rateLimitBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimitStore struct {
	mu         sync.Mutex
	buckets    map[string]*rateLimitBucket
	limit      rate.Limit
	burst      int
	ttl        time.Duration
	maxEntries int
}

// RateLimit enforces a per-client token bucket. Suitable for a single-instance
// MVP; swap for Redis-backed when we shard.
//
// 429 responses match the gateway's unified shape via plain JSON write to
// avoid coupling this middleware to the errcode pkg internals.
func RateLimit(ratePerSec float64, burst int) gin.HandlerFunc {
	if ratePerSec <= 0 || burst <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	store := newRateLimitStore(ratePerSec, burst, rateLimitBucketTTL, rateLimitMaxBucketEntries)
	store.startJanitor(rateLimitCleanupInterval)

	return func(c *gin.Context) {
		ip := clientIP(c)
		if !store.get(ip, time.Now()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "Too Many Requests",
				"reason":  "rate_limited",
			})
			return
		}
		c.Next()
	}
}

// AuthRateLimit adds a stricter per-client bucket for credential and token
// issuance routes. It is layered with the broader global limiter.
func AuthRateLimit(ratePerSec float64, burst int) gin.HandlerFunc {
	if ratePerSec <= 0 {
		ratePerSec = defaultAuthRatePerSec
	}
	if burst <= 0 {
		burst = defaultAuthBurst
	}
	store := newRateLimitStore(ratePerSec, burst, rateLimitBucketTTL, rateLimitMaxBucketEntries)
	store.startJanitor(rateLimitCleanupInterval)

	return func(c *gin.Context) {
		if !isAuthLimitedRoute(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}
		ip := clientIP(c)
		if !store.get(ip, time.Now()).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "Too Many Requests",
				"reason":  "auth_rate_limited",
			})
			return
		}
		c.Next()
	}
}

func newRateLimitStore(ratePerSec float64, burst int, ttl time.Duration, maxEntries int) *rateLimitStore {
	return &rateLimitStore{
		buckets:    make(map[string]*rateLimitBucket),
		limit:      rate.Limit(ratePerSec),
		burst:      burst,
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (s *rateLimitStore) get(ip string, now time.Time) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	if b, ok := s.buckets[ip]; ok {
		b.lastSeen = now
		return b.limiter
	}

	if s.maxEntries > 0 && len(s.buckets) >= s.maxEntries {
		s.cleanupLocked(now)
		if len(s.buckets) >= s.maxEntries {
			s.evictOldestLocked()
		}
	}

	l := rate.NewLimiter(s.limit, s.burst)
	s.buckets[ip] = &rateLimitBucket{limiter: l, lastSeen: now}
	return l
}

func (s *rateLimitStore) startJanitor(interval time.Duration) {
	if interval <= 0 || s.ttl <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for now := range ticker.C {
			s.cleanup(now)
		}
	}()
}

func (s *rateLimitStore) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
}

func (s *rateLimitStore) cleanupLocked(now time.Time) {
	if s.ttl <= 0 {
		return
	}
	for ip, b := range s.buckets {
		if now.Sub(b.lastSeen) > s.ttl {
			delete(s.buckets, ip)
		}
	}
}

func (s *rateLimitStore) evictOldestLocked() {
	var (
		oldestIP   string
		oldestSeen time.Time
		found      bool
	)
	for ip, b := range s.buckets {
		if !found || b.lastSeen.Before(oldestSeen) {
			oldestIP = ip
			oldestSeen = b.lastSeen
			found = true
		}
	}
	if found {
		delete(s.buckets, oldestIP)
	}
}

func clientIP(c *gin.Context) string {
	if ip := headerClientIP(c.GetHeader("X-Real-IP")); ip != "" {
		return ip
	}
	if ip := headerClientIP(c.GetHeader("X-Forwarded-For")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

func headerClientIP(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(part); err == nil {
			part = host
		}
		ip := net.ParseIP(part)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		return ip.String()
	}
	return ""
}

func isAuthLimitedRoute(method, path string) bool {
	path = strings.TrimRight(path, "/")
	if method == http.MethodGet && path == "/api/auth/captcha" {
		return true
	}
	if method != http.MethodPost {
		return false
	}
	switch path {
	case "/api/auth/email-code",
		"/api/auth/google/bind",
		"/api/auth/google/link-existing",
		"/api/auth/google/login",
		"/api/auth/google/register",
		"/api/auth/google/unbind",
		"/api/auth/login",
		"/api/auth/logout",
		"/api/auth/password/reset",
		"/api/auth/refresh",
		"/api/auth/register":
		return true
	default:
		return false
	}
}
