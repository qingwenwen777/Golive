package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

// AuthRequired accepts X-User-Id from api-gateway or verifies a direct JWT.
func AuthRequired(secret string) gin.HandlerFunc {
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	if err != nil {
		panic("gift-service jwt key set: " + err.Error())
	}
	return AuthRequiredWithKeySet(keys)
}

func AuthRequiredWithKeySet(keys *jwtauth.KeySet) gin.HandlerFunc {
	unauthorized := errcode.New(401, "Unauthorized")
	return func(c *gin.Context) {
		if uid := c.GetHeader("X-User-Id"); uid != "" {
			c.Set("userID", uid)
			c.Next()
			return
		}
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			errcode.Respond(c, unauthorized)
			return
		}
		uid, err := keys.VerifyAccess(strings.TrimPrefix(raw, prefix))
		if err != nil {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Set("userID", uid)
		c.Next()
	}
}

func UserIDFromCtx(c *gin.Context) string {
	v, ok := c.Get("userID")
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
