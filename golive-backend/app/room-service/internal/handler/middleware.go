package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

const ctxUserIDKey = "userID"

// AuthRequired verifies the Bearer JWT and stashes the user id in the context.
func AuthRequired(secret string) gin.HandlerFunc {
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	if err != nil {
		panic("room-service jwt key set: " + err.Error())
	}
	return AuthRequiredWithKeySet(keys)
}

func AuthRequiredWithKeySet(keys *jwtauth.KeySet) gin.HandlerFunc {
	unauthorized := errcode.New(401, "Unauthorized")
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			errcode.Respond(c, unauthorized)
			return
		}
		uid, err := keys.VerifyAccess(strings.TrimPrefix(raw, prefix))
		if err != nil || uid == "" {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Set(ctxUserIDKey, uid)
		c.Next()
	}
}

// OptionalAuth stores the user id when a valid token is present.
func OptionalAuth(secret string) gin.HandlerFunc {
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	if err != nil {
		panic("room-service jwt key set: " + err.Error())
	}
	return OptionalAuthWithKeySet(keys)
}

func OptionalAuthWithKeySet(keys *jwtauth.KeySet) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if strings.HasPrefix(raw, prefix) {
			if uid, err := keys.VerifyAccess(strings.TrimPrefix(raw, prefix)); err == nil && uid != "" {
				c.Set(ctxUserIDKey, uid)
			}
		}
		c.Next()
	}
}

func UserIDFromCtx(c *gin.Context) string {
	if v, ok := c.Get(ctxUserIDKey); ok {
		s, _ := v.(string)
		return s
	}
	return ""
}
