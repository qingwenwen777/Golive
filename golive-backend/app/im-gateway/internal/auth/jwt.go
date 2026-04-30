// Package auth verifies handshake tokens. The contract is:
//
//	token absent  → anonymous (read-only)
//	token invalid → reject handshake (HTTP 401)
//	token valid   → authenticated, can chat
package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a token is present but does not validate.
var ErrInvalidToken = errors.New("invalid token")

// Identity is what the WS handler attaches to each connection.
type Identity struct {
	UserID    string // empty for anonymous
	Anonymous bool
}

func (i Identity) CanChat() bool { return !i.Anonymous && i.UserID != "" }

// Verifier is intentionally tiny so tests can mock it.
type Verifier interface {
	Verify(token string) (Identity, error)
}

type HMACVerifier struct {
	secret []byte
}

func NewHMACVerifier(secret string) *HMACVerifier {
	return &HMACVerifier{secret: []byte(secret)}
}

// Verify implements the contract above. An empty token returns an anonymous
// identity (NOT an error). Any other failure returns ErrInvalidToken.
func (v *HMACVerifier) Verify(token string) (Identity, error) {
	if token == "" {
		return Identity{Anonymous: true}, nil
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return v.secret, nil
	})
	if err != nil || !parsed.Valid {
		return Identity{}, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Identity{}, ErrInvalidToken
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Identity{}, ErrInvalidToken
	}
	return Identity{UserID: sub}, nil
}
