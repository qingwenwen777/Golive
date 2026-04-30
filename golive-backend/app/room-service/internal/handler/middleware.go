package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

const ctxUserIDKey = "userID"

// AuthRequired verifies the Bearer JWT against the shared HS256 secret and
// stashes the user id (sub) in the context. Returns 401 on any failure.
func AuthRequired(secret string) gin.HandlerFunc {
	keyBytes := []byte(secret)
	unauthorized := errcode.New(401, "Unauthorized")
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			errcode.Respond(c, unauthorized)
			return
		}
		uid, err := parseAccess(strings.TrimPrefix(raw, prefix), keyBytes)
		if err != nil || uid == "" {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Set(ctxUserIDKey, uid)
		c.Next()
	}
}

// OptionalAuth parses a Bearer token when present and stores the user id. It
// never blocks public endpoints; invalid or absent tokens behave as guests.
func OptionalAuth(secret string) gin.HandlerFunc {
	keyBytes := []byte(secret)
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if strings.HasPrefix(raw, prefix) {
			if uid, err := parseAccess(strings.TrimPrefix(raw, prefix), keyBytes); err == nil && uid != "" {
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

func parseAccess(token string, secret []byte) (string, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing sub")
	}
	return sub, nil
}
