package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

func shuffleEntries(entries []model.LuckyBagEntry) {
	rand.Shuffle(len(entries), func(i, j int) {
		entries[i], entries[j] = entries[j], entries[i]
	})
}

var (
	ErrActiveLuckyBagExists = errors.New("active lucky bag exists")
	ErrLuckyBagNotFound     = errors.New("lucky bag not found")
	ErrLuckyBagClosed       = errors.New("lucky bag closed")
	ErrLuckyBagAlreadyJoin  = errors.New("lucky bag already joined")
	ErrLuckyBagUnauthorized = errors.New("lucky bag unauthorized")
)

// LuckyBagWinner is the per-winner result produced by a draw.
type LuckyBagWinner struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	Payout int64  `json:"payout"`
}

// LuckyBagWinners returns the winners of a drawn bag with their display name
// and avatar, newest payout first. Used to render the winner-list popup.
func (r *OrderRepo) LuckyBagWinners(ctx context.Context, bagID string) ([]LuckyBagWinner, error) {
	var winners []LuckyBagWinner
	err := r.db.WithContext(ctx).
		Table("lucky_bag_entries AS e").
		Select(`
e.user_id AS user_id,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), e.user_id) AS name,
COALESCE(u.avatar, '') AS avatar,
e.payout AS payout
`).
		Joins("LEFT JOIN users AS u ON u.id = e.user_id").
		Where("e.bag_id = ? AND e.status = ?", bagID, model.LuckyBagEntryWon).
		Order("e.payout DESC, e.created_at ASC").
		Scan(&winners).Error
	if err != nil {
		return nil, err
	}
	return winners, nil
}

// LatestLuckyBag returns the most recent bag in a room plus the caller's entry
// and the joined-participant count.
func (r *OrderRepo) LatestLuckyBag(ctx context.Context, roomID, userID string) (*model.LuckyBag, *model.LuckyBagEntry, int64, error) {
	var bag model.LuckyBag
	err := r.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		Take(&bag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, 0, nil
	}
	if err != nil {
		return nil, nil, 0, err
	}
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.LuckyBagEntry{}).
		Where("bag_id = ?", bag.ID).
		Count(&count).Error; err != nil {
		return nil, nil, 0, err
	}
	var entry *model.LuckyBagEntry
	if strings.TrimSpace(userID) != "" {
		var row model.LuckyBagEntry
		err = r.db.WithContext(ctx).
			Where("bag_id = ? AND user_id = ?", bag.ID, userID).
			Take(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, 0, err
		}
		if err == nil {
			entry = &row
		}
	}
	return &bag, entry, count, nil
}

func (r *OrderRepo) GetLuckyBag(ctx context.Context, bagID string) (*model.LuckyBag, error) {
	var bag model.LuckyBag
	err := r.db.WithContext(ctx).Where("id = ?", bagID).Take(&bag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrLuckyBagNotFound
	}
	if err != nil {
		return nil, err
	}
	return &bag, nil
}

