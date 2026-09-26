package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/wallet"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *UserRepo) CreditStripeTopupIfNeeded(
	ctx context.Context,
	id string,
	amount int64,
	sessionID string,
	currency string,
) (*model.User, *model.CoinTransaction, bool, error) {
	if amount <= 0 || strings.TrimSpace(sessionID) == "" {
		return nil, nil, false, errors.New("invalid stripe top-up")
	}
	var u model.User
	var coinTx model.CoinTransaction
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&u).Error; err != nil {
			return err
		}
		err := tx.Where("user_id = ? AND type = ? AND source_type = ? AND source_id = ?", id, model.CoinTxTopup, "stripe_checkout", sessionID).
			Take(&coinTx).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row, err := wallet.Credit(tx, id, amount, wallet.Entry{
			Type:        model.CoinTxTopup,
			Title:       "Stripe top-up",
			Description: fmt.Sprintf("Paid through Stripe Checkout (%s)", strings.ToUpper(currency)),
			SourceType:  "stripe_checkout",
			SourceID:    sessionID,
		})
		if err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		coinTx = *row
		created = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, false, err
	}
	return &u, &coinTx, created, nil
}

type DailyWatchTaskStats struct {
	Rooms        int64
	WatchSeconds int64
}

func (r *UserRepo) DailyWatchTaskStats(ctx context.Context, userID, watchDate string) (DailyWatchTaskStats, error) {
	var row struct {
		Rooms        int64
		WatchSeconds int64
	}
	err := r.db.WithContext(ctx).
		Table("room_watch_events").
		Select("COUNT(1) AS rooms, COALESCE(SUM(daily_watch_count), 0) * ? AS watch_seconds", 30).
		Where("user_id = ? AND watch_date = ?", userID, watchDate).
		Scan(&row).Error
	if isMissingRelation(err) {
		return DailyWatchTaskStats{}, nil
	}
	return DailyWatchTaskStats{Rooms: row.Rooms, WatchSeconds: row.WatchSeconds}, err
}

func (r *UserRepo) ClaimDailyCoinReward(
	ctx context.Context,
	id string,
	taskSourceID string,
	title string,
	description string,
	reward int64,
) (*model.User, *model.CoinTransaction, bool, error) {
	var u model.User
	var coinTx model.CoinTransaction
	created := false
	txID := dailyTaskTxID(id, taskSourceID)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the user row first so concurrent claims serialize here and the
		// check below sees any claim committed by the previous holder.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&u).Error; err != nil {
			return err
		}
		err := tx.Where("user_id = ? AND type = ? AND source_id = ?", id, model.CoinTxDailyTask, taskSourceID).
			Take(&coinTx).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row, err := wallet.Credit(tx, id, reward, wallet.Entry{
			ID:          txID,
			Type:        model.CoinTxDailyTask,
			Title:       title,
			Description: description,
			SourceType:  "daily_task",
			SourceID:    taskSourceID,
		})
		if err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Take(&u).Error; err != nil {
			return err
		}
		coinTx = *row
		created = true
		return nil
	})
	if isDuplicateKey(err) {
		// Another claim for this task and day won the primary key; the
		// rollback undid our credit, so report the existing claim.
		created = false
		u, coinTx = model.User{}, model.CoinTransaction{}
		err = r.db.WithContext(ctx).Where("id = ?", id).Take(&u).Error
		if err == nil {
			err = r.db.WithContext(ctx).Where("id = ?", txID).Take(&coinTx).Error
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, false, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, false, err
	}
	return &u, &coinTx, created, nil
}

// dailyTaskTxID derives the ledger row ID from the user and the task's daily
// source ID, so the primary key allows at most one daily-task row per user,
// task and day regardless of how claims interleave.
func dailyTaskTxID(userID, taskSourceID string) string {
	sum := sha256.Sum256([]byte(userID + "\x00" + taskSourceID))
	return "daily_" + hex.EncodeToString(sum[:24])
}

func (r *UserRepo) ListCoinTransactions(ctx context.Context, id string, limit int) ([]model.CoinTransaction, error) {
	if limit <= 0 {
		limit = 80
	}
	if limit > 200 {
		limit = 200
	}
	var rows []model.CoinTransaction
	err := r.db.WithContext(ctx).
		Where("user_id = ?", id).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *UserRepo) AdminAdjustCoins(ctx context.Context, userID, action string, amount int64, note, operatorID string) (*model.User, *model.CoinTransaction, error) {
	if amount <= 0 {
		return nil, nil, ErrInsufficientCoins
	}
	var u model.User
	var coinTx *model.CoinTransaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Take(&u).Error; err != nil {
			return err
		}
		entry := wallet.Entry{
			Type:           model.CoinTxAdminAdjust,
			Title:          "Admin coin adjustment",
			Description:    strings.TrimSpace(note),
			SourceType:     "admin",
			SourceID:       operatorID,
			CounterpartyID: operatorID,
		}
		var err error
		switch action {
		case "add":
			coinTx, err = wallet.Credit(tx, userID, amount, entry)
		case "deduct":
			// Operator corrections may take banned users' and frozen coins.
			coinTx, err = wallet.AdminDebit(tx, userID, amount, entry)
		case "freeze":
			entry.Type, entry.Title = model.CoinTxAdminFreeze, "Admin coin freeze"
			coinTx, err = wallet.Freeze(tx, userID, amount, entry)
		case "unfreeze":
			entry.Type, entry.Title = model.CoinTxAdminUnfreeze, "Admin coin unfreeze"
			coinTx, err = wallet.Unfreeze(tx, userID, amount, entry)
		default:
			return errors.New("invalid coin action")
		}
		if errors.Is(err, wallet.ErrInsufficientFunds) {
			return ErrInsufficientCoins
		}
		if err != nil {
			return err
		}
		return tx.Where("id = ?", userID).Take(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := r.hydrateUserLevel(ctx, &u); err != nil {
		return nil, nil, err
	}
	return &u, coinTx, nil
}
