// Package middleware contains the gateway's cross-cutting concerns. Each
// middleware is independently testable.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderRequestID = "X-Request-Id"

// RequestID ensures every request carries an X-Request-Id, generating one
// if the client didn't supply one. The id is echoed back on the response
// AND forwarded to upstreams (handled by the proxy layer reading
// c.Request.Header). Idempotency for /gifts/send + /super-chats depends on
// this header surviving the gateway hop.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
			c.Request.Header.Set(HeaderRequestID, id)
		}
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}
