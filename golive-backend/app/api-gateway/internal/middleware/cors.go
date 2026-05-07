package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CORS allows the configured origins (exact match), the methods the frontend
// uses, and exposes Idempotent-Replayed so the SPA can detect replay hits.
func CORS(allowedOrigins []string, maxAge int) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, X-CSRF-Token")
				h.Set("Access-Control-Expose-Headers", "Idempotent-Replayed, X-Request-Id")
				h.Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
