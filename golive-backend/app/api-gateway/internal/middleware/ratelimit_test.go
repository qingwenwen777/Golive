package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func TestRateLimitStoreCleanupRemovesIdleBuckets(t *testing.T) {
	store := newRateLimitStore(1, 1, time.Minute, 10)
	now := time.Unix(100, 0)

	_ = store.get("192.0.2.1", now)
	_ = store.get("192.0.2.2", now.Add(30*time.Second))

	store.cleanup(now.Add(80 * time.Second))

	store.mu.Lock()
	defer store.mu.Unlock()
	require.NotContains(t, store.buckets, "192.0.2.1")
	require.Contains(t, store.buckets, "192.0.2.2")
}

func TestRateLimitStoreEvictsOldestWhenFull(t *testing.T) {
	store := newRateLimitStore(1, 1, time.Hour, 2)
	now := time.Unix(100, 0)

	_ = store.get("192.0.2.1", now)
	_ = store.get("192.0.2.2", now.Add(time.Second))
	_ = store.get("192.0.2.3", now.Add(2*time.Second))

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.buckets, 2)
	require.NotContains(t, store.buckets, "192.0.2.1")
	require.Contains(t, store.buckets, "192.0.2.2")
	require.Contains(t, store.buckets, "192.0.2.3")
}

func TestClientIPPrefersForwardedHeaders(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "172.18.0.1:12345"
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.10, 172.18.0.1")
	c.Request.Header.Set("X-Real-IP", "198.51.100.5")

	require.Equal(t, "203.0.113.10", clientIP(c))
}

func TestAuthRateLimitOnlyLimitsAuthRoutes(t *testing.T) {
	r := gin.New()
	r.Use(AuthRateLimit(1, 1))
	r.GET("/api/rooms", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/auth/login", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/rooms", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNoContent, w.Code)
	}

	ok := httptest.NewRecorder()
	first := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	r.ServeHTTP(ok, first)
	require.Equal(t, http.StatusNoContent, ok.Code)

	limited := httptest.NewRecorder()
	second := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	second.RemoteAddr = "192.0.2.10:1234"
	r.ServeHTTP(limited, second)
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Contains(t, limited.Body.String(), "auth_rate_limited")
}
