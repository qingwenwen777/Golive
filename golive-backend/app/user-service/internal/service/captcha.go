package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
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
	code, err := randomCaptchaCode(6)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if err := s.rdb.Set(ctx, captchaKey(id), strings.ToLower(code), s.ttl).Err(); err != nil {
		return nil, err
	}
	image, err := renderCaptchaSVG(code)
	if err != nil {
		return nil, err
	}
	return &CaptchaChallenge{
		ID:        id,
		Image:     image,
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

func renderCaptchaSVG(code string) (string, error) {
	const width = 132
	const height = 44
	rng := &captchaRNG{}
	filterSeed := rng.rangeInt(1, 9999)
	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&svg, `<defs><filter id="warp"><feTurbulence type="fractalNoise" baseFrequency="0.028" numOctaves="2" seed="%d"/><feDisplacementMap in="SourceGraphic" scale="1.8"/></filter></defs>`, filterSeed)
	svg.WriteString(`<rect width="132" height="44" rx="8" fill="#f3f4f6"/>`)
	svg.WriteString(`<rect x="1" y="1" width="130" height="42" rx="7" fill="none" stroke="#e5e7eb"/>`)
	for i := 0; i < 42; i++ {
		x := rng.rangeInt(2, width-3)
		y := rng.rangeInt(2, height-3)
		size := rng.rangeInt(1, 2)
		opacity := rng.rangeInt(18, 42)
		fmt.Fprintf(&svg, `<circle cx="%d" cy="%d" r="%d" fill="%s" opacity="0.%02d"/>`, x, y, size, rng.choice(captchaNoiseColors), opacity)
	}
	for i := 0; i < 7; i++ {
		x1 := rng.rangeInt(-8, width/3)
		y1 := rng.rangeInt(6, height-6)
		c1x := rng.rangeInt(18, width/2)
		c1y := rng.rangeInt(-6, height+6)
		c2x := rng.rangeInt(width/2, width-18)
		c2y := rng.rangeInt(-6, height+6)
		x2 := rng.rangeInt((width*2)/3, width+8)
		y2 := rng.rangeInt(6, height-6)
		strokeWidth := rng.rangeInt(1, 3)
		opacity := rng.rangeInt(32, 62)
		fmt.Fprintf(&svg, `<path d="M%d %d C%d %d,%d %d,%d %d" fill="none" stroke="%s" stroke-width="%d" stroke-linecap="round" opacity="0.%02d"/>`, x1, y1, c1x, c1y, c2x, c2y, x2, y2, rng.choice(captchaStrokeColors), strokeWidth, opacity)
	}
	runes := []rune(code)
	for i, ch := range runes {
		x := 4 + i*21 + rng.rangeInt(-1, 1)
		y := 8 + rng.rangeInt(-1, 1)
		rotate := rng.rangeInt(-18, 18)
		fmt.Fprintf(&svg, `<g filter="url(#warp)" transform="rotate(%d %d %d)">`, rotate, x+9, y+14)
		writeCaptchaGlyph(&svg, rng, ch, x, y)
		svg.WriteString(`</g>`)
	}
	for i := 0; i < 2; i++ {
		x := rng.rangeInt(0, width-20)
		y := rng.rangeInt(10, height-12)
		fmt.Fprintf(&svg, `<rect x="%d" y="%d" width="%d" height="2" rx="1" fill="%s" opacity="0.45" transform="rotate(%d %d %d)"/>`, x, y, rng.rangeInt(36, 68), rng.choice(captchaStrokeColors), rng.rangeInt(-12, 12), x, y)
	}
	svg.WriteString(`</svg>`)
	if rng.err != nil {
		return "", rng.err
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg.String())), nil
}

var captchaTextColors = []string{"#111827", "#1f2937", "#172554", "#3b0764", "#713f12"}

var captchaNoiseColors = []string{"#9ca3af", "#93c5fd", "#f59e0b", "#a78bfa", "#6ee7b7"}

var captchaStrokeColors = []string{"#93c5fd", "#f59e0b", "#a78bfa", "#94a3b8", "#60a5fa"}

var captchaGlyphs = map[rune][7]string{
	'2': {"11110", "00001", "00001", "01110", "10000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"10010", "10010", "10010", "11111", "00010", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01111", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "11110"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "01010", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "11011", "10001"},
	'X': {"10001", "01010", "00100", "00100", "00100", "01010", "10001"},
	'Y': {"10001", "01010", "00100", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}

func writeCaptchaGlyph(svg *strings.Builder, rng *captchaRNG, ch rune, originX, originY int) {
	pattern, ok := captchaGlyphs[ch]
	if !ok {
		return
	}
	for row, line := range pattern {
		for col, bit := range line {
			if bit != '1' {
				continue
			}
			x := originX + col*4 + rng.rangeInt(-1, 1)
			y := originY + row*4 + rng.rangeInt(-1, 1)
			rotate := rng.rangeInt(-9, 9)
			fmt.Fprintf(svg, `<rect x="%d" y="%d" width="3.2" height="3.6" rx="0.8" fill="%s" transform="rotate(%d %d %d)"/>`, x, y, rng.choice(captchaTextColors), rotate, x+2, y+2)
		}
	}
}

type captchaRNG struct {
	err error
}

func (r *captchaRNG) rangeInt(min, max int) int {
	if max < min {
		return min
	}
	return min + r.int(max-min+1)
}

func (r *captchaRNG) choice(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[r.int(len(values))]
}

func (r *captchaRNG) int(max int) int {
	if r.err != nil || max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		r.err = err
		return 0
	}
	return int(n.Int64())
}
