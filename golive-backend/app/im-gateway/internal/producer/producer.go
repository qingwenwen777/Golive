// Package producer writes user-generated events (chat, etc.) to Kafka so
// chat-service can consume, audit, persist, and re-broadcast. The interface
// keeps the WS path independent from Kafka availability — tests use the
// no-op implementation and a real deployment swaps in Franz.
package producer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/pkg/logger"
)

// ChatEvent is what im-gateway puts on the chat topic. chat-service does
// dedup, sensitive-word filtering, persistence, and Redis publish.
type ChatEvent struct {
	RoomID   string `json:"roomId"`
	UserID   string `json:"userId"`
	Username string `json:"username,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	ClientID string `json:"clientId,omitempty"`
	Text     string `json:"text"`
	Ts       int64  `json:"ts"` // ms
}

type Producer interface {
	PublishChat(ctx context.Context, ev ChatEvent) error
	LocalEcho() bool
	Close() error
}

// noop logs and discards. Used when kafka.enabled = false (local dev).
type noop struct{}

func NewNoop() Producer { return noop{} }

func (noop) PublishChat(_ context.Context, ev ChatEvent) error {
	logger.L().Debug("noop chat", zap.String("room", ev.RoomID), zap.String("text", ev.Text))
	return nil
}
func (noop) LocalEcho() bool { return true }
func (noop) Close() error    { return nil }

// franz-backed producer.
type franzProducer struct {
	cl    *kgo.Client
	topic string
}

func NewFranz(brokers []string, topic string) (Producer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerLinger(5*time.Millisecond), // batch tiny chat events
	)
	if err != nil {
		return nil, err
	}
	return &franzProducer{cl: cl, topic: topic}, nil
}

func (f *franzProducer) PublishChat(ctx context.Context, ev ChatEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return f.cl.ProduceSync(ctx, &kgo.Record{
		Topic: f.topic,
		Key:   []byte(ev.RoomID), // partition by room → ordering within a room
		Value: body,
	}).FirstErr()
}

func (f *franzProducer) LocalEcho() bool { return false }

func (f *franzProducer) Close() error {
	f.cl.Close()
	return nil
}
