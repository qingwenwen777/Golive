package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

const (
	HeaderAuthorization = "Authorization"
	HeaderUserID        = "X-User-Id"
	bearerPrefix        = "Bearer "
)

// PublicRoute uniquely identifies a method+path pair that bypasses JWT.
type PublicRoute struct {
	Method string
	Path   string
}

// JWT validates access tokens with a single shared secret.
func JWT(secret string, public []PublicRoute) gin.HandlerFunc {
	keys, err := jwtauth.NewKeySet(secret, "", nil)
	if err != nil {
		panic("api-gateway jwt key set: " + err.Error())
	}
	return JWTWithKeySet(keys, public)
}

// JWTWithKeySet validates access tokens and injects X-User-Id for upstreams.
func JWTWithKeySet(keys *jwtauth.KeySet, public []PublicRoute) gin.HandlerFunc {
	publicSet := make(map[string]struct{}, len(public))
	for _, r := range public {
		publicSet[r.Method+" "+r.Path] = struct{}{}
	}
	unauthorized := errcode.New(401, "Unauthorized")

	return func(c *gin.Context) {
		c.Request.Header.Del(HeaderUserID)

		key := c.Request.Method + " " + c.FullPath()
		if _, ok := publicSet[key]; ok {
			if uid, err := parseFromHeader(c.GetHeader(HeaderAuthorization), keys); err == nil {
				c.Request.Header.Set(HeaderUserID, uid)
				c.Set("userID", uid)
			}
			c.Next()
			return
		}

		uid, err := parseFromHeader(c.GetHeader(HeaderAuthorization), keys)
		if err != nil {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Request.Header.Set(HeaderUserID, uid)
		c.Set("userID", uid)
		c.Next()
	}
}

func parseFromHeader(raw string, keys *jwtauth.KeySet) (string, error) {
	if !strings.HasPrefix(raw, bearerPrefix) {
		return "", errors.New("missing bearer")
	}
	return keys.VerifyAccess(strings.TrimPrefix(raw, bearerPrefix))
}
