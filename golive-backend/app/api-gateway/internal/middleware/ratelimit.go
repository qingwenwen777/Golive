package middleware

import (
	"net"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimit enforces a per-IP token bucket. Suitable for a single-instance
// MVP; swap for Redis-backed when we shard.
//
// 429 responses match the gateway's unified shape via plain JSON write to
// avoid coupling this middleware to the errcode pkg internals.
func RateLimit(ratePerSec float64, burst int) gin.HandlerFunc {
	if ratePerSec <= 0 || burst <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	var (
		mu       sync.Mutex
		buckets  = make(map[string]*rate.Limiter)
	)
	get := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := buckets[ip]
		if !ok {
			l = rate.NewLimiter(rate.Limit(ratePerSec), burst)
			buckets[ip] = l
		}
		return l
	}
	return func(c *gin.Context) {
		ip := clientIP(c)
		if !get(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "Too Many Requests",
				"reason":  "rate_limited",
			})
			return
		}
		c.Next()
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
