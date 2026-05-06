package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	rateLimitBucketTTL        = 10 * time.Minute
	rateLimitCleanupInterval  = time.Minute
	rateLimitMaxBucketEntries = 100_000
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

// RateLimit enforces a per-IP token bucket. Suitable for a single-instance
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
	// gin.ClientIP honors X-Forwarded-For when TrustedProxies is set; we
	// haven't configured it, so this falls back to RemoteAddr — exactly
	// what we want for an edge gateway behind no other LB.
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
