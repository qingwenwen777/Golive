package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/qingwenwen777/golive/pkg/errcode"
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

// JWT validates the access token (HS256). On success, the user id is written
// to the request header X-User-Id so the upstream can trust it. On failure
// for non-public routes it short-circuits with 401 — never let the request
// reach the upstream with a forged or absent token.
func JWT(secret string, public []PublicRoute) gin.HandlerFunc {
	keyBytes := []byte(secret)
	publicSet := make(map[string]struct{}, len(public))
	for _, r := range public {
		publicSet[r.Method+" "+r.Path] = struct{}{}
	}
	unauthorized := errcode.New(401, "Unauthorized")

	return func(c *gin.Context) {
		// Always strip any inbound X-User-Id — only we get to set it.
		c.Request.Header.Del(HeaderUserID)

		key := c.Request.Method + " " + c.FullPath()
		if _, ok := publicSet[key]; ok {
			// Optional pass-through: if the client did send a valid token,
			// still attach X-User-Id so upstreams can correlate. Failure on
			// a public route is non-fatal.
			if uid, err := parseFromHeader(c.GetHeader(HeaderAuthorization), keyBytes); err == nil {
				c.Request.Header.Set(HeaderUserID, uid)
				c.Set("userID", uid)
			}
			c.Next()
			return
		}

		uid, err := parseFromHeader(c.GetHeader(HeaderAuthorization), keyBytes)
		if err != nil {
			errcode.Respond(c, unauthorized)
			return
		}
		c.Request.Header.Set(HeaderUserID, uid)
		c.Set("userID", uid)
		c.Next()
	}
}

func parseFromHeader(raw string, secret []byte) (string, error) {
	if !strings.HasPrefix(raw, bearerPrefix) {
		return "", errors.New("missing bearer")
	}
	tok, err := jwt.Parse(strings.TrimPrefix(raw, bearerPrefix), func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !tok.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing sub")
	}
	return sub, nil
}
