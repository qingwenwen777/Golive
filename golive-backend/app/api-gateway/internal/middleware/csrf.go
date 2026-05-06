package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

const HeaderCSRFToken = "X-CSRF-Token"

var errInvalidCSRF = errcode.New(http.StatusForbidden, "Invalid CSRF token").WithReason("csrf_invalid")

type CSRFProtector struct {
	secret         []byte
	tokenTTL       time.Duration
	allowedOrigins map[string]struct{}
}

func NewCSRFProtector(secret string, tokenTTL time.Duration, allowedOrigins []string) (*CSRFProtector, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("csrf secret is required")
	}
	if tokenTTL <= 0 {
		tokenTTL = 12 * time.Hour
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if normalized, ok := canonicalOrigin(origin); ok && normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	return &CSRFProtector{
		secret:         []byte(secret),
		tokenTTL:       tokenTTL,
		allowedOrigins: allowed,
	}, nil
}

func (p *CSRFProtector) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		origin, ok := canonicalOrigin(c.GetHeader("Origin"))
		if !ok {
			errcode.Respond(c, errInvalidCSRF)
			return
		}
		if origin == "" {
			c.Next()
			return
		}
		if !p.originAllowed(c.Request, origin) {
			errcode.Respond(c, errInvalidCSRF)
			return
		}
		if !p.verify(c.GetHeader(HeaderCSRFToken), origin, time.Now()) {
			errcode.Respond(c, errInvalidCSRF)
			return
		}
		c.Next()
	}
}

func (p *CSRFProtector) Token(c *gin.Context) {
	origin, ok := canonicalOrigin(c.GetHeader("Origin"))
	if !ok {
		errcode.Respond(c, errInvalidCSRF)
		return
	}
	if origin == "" {
		origin, ok = requestOrigin(c.Request)
		if !ok {
			errcode.Respond(c, errInvalidCSRF)
			return
		}
	}
	if !p.originAllowed(c.Request, origin) {
		errcode.Respond(c, errInvalidCSRF)
		return
	}
	token, err := p.sign(origin, time.Now())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (p *CSRFProtector) sign(origin string, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	payload := ts + "." + nonceText
	sig := p.mac(origin, payload)
	return payload + "." + sig, nil
}

func (p *CSRFProtector) verify(token, origin string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	issuedAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	issued := time.Unix(issuedAt, 0)
	if now.Before(issued.Add(-time.Minute)) || now.After(issued.Add(p.tokenTTL)) {
		return false
	}
	payload := parts[0] + "." + parts[1]
	expected := p.mac(origin, payload)
	return hmac.Equal([]byte(expected), []byte(parts[2]))
}

func (p *CSRFProtector) mac(origin, payload string) string {
	mac := hmac.New(sha256.New, p.secret)
	_, _ = mac.Write([]byte(origin))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (p *CSRFProtector) originAllowed(r *http.Request, origin string) bool {
	if sameOrigin, ok := requestOrigin(r); ok && origin == sameOrigin {
		return true
	}
	_, ok := p.allowedOrigins[origin]
	return ok
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func requestOrigin(r *http.Request) (string, bool) {
	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return canonicalOrigin(proto + "://" + host)
}

func canonicalOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(u.Host), true
}

func firstHeaderValue(raw string) string {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}
