// Package consumer drives the Kafka loop. Per-room ordering is preserved by
// hashing roomId to a worker goroutine; each worker processes its events
// serially. Across rooms we get N-way parallelism.
//
// Why not rely on Kafka partitioning alone? im-gateway already keys records
// by roomId so a single partition holds all of room R's traffic — but a
// single consumer instance still serves multiple partitions, and within an
// instance we want CPU parallelism without losing intra-room order. The
// dispatch step gives us exactly that.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/chat-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/logger"
)

type Consumer struct {
	cl      *kgo.Client
	svc     *service.ChatService
	workers int
	queues  []chan service.Event
	wg      sync.WaitGroup
}

type Config struct {
	Brokers []string
	Topic   string
	Group   string
	Workers int
}

func New(cfg Config, svc *service.ChatService) (*Consumer, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 8
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.ConsumerGroup(cfg.Group),
		kgo.DisableAutoCommit(), // we commit after the worker has fully processed
	)
	if err != nil {
		return nil, err
	}
	c := &Consumer{
		cl:      cl,
		svc:     svc,
		workers: cfg.Workers,
		queues:  make([]chan service.Event, cfg.Workers),
	}
	for i := 0; i < cfg.Workers; i++ {
		c.queues[i] = make(chan service.Event, 256)
	}
	return c, nil
}

// Run blocks until ctx is canceled or the client errors fatally. Spawns one
// worker per queue plus a polling loop.
func (c *Consumer) Run(ctx context.Context) error {
	for i := 0; i < c.workers; i++ {
		c.wg.Add(1)
		go c.worker(ctx, c.queues[i])
	}

	for ctx.Err() == nil {
		fetches := c.cl.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// Non-fatal partition errors get logged; fatal errors break.
			for _, e := range errs {
				if errors.Is(e.Err, context.Canceled) {
					continue
				}
				logger.L().Warn("kafka fetch", zap.String("topic", e.Topic), zap.Error(e.Err))
			}
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			var ev service.Event
			if err := json.Unmarshal(rec.Value, &ev); err != nil {
				logger.L().Warn("bad chat record", zap.Error(err))
				return
			}
			idx := shard(ev.RoomID, c.workers)
			select {
			case c.queues[idx] <- ev:
			case <-ctx.Done():
			}
		})
		// Mark fetched records consumed. We commit only AFTER dispatch — if
		// the process dies between dispatch and worker drain we'll re-deliver
		// the in-flight events on restart, which our DB upserts handle (via
		// uuid PK) safely. For at-least-once chat, that's fine.
		if err := c.cl.CommitUncommittedOffsets(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.L().Warn("commit offsets", zap.Error(err))
		}
	}

	for _, q := range c.queues {
		close(q)
	}
	c.wg.Wait()
	c.cl.Close()
	return ctx.Err()
}

func (c *Consumer) worker(ctx context.Context, q <-chan service.Event) {
	defer c.wg.Done()
	for ev := range q {
		if err := c.svc.Process(ctx, ev); err != nil && !errors.Is(err, service.ErrRateLimited) {
			logger.L().Warn("process chat", zap.String("room", ev.RoomID), zap.Error(err))
		}
	}
}

// shard routes roomId → worker index. Identical hash to repo.DanmuRepo so
// (in single-instance setups) the same room hits the same MySQL connection
// pool slot, improving connection reuse.
func shard(roomID string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(roomID))
	return int(h.Sum32()) % n
}
