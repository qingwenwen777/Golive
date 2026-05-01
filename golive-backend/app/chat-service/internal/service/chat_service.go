// Package service stitches together filter + ratelimit + persistence + publish.
//
// The orchestration is intentionally linear so the cost of each stage is
// obvious in pprof traces:
//
//	Process(event)
//	  rate-limit -> drop ?
//	  sanitize   -> mask sensitive words
//	  persist    -> MySQL shard
//	  publish    -> redis room:<id>
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/chat-service/internal/filter"
	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/app/chat-service/internal/ratelimit"
	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
)

// Event is the Kafka payload im-gateway puts on the danmu topic.
type Event struct {
	RoomID   string `json:"roomId"`
	UserID   string `json:"userId"`
	ClientID string `json:"clientId,omitempty"`
	Username string `json:"username,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Text     string `json:"text"`
	Ts       int64  `json:"ts"`
}

// ErrRateLimited is returned to the caller when an event is dropped by the
// limiter. The caller decides whether to surface this (e.g. metrics, system
// nudge to client). It is NOT a fatal error.
var ErrRateLimited = errors.New("rate limited")

type ChatService struct {
	filter  *filter.Filter
	limiter *ratelimit.Limiter
	danmus  *repo.DanmuRepo
	pub     *repo.Publisher
}

func New(f *filter.Filter, l *ratelimit.Limiter, d *repo.DanmuRepo, p *repo.Publisher) *ChatService {
	return &ChatService{filter: f, limiter: l, danmus: d, pub: p}
}

// Process runs an inbound event through the full pipeline. Errors from any
// stage abort the rest of the pipeline. ErrRateLimited is returned without
// hitting MySQL or Redis.
func (s *ChatService) Process(ctx context.Context, ev Event) error {
	if ev.RoomID == "" || ev.Text == "" {
		return errors.New("empty room or text")
	}
	if ev.UserID != "" {
		ok, err := s.limiter.Allow(ctx, ev.UserID)
		if err != nil {
			return fmt.Errorf("ratelimit: %w", err)
		}
		if !ok {
			return ErrRateLimited
		}
	}

	cleanText := s.filter.Replace(ev.Text)
	username := ev.Username
	if username == "" {
		username = ev.UserID
	}

	id := safeClientID(ev.ClientID)
	if id == "" {
		id = uuid.NewString()
	}

	d := &model.Danmu{
		ID:       id,
		RoomID:   ev.RoomID,
		UserID:   ev.UserID,
		Username: username,
		Avatar:   ev.Avatar,
		Text:     cleanText,
		Ts:       ev.Ts,
	}
	if err := s.danmus.Insert(ctx, d); err != nil {
		return fmt.Errorf("persist: %w", err)
	}
	payload, err := json.Marshal(d.ToPublic())
	if err != nil {
		return err
	}
	if err := s.pub.Publish(ctx, ev.RoomID, payload); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

func safeClientID(id string) string {
	if id == "" || len(id) > 80 {
		return ""
	}
	for _, r := range id {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return ""
		}
	}
	return id
}

// History serves GET /rooms/:id/danmus.
func (s *ChatService) History(ctx context.Context, roomID string, before int64, limit int) ([]model.Public, error) {
	rows, err := s.danmus.History(ctx, roomID, before, limit)
	if err != nil {
		return nil, err
	}
	superChats, err := s.danmus.SuperChatHistory(ctx, roomID, before, limit)
	if err != nil {
		return nil, err
	}

	out := make([]model.Public, 0, len(rows)+len(superChats))
	for i := range rows {
		out = append(out, rows[i].ToPublic())
	}
	for i := range superChats {
		tier := superChats[i].Tier
		out = append(out, model.Public{
			Type:   "super_chat",
			ID:     superChats[i].ID,
			User:   superChats[i].User,
			Avatar: superChats[i].Avatar,
			Amount: formatCoinAmount(superChats[i].Amount),
			Tier:   &tier,
			Text:   superChats[i].Text,
			Ts:     superChats[i].Ts,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Ts > out[j].Ts
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func formatCoinAmount(amount int64) string {
	raw := strconv.FormatInt(amount, 10)
	n := len(raw)
	if n <= 3 {
		return "\u697c" + raw
	}
	out := make([]byte, 0, n+(n-1)/3)
	head := n % 3
	if head == 0 {
		head = 3
	}
	out = append(out, raw[:head]...)
	for i := head; i < n; i += 3 {
		out = append(out, ',')
		out = append(out, raw[i:i+3]...)
	}
	return "\u697c" + string(out)
}
