package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

// AuthRequired validates the access token. The api-gateway already verifies
// the JWT for us and writes X-User-Id, but this defense-in-depth check
// ensures direct hits to gift-service (e.g. internal LB bypass) still
// require a valid token. We accept either:
//
//	X-User-Id (set by api-gateway after its own JWT check)
//	Authorization: Bearer <jwt>     (verified locally)
func AuthRequired(secret string) gin.HandlerFunc {
	keyBytes := []byte(secret)
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
		uid, err := parseJWT(strings.TrimPrefix(raw, prefix), keyBytes)
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

func parseJWT(token string, secret []byte) (string, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return "", errors.New("alg")
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("sub")
	}
	return sub, nil
}
