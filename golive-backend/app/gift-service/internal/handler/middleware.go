package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

// AuthRequired verifies the caller's JWT and stores the authenticated user id.
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
		uid, err := userIDFromBearer(c, keys)
		if err != nil {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Set("userID", uid)
		c.Next()
	}
}

func OptionalAuthWithKeySet(keys *jwtauth.KeySet) gin.HandlerFunc {
	return func(c *gin.Context) {
		if uid, err := userIDFromBearer(c, keys); err == nil {
			c.Set("userID", uid)
		}
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

func userIDFromBearer(c *gin.Context, keys *jwtauth.KeySet) (string, error) {
	raw := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return "", jwtauth.ErrInvalidToken
	}
	return keys.VerifyAccess(strings.TrimPrefix(raw, prefix))
}
