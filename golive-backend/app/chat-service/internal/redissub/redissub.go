// Package redissub is a passive listener on the Redis pub/sub channels that
// im-gateway broadcasts live chat into ("room:<id>"). Its sole job is to
// persist incoming chat messages into the sharded danmus tables so that
// GET /chat/rooms/:id/danmus can return real history when a viewer re-enters.
//
// Why this exists: in the current deployment im-gateway runs with kafka
// disabled, so the only chat path is the direct Redis broadcast it produces
// itself. Without this subscriber, chats are never written to MySQL. SuperChats
// are persisted separately by gift-service, which is why a re-entering viewer
// previously only saw SuperChats restored.
//
// This subscriber is idempotent: it uses the message id (set by im-gateway
// from the WS clientId or a fresh uuid) as the danmus row primary key, so
// reprocessing the same payload does not duplicate.
package redissub

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-redis/redis/v9"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// pattern matches the channel layout used by im-gateway's pubsub.RoomChannel.
const pattern = "room:*"

// Subscriber listens to room:* and writes "chat" messages to MySQL.
type Subscriber struct {
	rdb     *redis.Client
	danmus  *repo.DanmuRepo
	channel string
}

func New(rdb *redis.Client, danmus *repo.DanmuRepo) *Subscriber {
	return &Subscriber{rdb: rdb, danmus: danmus, channel: pattern}
}

// inboundChat mirrors what hub.EncodeChat / chat-service's own publisher emit.
// We only care about chat-type messages; super_chat / gift / system are
// either already persisted elsewhere or have no need to be stored.
type inboundChat struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	User   string `json:"user"`
	Avatar string `json:"avatar,omitempty"`
	Text   string `json:"text"`
	Color  string `json:"color,omitempty"`
	Ts     int64  `json:"ts"`
}

// Run blocks until ctx is canceled. It reconnects implicitly via go-redis on
// transient broker errors (PSubscribe handles that internally).
func (s *Subscriber) Run(ctx context.Context) error {
	ps := s.rdb.PSubscribe(ctx, s.channel)
	defer ps.Close()

	// Wait for the subscription to be established before consuming, so an
	// early ctx-cancel returns cleanly.
	if _, err := ps.Receive(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	logger.L().Info("redis chat subscriber started", zap.String("pattern", s.channel))

	ch := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			s.handle(ctx, msg)
		}
	}
}

func (s *Subscriber) handle(ctx context.Context, msg *redis.Message) {
	roomID := strings.TrimPrefix(msg.Channel, "room:")
	if roomID == "" || roomID == msg.Channel {
		return
	}
	var ev inboundChat
	if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
		// Non-JSON or unexpected shape — ignore.
		return
	}
	if ev.Type != "chat" || ev.ID == "" || ev.Text == "" {
		// We only persist chat. SuperChat is handled by gift-service.
		return
	}
	d := &model.Danmu{
		ID:       ev.ID,
		RoomID:   roomID,
		Username: ev.User,
		Avatar:   ev.Avatar,
		Text:     ev.Text,
		Color:    ev.Color,
		Ts:       ev.Ts,
	}
	if err := s.danmus.Insert(ctx, d); err != nil {
		// Duplicate key (re-delivery) is fine — log at debug only.
		if isDuplicateKey(err) {
			return
		}
		logger.L().Warn("persist live chat",
			zap.String("room", roomID),
			zap.String("id", ev.ID),
			zap.Error(err))
	}
}

// isDuplicateKey is a soft check so we don't spam warn logs on safe
// idempotent re-inserts. We don't depend on a specific MySQL driver type to
// avoid coupling — substring match is enough for log filtering.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") ||
		strings.Contains(msg, "duplicate key")
}
