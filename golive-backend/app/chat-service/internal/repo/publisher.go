package repo

import (
	"context"

	"github.com/go-redis/redis/v9"
)

// Publisher fans the broadcast-ready chat message to im-gateway via Redis
// pub/sub. The channel name MUST match what im-gateway subscribes to.
type Publisher struct {
	rdb *redis.Client
}

func NewPublisher(rdb *redis.Client) *Publisher { return &Publisher{rdb: rdb} }

func (p *Publisher) Publish(ctx context.Context, roomID string, payload []byte) error {
	return p.rdb.Publish(ctx, "room:"+roomID, payload).Err()
}
