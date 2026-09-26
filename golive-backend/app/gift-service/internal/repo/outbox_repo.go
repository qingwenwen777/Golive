package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

// ErrClaimRace means a claim's UPDATE did not hold every row its SELECT
// picked. The claim is rolled back, so nothing is handed out twice.
var ErrClaimRace = errors.New("outbox claim raced another worker")

type OutboxRepo struct{ db *gorm.DB }

func NewOutboxRepo(db *gorm.DB) *OutboxRepo { return &OutboxRepo{db: db} }

// Claim selects up-to `limit` pending rows whose NextAt is in the past and
// holds them for `hold`: status stays 'pending' but next_at moves into the
// future so other workers skip them. The caller flips a row to 'sent' after a
// successful publish or reschedules it on failure; a crashed worker's rows
// become claimable again once the hold lapses.
//
// The SELECT is FOR UPDATE SKIP LOCKED (MySQL 8.0+, MariaDB 10.6+; SQLite
// drops the clause and serialises writers instead), so a concurrent Claim
// skips the rows this one is claiming instead of reading them from its own
// snapshot and returning them too.
func (r *OutboxRepo) Claim(ctx context.Context, limit int, hold time.Duration) ([]model.LocalMessage, error) {
	var rows []model.LocalMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_at <= ?", model.OutboxStatusPending, now).
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
		res := tx.Model(&model.LocalMessage{}).
			Where("id IN ? AND status = ? AND next_at <= ?", ids, model.OutboxStatusPending, now).
			Update("next_at", now.Add(hold))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != int64(len(ids)) {
			return ErrClaimRace
		}
		return nil
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
