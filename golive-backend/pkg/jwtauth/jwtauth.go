package jwtauth

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type KeyConfig struct {
	KID    string `mapstructure:"kid"`
	Secret string `mapstructure:"secret"`
}

type KeySet struct {
	active     key
	byKID      map[string]key
	verifyKeys []key
}

type key struct {
	kid    string
	secret []byte
}

func NewKeySet(legacySecret, activeKID string, configured []KeyConfig) (*KeySet, error) {
	activeKID = strings.TrimSpace(activeKID)
	if len(configured) == 0 {
		if legacySecret == "" {
			return nil, errors.New("jwt secret is required")
		}
		k := key{secret: []byte(legacySecret)}
		return &KeySet{
			active:     k,
			byKID:      map[string]key{},
			verifyKeys: []key{k},
		}, nil
	}

	byKID := make(map[string]key, len(configured))
	ordered := make([]key, 0, len(configured)+1)
	for i, cfg := range configured {
		kid := strings.TrimSpace(cfg.KID)
		if kid == "" {
			return nil, fmt.Errorf("jwt.secrets[%d].kid is required", i)
		}
		if cfg.Secret == "" {
			return nil, fmt.Errorf("jwt.secrets[%d].secret is required", i)
		}
		if _, exists := byKID[kid]; exists {
			return nil, fmt.Errorf("duplicate jwt kid %q", kid)
		}
		k := key{kid: kid, secret: []byte(cfg.Secret)}
		byKID[kid] = k
		ordered = append(ordered, k)
	}

	if activeKID == "" {
		activeKID = strings.TrimSpace(configured[0].KID)
	}
	active, ok := byKID[activeKID]
	if !ok {
		return nil, fmt.Errorf("active jwt kid %q is not configured", activeKID)
	}

	verifyKeys := []key{active}
	for _, k := range ordered {
		if k.kid != active.kid {
			verifyKeys = append(verifyKeys, k)
		}
	}
	if legacySecret != "" && !containsSecret(verifyKeys, []byte(legacySecret)) {
		verifyKeys = append(verifyKeys, key{secret: []byte(legacySecret)})
	}

	return &KeySet{active: active, byKID: byKID, verifyKeys: verifyKeys}, nil
}

func (ks *KeySet) ActiveKID() string {
	if ks == nil {
		return ""
	}
	return ks.active.kid
}

func (ks *KeySet) SignAccess(userID string, now time.Time, ttl time.Duration) (string, error) {
	if ks == nil || len(ks.active.secret) == 0 {
		return "", errors.New("jwt key set is not configured")
	}
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"typ": "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if ks.active.kid != "" {
		tok.Header["kid"] = ks.active.kid
	}
	return tok.SignedString(ks.active.secret)
}

func (ks *KeySet) VerifyAccess(token string) (string, error) {
	if ks == nil || len(ks.verifyKeys) == 0 {
		return "", ErrInvalidToken
	}

	kid, err := headerKID(token)
	if err != nil {
		return "", ErrInvalidToken
	}
	if kid != "" {
		k, ok := ks.byKID[kid]
		if !ok {
			return "", ErrInvalidToken
		}
		return verifyAccessWithKey(token, k.secret)
	}

	for _, k := range ks.verifyKeys {
		if uid, err := verifyAccessWithKey(token, k.secret); err == nil {
			return uid, nil
		}
	}
	return "", ErrInvalidToken
}

func headerKID(token string) (string, error) {
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	kid, _ := parsed.Header["kid"].(string)
	return strings.TrimSpace(kid), nil
}

func verifyAccessWithKey(token string, secret []byte) (string, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		func(t *jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil || !parsed.Valid {
		return "", ErrInvalidToken
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", ErrInvalidToken
	}
	return sub, nil
}

func containsSecret(keys []key, secret []byte) bool {
	for _, k := range keys {
		if bytes.Equal(k.secret, secret) {
			return true
		}
	}
	return false
}
