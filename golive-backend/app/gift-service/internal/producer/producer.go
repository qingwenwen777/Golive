// Package producer publishes outbox-claimed events to Kafka. The interface
// keeps the worker testable without a live broker.
package producer

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

type Producer interface {
	Publish(ctx context.Context, msg *model.LocalMessage) error
	Close() error
}

// noop just succeeds — useful for local dev when kafka isn't running.
type noop struct{}

func NewNoop() Producer                                         { return noop{} }
func (noop) Publish(context.Context, *model.LocalMessage) error { return nil }
func (noop) Close() error                                       { return nil }

type redisFanout struct {
	rdb *redis.Client
}

func NewRedisFanout(rdb *redis.Client) Producer { return &redisFanout{rdb: rdb} }

func (p *redisFanout) Publish(ctx context.Context, msg *model.LocalMessage) error {
	if p.rdb == nil {
		return errors.New("redis client nil")
	}
	if msg.RoomID == "" {
		return errors.New("room id missing")
	}
	return p.rdb.Publish(ctx, "room:"+msg.RoomID, msg.Payload).Err()
}

func (p *redisFanout) Close() error { return nil }

type franzProducer struct {
	cl    *kgo.Client
	topic string
}

func NewFranz(brokers []string, topic string) (Producer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerLinger(5*time.Millisecond),
		kgo.RecordRetries(3),
	)
	if err != nil {
		return nil, err
	}
	return &franzProducer{cl: cl, topic: topic}, nil
}

func (f *franzProducer) Publish(ctx context.Context, msg *model.LocalMessage) error {
	return f.cl.ProduceSync(ctx, &kgo.Record{
		Topic: f.topic,
		Key:   []byte(msg.BizID),
		Value: []byte(msg.Payload),
		Headers: []kgo.RecordHeader{
			{Key: "topic", Value: []byte(msg.Topic)},
		},
	}).FirstErr()
}

func (f *franzProducer) Close() error {
	f.cl.Close()
	return nil
}
