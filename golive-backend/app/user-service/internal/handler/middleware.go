package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
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

func AdminRequired(users *repo.UserRepo) gin.HandlerFunc {
	forbidden := errcode.New(http.StatusForbidden, "Admin permission required")
	return func(c *gin.Context) {
		uid := UserIDFromCtx(c)
		if uid == "" {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		u, err := users.FindByID(c.Request.Context(), uid)
		if err != nil {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		if u.Role != model.RoleAdmin {
			errcode.Respond(c, forbidden)
			return
		}
		if u.Banned {
			errcode.Respond(c, service.ErrUserBanned)
			return
		}
		c.Next()
	}
}

// NotBanned rejects banned users with 403 user_banned. Banned users can still
// sign in, read their own account and appeal, so it guards only the actions
// they must not take while banned. Must run after AuthRequired.
func NotBanned(users *repo.UserRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserIDFromCtx(c)
		if uid == "" {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		u, err := users.FindByID(c.Request.Context(), uid)
		if err != nil {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		if u.Banned {
			errcode.Respond(c, service.ErrUserBanned)
			return
		}
		c.Next()
	}
}