// RoomChannelID returns the channel id for a room (used for follow checks).
func (r *OrderRepo) RoomChannelID(ctx context.Context, roomID string) (string, error) {
	var channelID string
	err := r.db.WithContext(ctx).Raw(`
SELECT COALESCE(NULLIF(channel_id, ''), '')
FROM rooms
WHERE id = ?
`, roomID).Row().Scan(&channelID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(channelID), nil
}

// FanBadgeLevelFor returns the caller's fan badge level for a creator and
// whether a badge exists.
func (r *OrderRepo) FanBadgeLevelFor(ctx context.Context, userID, creatorID string) (int, bool, error) {
	var badge model.FanBadge
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Take(&badge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if isMissingTable(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return FanBadgeLevel(badge.TotalContribution), true, nil
}

// CreateLuckyBag escrows the owner's balance and persists the bag + outbox.
func (r *OrderRepo) CreateLuckyBag(ctx context.Context, bag *model.LuckyBag, outboxPayload []byte) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var active int64
		if err := tx.Model(&model.LuckyBag{}).
			Where("room_id = ? AND status = ?", bag.RoomID, model.LuckyBagOpen).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrActiveLuckyBagExists
		}
		if err := debitUserBalance(tx, bag.OwnerID, bag.TotalCoin); err != nil {
			return err
		}
		if err := tx.Create(bag).Error; err != nil {
			return err
		}
		if err := createCoinTransaction(
			tx,
			bag.OwnerID,
			-bag.TotalCoin,
			model.CoinTxLuckyBagSend,
			"福袋发放",
			luckyBagDesc(bag),
			"lucky_bag",
			bag.ID,
			bag.RoomID,
			"",
		); err != nil {
			return err
		}
		return tx.Create(&model.LocalMessage{
			BizID:   bag.ID,
			RoomID:  bag.RoomID,
			Topic:   model.OutboxTopicLuckyBag,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error
	})
}

// JoinLuckyBag records a participation entry (idempotent per user).
func (r *OrderRepo) JoinLuckyBag(ctx context.Context, entry *model.LuckyBagEntry, outboxPayload []byte) (*model.LuckyBagEntry, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var bag model.LuckyBag
		if err := tx.Where("id = ?", entry.BagID).Take(&bag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLuckyBagNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if bag.Status != model.LuckyBagOpen || !now.Before(bag.CloseAt) {
			return ErrLuckyBagClosed
		}
		entry.RoomID = bag.RoomID
		if err := tx.Create(entry).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrLuckyBagAlreadyJoin
			}
			return err
		}
		return tx.Create(&model.LocalMessage{
			BizID:   entry.ID,
			RoomID:  entry.RoomID,
			Topic:   model.OutboxTopicLuckyBag,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error
	})
	if errors.Is(err, ErrLuckyBagAlreadyJoin) {
		var existing model.LuckyBagEntry
		ferr := r.db.WithContext(ctx).
			Where("bag_id = ? AND user_id = ?", entry.BagID, entry.UserID).
			Take(&existing).Error
		if ferr != nil {
			return nil, ferr
		}
		return &existing, err
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// DueLuckyBags returns open bags whose countdown has elapsed.
func (r *OrderRepo) DueLuckyBags(ctx context.Context, limit int) ([]model.LuckyBag, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var bags []model.LuckyBag
	err := r.db.WithContext(ctx).
		Where("status = ? AND close_at <= ?", model.LuckyBagOpen, time.Now().UTC()).
		Order("close_at ASC").
		Limit(limit).
		Find(&bags).Error
	return bags, err
}

// DrawLuckyBag settles a bag: pays winners their packet, marks the rest
// missed, refunds the unused remainder to the owner, and emits an outbox
// event. `packets` has len == bag.Count; only the first len(winners) are paid.
// All joined participants are eligible (the room-presence requirement was
// removed so any joiner can win).
func (r *OrderRepo) DrawLuckyBag(ctx context.Context, bagID string, packets []int64, ts int64) (*model.LuckyBag, []LuckyBagWinner, error) {
	var drawnBag model.LuckyBag
	var winners []LuckyBagWinner
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var bag model.LuckyBag
		if err := tx.Where("id = ?", bagID).Take(&bag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLuckyBagNotFound
			}
			return err
		}
		if bag.Status != model.LuckyBagOpen {
			return ErrLuckyBagClosed
		}
		// Claim the draw: guard against a concurrent scheduler tick.
		res := tx.Model(&model.LuckyBag{}).
			Where("id = ? AND status = ?", bagID, model.LuckyBagOpen).
			Updates(map[string]any{"status": model.LuckyBagDrawn, "drawn_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrLuckyBagClosed
		}

		var entries []model.LuckyBagEntry
		if err := tx.Where("bag_id = ? AND status = ?", bagID, model.LuckyBagEntryJoined).
			Order("created_at ASC, id ASC").
			Find(&entries).Error; err != nil {
			return err
		}
		eligible := append([]model.LuckyBagEntry(nil), entries...)
		shuffleEntries(eligible)

		winnerCount := len(packets)
		if len(eligible) < winnerCount {
			winnerCount = len(eligible)
		}
		var paid int64
		for i := 0; i < winnerCount; i++ {
			e := eligible[i]
			payout := packets[i]
			paid += payout
			if res := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", payout, e.UserID); res.Error != nil {
				return res.Error
			}
			if err := createCoinTransaction(
				tx,
				e.UserID,
				payout,
				model.CoinTxLuckyBagPayout,
				"福袋中奖",
				luckyBagDesc(&bag),
				"lucky_bag",
				bag.ID,
				bag.RoomID,
				bag.OwnerID,
			); err != nil {
				return err
			}
			if err := tx.Model(&model.LuckyBagEntry{}).Where("id = ?", e.ID).
				Updates(map[string]any{"status": model.LuckyBagEntryWon, "payout": payout}).Error; err != nil {
				return err
			}
			winners = append(winners, LuckyBagWinner{UserID: e.UserID, Payout: payout})
		}
		// Everyone who joined but did not win is marked missed.
		if err := tx.Model(&model.LuckyBagEntry{}).
			Where("bag_id = ? AND status = ?", bagID, model.LuckyBagEntryJoined).
			Update("status", model.LuckyBagEntryMissed).Error; err != nil {
			return err
		}
		// Refund the unused remainder to the owner.
		refund := bag.TotalCoin - paid
		if refund > 0 {
			if res := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", refund, bag.OwnerID); res.Error != nil {
				return res.Error
			}
			if err := createCoinTransaction(
				tx,
				bag.OwnerID,
				refund,
				model.CoinTxLuckyBagRefund,
				"福袋退还",
				luckyBagDesc(&bag),
				"lucky_bag",
				bag.ID,
				bag.RoomID,
				"",
			); err != nil {
				return err
			}
		}

		bag.Status = model.LuckyBagDrawn
		bag.DrawnAt = &now
		payload, err := MarshalLuckyBagOutbox("drawn", &bag, int64(len(entries)), len(winners), ts)
		if err != nil {
			return err
		}
		if err := tx.Create(&model.LocalMessage{
			BizID:   bag.ID,
			RoomID:  bag.RoomID,
			Topic:   model.OutboxTopicLuckyBag,
			Payload: string(payload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error; err != nil {
			return err
		}
		drawnBag = bag
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &drawnBag, winners, nil
}

// CancelLuckyBag refunds the full escrow to the owner before any draw.
func (r *OrderRepo) CancelLuckyBag(ctx context.Context, bagID, ownerID string, outboxPayload []byte) (*model.LuckyBag, error) {
	var cancelled model.LuckyBag
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var bag model.LuckyBag
		if err := tx.Where("id = ?", bagID).Take(&bag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLuckyBagNotFound
			}
			return err
		}
		if bag.OwnerID != ownerID {
			return ErrLuckyBagUnauthorized
		}
		if bag.Status != model.LuckyBagOpen {
			return ErrLuckyBagClosed
		}
		res := tx.Model(&model.LuckyBag{}).
			Where("id = ? AND status = ?", bagID, model.LuckyBagOpen).
			Updates(map[string]any{"status": model.LuckyBagCancelled, "drawn_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrLuckyBagClosed
		}
		if err := tx.Model(&model.LuckyBagEntry{}).
			Where("bag_id = ? AND status = ?", bagID, model.LuckyBagEntryJoined).
			Update("status", model.LuckyBagEntryMissed).Error; err != nil {
			return err
		}
		if res := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", bag.TotalCoin, bag.OwnerID); res.Error != nil {
			return res.Error
		}
		if err := createCoinTransaction(
			tx,
			bag.OwnerID,
			bag.TotalCoin,
			model.CoinTxLuckyBagRefund,
			"福袋退还",
			luckyBagDesc(&bag),
			"lucky_bag",
			bag.ID,
			bag.RoomID,
			"",
		); err != nil {
			return err
		}
		if err := tx.Create(&model.LocalMessage{
			BizID:   bag.ID,
			RoomID:  bag.RoomID,
			Topic:   model.OutboxTopicLuckyBag,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error; err != nil {
			return err
		}
		bag.Status = model.LuckyBagCancelled
		bag.DrawnAt = &now
		cancelled = bag
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cancelled, nil
}

func luckyBagDesc(bag *model.LuckyBag) string {
	if bag == nil {
		return ""
	}
	if strings.TrimSpace(bag.Message) != "" {
		return bag.Message
	}
	return fmt.Sprintf("%d coins x%d", bag.TotalCoin, bag.Count)
}

// MarshalLuckyBagOutbox builds the broadcast payload. Field names match
// im-gateway/useRoomRealtime expectations.
func MarshalLuckyBagOutbox(event string, bag *model.LuckyBag, participantCount int64, winnerCount int, ts int64) ([]byte, error) {
	if bag == nil {
		return nil, fmt.Errorf("bag nil")
	}
	payload := map[string]any{
		"type":             "lucky_bag",
		"event":            event,
		"bag":              bag,
		"participantCount": participantCount,
		"winnerCount":      winnerCount,
		"ts":               ts,
	}
	return json.Marshal(payload)
}
