package jwtauth

import (
	"bytes"
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type KeyConfig struct {
	KID            string `mapstructure:"kid"`
	Alg            string `mapstructure:"alg"`
	Secret         string `mapstructure:"secret"`
	PrivateKey     string `mapstructure:"private_key"`
	PrivateKeyFile string `mapstructure:"private_key_file"`
	PublicKey      string `mapstructure:"public_key"`
	PublicKeyFile  string `mapstructure:"public_key_file"`
}

type KeySet struct {
	active     key
	byKID      map[string]key
	verifyKeys []key
}

type key struct {
	kid        string
	alg        string
	secret     []byte
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

func NewKeySet(legacySecret, activeKID string, configured []KeyConfig) (*KeySet, error) {
	activeKID = strings.TrimSpace(activeKID)
	if len(configured) == 0 {
		if legacySecret == "" {
			return nil, errors.New("jwt secret is required")
		}
		k := key{alg: jwt.SigningMethodHS256.Alg(), secret: []byte(legacySecret)}
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
		if _, exists := byKID[kid]; exists {
			return nil, fmt.Errorf("duplicate jwt kid %q", kid)
		}
		k, err := newKey(cfg)
		if err != nil {
			return nil, fmt.Errorf("jwt.secrets[%d]: %w", i, err)
		}
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
		verifyKeys = append(verifyKeys, key{alg: jwt.SigningMethodHS256.Alg(), secret: []byte(legacySecret)})
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
	if ks == nil {
		return "", errors.New("jwt key set is not configured")
	}
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
		"typ": "access",
	}
	method, signingKey, err := ks.active.signingMaterial()
	if err != nil {
		return "", err
	}
	tok := jwt.NewWithClaims(method, claims)
	if ks.active.kid != "" {
		tok.Header["kid"] = ks.active.kid
	}
	return tok.SignedString(signingKey)
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
		return verifyAccessWithJWTKey(token, k)
	}

	for _, k := range ks.verifyKeys {
		if uid, err := verifyAccessWithJWTKey(token, k); err == nil {
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
	return verifyAccessWithJWTKey(token, key{alg: jwt.SigningMethodHS256.Alg(), secret: secret})
}

func verifyAccessWithJWTKey(token string, k key) (string, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		func(t *jwt.Token) (any, error) { return k.verificationMaterial() },
		jwt.WithValidMethods([]string{k.alg}),
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

func (k key) signingMaterial() (jwt.SigningMethod, any, error) {
	switch k.alg {
	case jwt.SigningMethodHS256.Alg():
		if len(k.secret) == 0 {
			return nil, nil, errors.New("jwt hs256 signing secret is not configured")
		}
		return jwt.SigningMethodHS256, k.secret, nil
	case jwt.SigningMethodRS256.Alg():
		if k.privateKey == nil {
			return nil, nil, errors.New("jwt rs256 private key is not configured")
		}
		return jwt.SigningMethodRS256, k.privateKey, nil
	default:
		return nil, nil, fmt.Errorf("unsupported jwt alg %q", k.alg)
	}
}

func (k key) verificationMaterial() (any, error) {
	switch k.alg {
	case jwt.SigningMethodHS256.Alg():
		if len(k.secret) == 0 {
			return nil, ErrInvalidToken
		}
		return k.secret, nil
	case jwt.SigningMethodRS256.Alg():
		if k.publicKey == nil {
			return nil, ErrInvalidToken
		}
		return k.publicKey, nil
	default:
		return nil, ErrInvalidToken
	}
}

func newKey(cfg KeyConfig) (key, error) {
	kid := strings.TrimSpace(cfg.KID)
	alg := strings.ToUpper(strings.TrimSpace(cfg.Alg))
	if alg == "" {
		alg = jwt.SigningMethodHS256.Alg()
	}
	k := key{kid: kid, alg: alg}

	switch alg {
	case jwt.SigningMethodHS256.Alg():
		if cfg.Secret == "" {
			return key{}, errors.New("secret is required for HS256")
		}
		k.secret = []byte(cfg.Secret)
		return k, nil
	case jwt.SigningMethodRS256.Alg():
		privatePEM, err := readKeyMaterial(cfg.PrivateKey, cfg.PrivateKeyFile)
		if err != nil {
			return key{}, fmt.Errorf("read private key: %w", err)
		}
		publicPEM, err := readKeyMaterial(cfg.PublicKey, cfg.PublicKeyFile)
		if err != nil {
			return key{}, fmt.Errorf("read public key: %w", err)
		}
		if len(privatePEM) == 0 && len(publicPEM) == 0 {
			return key{}, errors.New("private_key/private_key_file or public_key/public_key_file is required for RS256")
		}
		if len(privatePEM) > 0 {
			privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privatePEM)
			if err != nil {
				return key{}, fmt.Errorf("parse private key: %w", err)
			}
			k.privateKey = privateKey
			k.publicKey = &privateKey.PublicKey
		}
		if len(publicPEM) > 0 {
			publicKey, err := jwt.ParseRSAPublicKeyFromPEM(publicPEM)
			if err != nil {
				return key{}, fmt.Errorf("parse public key: %w", err)
			}
			k.publicKey = publicKey
		}
		return k, nil
	default:
		return key{}, fmt.Errorf("unsupported alg %q", alg)
	}
}

func readKeyMaterial(inline, path string) ([]byte, error) {
	inline = strings.TrimSpace(inline)
	path = strings.TrimSpace(path)
	if inline != "" {
		return []byte(strings.ReplaceAll(inline, `\n`, "\n")), nil
	}
	if path == "" {
		return nil, nil
	}
	return os.ReadFile(path)
}

func containsSecret(keys []key, secret []byte) bool {
	for _, k := range keys {
		if bytes.Equal(k.secret, secret) {
			return true
		}
	}
	return false
}
