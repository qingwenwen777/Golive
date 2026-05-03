package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

var ErrInvalidCaptcha = errcode.New(http.StatusBadRequest, "Invalid captcha").WithReason("invalid_captcha")

type CaptchaChallenge struct {
	ID        string `json:"id"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expiresIn"`
}

type CaptchaService struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewCaptchaService(rdb *redis.Client, ttl time.Duration) *CaptchaService {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CaptchaService{rdb: rdb, ttl: ttl}
}

func (s *CaptchaService) Generate(ctx context.Context) (*CaptchaChallenge, error) {
	code, err := randomCaptchaCode(5)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if err := s.rdb.Set(ctx, captchaKey(id), strings.ToLower(code), s.ttl).Err(); err != nil {
		return nil, err
	}
	return &CaptchaChallenge{
		ID:        id,
		Image:     renderCaptchaSVG(code),
		ExpiresIn: int(s.ttl.Seconds()),
	}, nil
}

func (s *CaptchaService) Verify(ctx context.Context, id, answer string) error {
	id = strings.TrimSpace(id)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if id == "" || answer == "" {
		return ErrInvalidCaptcha
	}
	key := captchaKey(id)
	expected, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return ErrInvalidCaptcha
	}
	if err != nil {
		return err
	}
	_ = s.rdb.Del(ctx, key).Err()
	if answer != strings.ToLower(expected) {
		return ErrInvalidCaptcha
	}
	return nil
}

func captchaKey(id string) string {
	return "captcha:" + id
}

func randomCaptchaCode(length int) (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}

func renderCaptchaSVG(code string) string {
	escaped := html.EscapeString(code)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="132" height="44" viewBox="0 0 132 44">
<rect width="132" height="44" rx="8" fill="#f7f7f7"/>
<path d="M8 30 C28 12, 48 38, 70 20 S104 12, 124 29" fill="none" stroke="#d1d5db" stroke-width="2"/>
<path d="M12 16 C34 32, 52 10, 74 25 S106 34, 120 14" fill="none" stroke="#93c5fd" stroke-width="1.6"/>
<text x="50%%" y="29" text-anchor="middle" font-family="Arial, sans-serif" font-size="24" font-weight="800" letter-spacing="4" fill="#111827" transform="rotate(-2 66 22)">%s</text>
</svg>`, escaped)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
