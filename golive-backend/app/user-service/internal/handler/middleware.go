package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const ctxUserIDKey = "userID"

// AuthRequired validates the Bearer token and stores userId in gin.Context.
func AuthRequired(svc *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		uid, err := svc.ParseAccess(strings.TrimPrefix(raw, prefix))
		if err != nil || uid == "" {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		c.Set(ctxUserIDKey, uid)
		c.Next()
	}
}

// UserIDFromCtx returns the authenticated user id, or "" if absent.
func UserIDFromCtx(c *gin.Context) string {
	v, ok := c.Get(ctxUserIDKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
