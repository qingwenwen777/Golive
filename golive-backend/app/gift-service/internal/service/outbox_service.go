// Package service / outbox runs the transactional-outbox drain loop. It
// claims pending local_messages rows in batches, hands each to the producer,
// and either marks 'sent' or reschedules with exponential backoff. After
// MaxRetries the row goes 'dead' for ops to investigate.
package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/producer"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/logger"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type OutboxConfig struct {
	PollInterval time.Duration
	BatchSize    int
	MaxRetries   int
	BaseBackoff  time.Duration
}

type OutboxService struct {
	repo *repo.OutboxRepo
	prod producer.Producer
	cfg  OutboxConfig
}

func NewOutboxService(r *repo.OutboxRepo, p producer.Producer, cfg OutboxConfig) *OutboxService {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = 2 * time.Second
	}
	return &OutboxService{repo: r, prod: p, cfg: cfg}
}

// Run blocks until ctx is canceled. Polls and drains.
func (s *OutboxService) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.drainOnce(ctx)
		}
	}
}

// drainOnce is exported via tests via the underscore alias below.
func (s *OutboxService) drainOnce(ctx context.Context) int {
	rows, err := s.repo.Claim(ctx, s.cfg.BatchSize)
	if err != nil {
		logger.L().Warn("outbox claim", zap.Error(err))
		return 0
	}
	for i := range rows {
		s.handleOne(ctx, &rows[i])
	}
	return len(rows)
}

func (s *OutboxService) handleOne(ctx context.Context, m *model.LocalMessage) {
	// New trace per outbox row. We can't link back to the original /gifts/send
	// trace (it's already done by the time we get here), but we tag with the
	// order id so cross-querying by attribute works.
	ctx, span := obs.Tracer("gift-service/outbox").Start(ctx, "outbox.publish")
	span.SetAttributes(
		attribute.String("biz.id", m.BizID),
		attribute.String("topic", m.Topic),
		attribute.Int("retries", m.Retries),
	)
	defer span.End()

	if err := s.prod.Publish(ctx, m); err != nil {
		span.RecordError(err)
		backoff := s.cfg.BaseBackoff << min(m.Retries, 6) // cap exponent
		obs.OutboxFailed.WithLabelValues(m.Topic).Inc()
		obs.KafkaProducedTotal.WithLabelValues("gift-service", m.Topic, "error").Inc()
		if err := s.repo.Reschedule(ctx, m.ID, m.Retries, s.cfg.MaxRetries, backoff); err != nil {
			logger.L().Warn("outbox reschedule", zap.Error(err))
		}
		return
	}
	obs.OutboxPublished.WithLabelValues(m.Topic).Inc()
	obs.KafkaProducedTotal.WithLabelValues("gift-service", m.Topic, "ok").Inc()
	if err := s.repo.MarkSent(ctx, m.ID); err != nil {
		logger.L().Warn("outbox mark sent", zap.Error(err))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DrainOnce is the test-visible entrypoint.
func (s *OutboxService) DrainOnce(ctx context.Context) int { return s.drainOnce(ctx) }
