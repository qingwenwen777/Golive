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
	"strings"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/chatfilter"
	"github.com/qingwenwen777/golive/pkg/chatlimit"
)

// Event is the Kafka payload im-gateway puts on the danmu topic. im-gateway
// derives ID and every display field server-side; only Text is user input.
type Event struct {
	ID        string                 `json:"id"`
	RoomID    string                 `json:"roomId"`
	UserID    string                 `json:"userId"`
	Username  string                 `json:"username,omitempty"`
	Avatar    string                 `json:"avatar,omitempty"`
	Text      string                 `json:"text"`
	Role      string                 `json:"role,omitempty"`
	FanBadge  *model.FanBadgePayload `json:"fanBadge,omitempty"`
	UserLevel int                    `json:"userLevel,omitempty"`
	Ts        int64                  `json:"ts"`
}

// ErrRateLimited is returned to the caller when an event is dropped by the
// limiter. The caller decides whether to surface this (e.g. metrics, system
// nudge to client). It is NOT a fatal error.
var ErrRateLimited = errors.New("rate limited")

type ChatService struct {
	filter  *chatfilter.Filter
	limiter *chatlimit.Limiter
	danmus  *repo.DanmuRepo
	pub     *repo.Publisher
}

func New(f *chatfilter.Filter, l *chatlimit.Limiter, d *repo.DanmuRepo, p *repo.Publisher) *ChatService {
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

	// Message ids are server-generated uuids (never a client-chosen string
	// that could collide with, and overwrite, someone else's message).
	id := ev.ID
	if _, err := uuid.Parse(id); err != nil {
		id = uuid.NewString()
	}
	fanBadge := safeFanBadge(ev.FanBadge)

	d := &model.Danmu{
		ID:        id,
		RoomID:    ev.RoomID,
		UserID:    ev.UserID,
		Username:  username,
		Avatar:    ev.Avatar,
		Text:      cleanText,
		Role:      safeRole(ev.Role),
		UserLevel: safeUserLevel(ev.UserLevel),
		Ts:        ev.Ts,
	}
	if fanBadge != nil {
		d.FanBadgeCreatorID = fanBadge.CreatorID
		d.FanBadgeLevel = fanBadge.Level
	}
	if err := s.danmus.Insert(ctx, d); err != nil {
		return fmt.Errorf("persist: %w", err)
	}
	pub := d.ToPublic()
	payload, err := json.Marshal(pub)
	if err != nil {
		return err
	}
	if err := s.pub.Publish(ctx, ev.RoomID, payload); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

func safeFanBadge(in *model.FanBadgePayload) *model.FanBadgePayload {
	if in == nil || strings.TrimSpace(in.CreatorID) == "" || in.Level < 1 {
		return nil
	}
	creatorID := strings.TrimSpace(in.CreatorID)
	if len(creatorID) > 80 || strings.ContainsAny(creatorID, " \t\r\n") {
		return nil
	}
	level := in.Level
	if level > 99 {
		level = 99
	}
	return &model.FanBadgePayload{CreatorID: creatorID, Level: level}
}

func safeRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "moderator" {
		return role
	}
	return ""
}

func safeUserLevel(level int) int {
	if level < 1 {
		return 0
	}
	if level > 99 {
		return 99
	}
	return level
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
	userIDs := make([]string, 0, len(rows))
	for i := range rows {
		userIDs = append(userIDs, rows[i].UserID)
	}
	fanBadges, err := s.danmus.FanBadgesForRoomUsers(ctx, roomID, userIDs)
	if err != nil {
		return nil, err
	}
	profiles, err := s.danmus.UserProfiles(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	out := make([]model.Public, 0, len(rows)+len(superChats))
	for i := range rows {
		out = append(out, decorateChat(rows[i].ToPublic(), fanBadges, profiles))
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

// formatCoinAmount returns the bare integer string. The currency symbol is
// added by the frontend so it stays under UTF-8 control end-to-end (Safari
// has been observed to mis-render a non-UTF-8-routed yen sign as \u697c).
func formatCoinAmount(amount int64) string {
	return strconv.FormatInt(amount, 10)
}

// decorateChat replaces a history chat's identity with current server-side
// data looked up by user id: the fan badge (nil if none — the badge stored
// with the row is ignored, older rows may carry a client-supplied one) and,
// when the user is known, name, avatar and level.
func decorateChat(item model.Public, badges map[string]*model.FanBadgePayload, profiles map[string]repo.UserProfile) model.Public {
	item.FanBadge = badges[item.UserID]
	if p, ok := profiles[item.UserID]; ok && item.UserID != "" {
		item.User = p.Name
		item.Avatar = p.Avatar
		item.UserLevel = p.Level
	}
	return item
}

// ErrMessageNotFound is returned by HideMessage for an unknown message.
var ErrMessageNotFound = repo.ErrDanmuNotFound

// HideMessage removes a chat message from history. It backs room-service's
// report moderation (internal endpoint).
func (s *ChatService) HideMessage(ctx context.Context, roomID, messageID string) error {
	return s.danmus.Hide(ctx, roomID, messageID)
}

// FanBadge returns userID's badge for roomID's owner, or nil. It backs
// im-gateway's live chat decoration (internal endpoint).
func (s *ChatService) FanBadge(ctx context.Context, roomID, userID string) (*model.FanBadgePayload, error) {
	badges, err := s.danmus.FanBadgesForRoomUsers(ctx, roomID, []string{userID})
	if err != nil {
		return nil, err
	}
	return badges[userID], nil
}
