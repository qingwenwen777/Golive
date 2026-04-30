package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

type OutboxRepo struct{ db *gorm.DB }

func NewOutboxRepo(db *gorm.DB) *OutboxRepo { return &OutboxRepo{db: db} }

// Claim selects up-to `limit` pending rows whose NextAt is in the past, then
// flips their status to 'sent' optimistically. We use a SELECT then UPDATE
// guarded by version (status='pending') to avoid two workers grabbing the
// same row. The actual Kafka send happens after this — failures roll the
// status back to pending with incremented retry counter.
func (r *OutboxRepo) Claim(ctx context.Context, limit int) ([]model.LocalMessage, error) {
	var rows []model.LocalMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = ? AND next_at <= ?", model.OutboxStatusPending, time.Now()).
			Order("id ASC").
			Limit(limit).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint64, len(rows))
		for i, m := range rows {
			ids[i] = m.ID
		}
		// Mark "in-flight" by leaving status=pending but bumping next_at far
		// into the future so other workers skip it. We'll either flip to
		// 'sent' after a successful publish or reset next_at on failure.
		hold := time.Now().Add(30 * time.Second)
		return tx.Model(&model.LocalMessage{}).
			Where("id IN ? AND status = ?", ids, model.OutboxStatusPending).
			Update("next_at", hold).Error
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// MarkSent flips a message to terminal 'sent'.
func (r *OutboxRepo) MarkSent(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.LocalMessage{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": model.OutboxStatusSent, "next_at": time.Now()}).Error
}

// Reschedule increments retry count and pushes next_at out by backoff. If
// retries exceeds maxRetries, the message is marked dead instead.
func (r *OutboxRepo) Reschedule(ctx context.Context, id uint64, retries, maxRetries int, backoff time.Duration) error {
	updates := map[string]any{
		"retries": retries + 1,
		"next_at": time.Now().Add(backoff),
	}
	if retries+1 >= maxRetries {
		updates["status"] = model.OutboxStatusDead
	}
	return r.db.WithContext(ctx).Model(&model.LocalMessage{}).
		Where("id = ?", id).Updates(updates).Error
}

// CountByStatus is exposed for /metrics / debug.
func (r *OutboxRepo) CountByStatus(ctx context.Context, status string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.LocalMessage{}).
		Where("status = ?", status).Count(&n).Error
	return n, err
}
