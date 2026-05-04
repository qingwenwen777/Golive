// Package repo houses persistence for the gift domain.
//
// The hot path uses a single transaction containing:
//
//  1. UPDATE users SET coin_balance = coin_balance - amount
//     WHERE id = ? AND coin_balance >= amount    -- atomic balance check
//  2. INSERT INTO {gift_orders | super_chat_orders}     -- success ledger
//  3. INSERT INTO local_messages (status='pending')      -- outbox
//
// If (1) reports zero rows affected → balance was insufficient and we roll
// back. If (2) hits a UNIQUE(request_id) violation → another request for the
// same requestId beat us; we roll back and fetch the existing order so the
// caller gets a "replay" outcome — this is the DB-level idempotency safety
// net behind the Redis cache.
package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/userlevel"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrDuplicateRequest  = errors.New("duplicate request")
	ErrRoomOwnerNotFound = errors.New("room owner not found")
	ErrActiveBetExists   = errors.New("active bet round exists")
	ErrBetRoundNotFound  = errors.New("bet round not found")
	ErrBetClosed         = errors.New("bet closed")
	ErrBetAlreadyPlaced  = errors.New("bet already placed")
	ErrBetUnauthorized   = errors.New("bet unauthorized")
	ErrBetNoWinners      = errors.New("bet has no winners")
)

type OrderRepo struct{ db *gorm.DB }

func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

func (r *OrderRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.GiftOrder{},
		&model.SuperChatOrder{},
		&model.BetRound{},
		&model.BetWager{},
		&model.LocalMessage{},
		&model.FanBadge{},
		&model.CoinTransaction{},
	)
}

type FanBadgeContributionMode int

const (
	FanBadgeNoChange FanBadgeContributionMode = iota
	FanBadgeIfExists
	FanBadgeCreate
)

func (r *OrderRepo) DisplayNameForUser(ctx context.Context, userID string) (string, error) {
	var name string
	err := r.db.WithContext(ctx).Raw(`
SELECT COALESCE(NULLIF(display_name, ''), NULLIF(username, ''), '')
FROM users
WHERE id = ?
`, userID).Row().Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}

// AvatarForUser returns the user's avatar URL, or "" if unknown. Used so the
// SuperChat broadcast payload carries the sender's avatar — without it, other
// viewers fall back to the default avatar in the SC card.
func (r *OrderRepo) AvatarForUser(ctx context.Context, userID string) (string, error) {
	var avatar string
	err := r.db.WithContext(ctx).Raw(`
SELECT COALESCE(avatar, '')
FROM users
WHERE id = ?
`, userID).Row().Scan(&avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(avatar), nil
}

func createCoinTransaction(
	tx *gorm.DB,
	userID string,
	amount int64,
	txType string,
	title string,
	description string,
	sourceType string,
	sourceID string,
	roomID string,
	counterpartyID string,
) error {
	if userID == "" || amount == 0 {
		return nil
	}
	var balanceAfter int64
	if err := tx.Raw("SELECT coin_balance FROM users WHERE id = ?", userID).Row().Scan(&balanceAfter); err != nil {
		return err
	}
	return tx.Create(&model.CoinTransaction{
		ID:             uuid.NewString(),
		UserID:         userID,
		Type:           txType,
		Amount:         amount,
		BalanceAfter:   balanceAfter,
		Title:          title,
		Description:    description,
		SourceType:     sourceType,
		SourceID:       sourceID,
		RoomID:         roomID,
		CounterpartyID: counterpartyID,
	}).Error
}

func FanBadgeLevel(totalContribution int64) int {
	if totalContribution <= 0 {
		return 1
	}
	level := 1
	threshold := int64(10)
	for level < 99 && totalContribution >= threshold {
		level++
		threshold += int64(level) * 10
	}
	return level
}

func (r *OrderRepo) ListFanBadges(ctx context.Context, userID string) ([]model.FanBadge, error) {
	var badges []model.FanBadge
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("level DESC, total_contribution DESC, updated_at DESC").
		Find(&badges).Error
	if err != nil {
		return nil, err
	}
	for i := range badges {
		badges[i].Level = FanBadgeLevel(badges[i].TotalContribution)
	}
	sort.SliceStable(badges, func(i, j int) bool {
		if badges[i].Level != badges[j].Level {
			return badges[i].Level > badges[j].Level
		}
		if badges[i].TotalContribution != badges[j].TotalContribution {
			return badges[i].TotalContribution > badges[j].TotalContribution
		}
		return badges[i].UpdatedAt.After(badges[j].UpdatedAt)
	})
	return badges, nil
}

func (r *OrderRepo) UserLevel(ctx context.Context, userID string) (int, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("user_id = ? AND type = ? AND amount > 0", userID, model.CoinTxTopup).
		Row().
		Scan(&total)
	if err != nil {
		return 1, err
	}
	return userlevel.LevelForTotalTopup(total), nil
}

type BetOptionSummary struct {
	Option string `json:"option"`
	Count  int64  `json:"count"`
	Total  int64  `json:"total"`
}

func (r *OrderRepo) RoomOwner(ctx context.Context, roomID string) (string, error) {
	return roomOwnerID(r.db.WithContext(ctx), roomID)
}

// GetBetRound loads a bet round by id. Returns ErrBetRoundNotFound if absent.
func (r *OrderRepo) GetBetRound(ctx context.Context, roundID string) (*model.BetRound, error) {
	var round model.BetRound
	err := r.db.WithContext(ctx).Where("id = ?", roundID).Take(&round).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrBetRoundNotFound
	}
	if err != nil {
		return nil, err
	}
	return &round, nil
}

func (r *OrderRepo) LatestBetRound(ctx context.Context, roomID, userID string) (*model.BetRound, []BetOptionSummary, *model.BetWager, error) {
	var round model.BetRound
	err := r.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		Take(&round).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if err := r.closeExpiredBetRound(ctx, &round); err != nil {
		return nil, nil, nil, err
	}
	summaries, err := r.betSummaries(ctx, round.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	var wager *model.BetWager
	if strings.TrimSpace(userID) != "" {
		var row model.BetWager
		err = r.db.WithContext(ctx).
			Where("round_id = ? AND user_id = ?", round.ID, userID).
			Take(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, err
		}
		if err == nil {
			wager = &row
		}
	}
	return &round, summaries, wager, nil
}

func (r *OrderRepo) CreateBetRound(ctx context.Context, round *model.BetRound, outboxPayload []byte) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&model.BetRound{}).
			Where("room_id = ? AND status = ? AND close_at <= ?", round.RoomID, model.BetRoundOpen, now).
			Update("status", model.BetRoundClosed).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&model.BetRound{}).
			Where("room_id = ? AND status IN ?", round.RoomID, []string{model.BetRoundOpen, model.BetRoundClosed}).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrActiveBetExists
		}
		if err := tx.Create(round).Error; err != nil {
			return err
		}
		return tx.Create(&model.LocalMessage{
			BizID:   round.ID,
			RoomID:  round.RoomID,
			Topic:   model.OutboxTopicBet,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error
	})
}

func (r *OrderRepo) PlaceBetWager(ctx context.Context, wager *model.BetWager, outboxPayload []byte) (*model.BetWager, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var round model.BetRound
		if err := tx.Where("id = ?", wager.RoundID).Take(&round).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if round.Status != model.BetRoundOpen || !now.Before(round.CloseAt) {
			if round.Status == model.BetRoundOpen && !now.Before(round.CloseAt) {
				if err := tx.Model(&model.BetRound{}).
					Where("id = ? AND status = ?", round.ID, model.BetRoundOpen).
					Update("status", model.BetRoundClosed).Error; err != nil {
					return err
				}
			}
			return ErrBetClosed
		}
		wager.RoomID = round.RoomID
		wager.Amount = round.Amount
		res := tx.Exec(
			"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND coin_balance >= ?",
			wager.Amount, wager.UserID, wager.Amount,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInsufficientFunds
		}
		if err := tx.Create(wager).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrBetAlreadyPlaced
			}
			return err
		}
		if err := createCoinTransaction(
			tx,
			wager.UserID,
			-wager.Amount,
			model.CoinTxBetWager,
			"竞猜消费",
			round.Question,
			"bet_wager",
			wager.ID,
			wager.RoomID,
			round.OwnerID,
		); err != nil {
			return err
		}
		return tx.Create(&model.LocalMessage{
			BizID:   wager.ID,
			RoomID:  wager.RoomID,
			Topic:   model.OutboxTopicBet,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error
	})
	if errors.Is(err, ErrBetAlreadyPlaced) {
		var existing model.BetWager
		ferr := r.db.WithContext(ctx).
			Where("round_id = ? AND user_id = ?", wager.RoundID, wager.UserID).
			Take(&existing).Error
		if ferr != nil {
			return nil, ferr
		}
		return &existing, err
	}
	if err != nil {
		return nil, err
	}
	return wager, nil
}

func (r *OrderRepo) SettleBetRound(ctx context.Context, roundID, ownerID, winningOption string, outboxPayload []byte) (*model.BetRound, []model.BetWager, error) {
	var settledRound model.BetRound
	var settledWagers []model.BetWager
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var round model.BetRound
		if err := tx.Where("id = ?", roundID).Take(&round).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		if round.OwnerID != ownerID {
			return ErrBetUnauthorized
		}
		if round.Status != model.BetRoundOpen && round.Status != model.BetRoundClosed {
			return ErrBetClosed
		}
		var wagers []model.BetWager
		if err := tx.Where("round_id = ? AND status = ?", roundID, model.StatusLocked).
			Order("created_at ASC, id ASC").
			Find(&wagers).Error; err != nil {
			return err
		}

		var winnerPool, loserPool int64
		for _, wager := range wagers {
			if wager.Option == winningOption {
				winnerPool += wager.Amount
			} else {
				loserPool += wager.Amount
			}
		}
		if winnerPool <= 0 {
			return ErrBetNoWinners
		}
		res := tx.Model(&model.BetRound{}).Where("id = ? AND status IN ?", roundID, []string{model.BetRoundOpen, model.BetRoundClosed}).
			Updates(map[string]any{
				"status":         model.BetRoundSettled,
				"winning_option": winningOption,
				"settled_at":     now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBetClosed
		}

		payouts := make([]int64, len(wagers))
		var paidBonus int64
		firstWinner := -1
		for i := range wagers {
			if wagers[i].Option != winningOption {
				continue
			}
			if firstWinner == -1 {
				firstWinner = i
			}
			bonus := loserPool * wagers[i].Amount / winnerPool
			paidBonus += bonus
			payouts[i] = wagers[i].Amount + bonus
		}
		remainder := loserPool - paidBonus
		if remainder > 0 && firstWinner >= 0 {
			payouts[firstWinner] += remainder
		}

		for i := range wagers {
			w := &wagers[i]
			updates := map[string]any{"status": model.StatusLost, "payout": int64(0)}
			if w.Option == winningOption {
				payout := payouts[i]
				updates = map[string]any{"status": model.StatusWon, "payout": payout}
				res := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", payout, w.UserID)
				if res.Error != nil {
					return res.Error
				}
				if err := createCoinTransaction(
					tx,
					w.UserID,
					payout,
					model.CoinTxBetPayout,
					"竞猜获得",
					round.Question,
					"bet_wager",
					w.ID,
					w.RoomID,
					round.OwnerID,
				); err != nil {
					return err
				}
			}
			if err := tx.Model(&model.BetWager{}).Where("id = ?", w.ID).Updates(updates).Error; err != nil {
				return err
			}
			w.Status = updates["status"].(string)
			w.Payout = updates["payout"].(int64)
		}

		if err := tx.Create(&model.LocalMessage{
			BizID:   round.ID,
			RoomID:  round.RoomID,
			Topic:   model.OutboxTopicBet,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error; err != nil {
			return err
		}
		round.Status = model.BetRoundSettled
		round.WinningOption = winningOption
		round.SettledAt = &now
		settledRound = round
		settledWagers = wagers
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &settledRound, settledWagers, nil
}

func (r *OrderRepo) CancelBetRound(ctx context.Context, roundID, ownerID string, outboxPayload []byte) (*model.BetRound, []model.BetWager, error) {
	var cancelledRound model.BetRound
	var refunded []model.BetWager
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var round model.BetRound
		if err := tx.Where("id = ?", roundID).Take(&round).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		if round.OwnerID != ownerID {
			return ErrBetUnauthorized
		}
		if round.Status != model.BetRoundOpen && round.Status != model.BetRoundClosed {
			return ErrBetClosed
		}
		res := tx.Model(&model.BetRound{}).Where("id = ? AND status IN ?", roundID, []string{model.BetRoundOpen, model.BetRoundClosed}).
			Updates(map[string]any{"status": model.BetRoundCancelled, "settled_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBetClosed
		}
		if err := tx.Where("round_id = ? AND status = ?", roundID, model.StatusLocked).
			Find(&refunded).Error; err != nil {
			return err
		}
		for i := range refunded {
			w := &refunded[i]
			if err := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", w.Amount, w.UserID).Error; err != nil {
				return err
			}
			if err := createCoinTransaction(
				tx,
				w.UserID,
				w.Amount,
				model.CoinTxBetRefund,
				"竞猜返还",
				round.Question,
				"bet_wager",
				w.ID,
				w.RoomID,
				round.OwnerID,
			); err != nil {
				return err
			}
			if err := tx.Model(&model.BetWager{}).Where("id = ?", w.ID).
				Updates(map[string]any{"status": model.StatusRefunded, "payout": w.Amount}).Error; err != nil {
				return err
			}
			w.Status = model.StatusRefunded
			w.Payout = w.Amount
		}
		if err := tx.Create(&model.LocalMessage{
			BizID:   round.ID,
			RoomID:  round.RoomID,
			Topic:   model.OutboxTopicBet,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  now,
		}).Error; err != nil {
			return err
		}
		round.Status = model.BetRoundCancelled
		round.SettledAt = &now
		cancelledRound = round
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &cancelledRound, refunded, nil
}

func (r *OrderRepo) closeExpiredBetRound(ctx context.Context, round *model.BetRound) error {
	if round == nil || round.Status != model.BetRoundOpen || time.Now().UTC().Before(round.CloseAt) {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.BetRound{}).
		Where("id = ? AND status = ?", round.ID, model.BetRoundOpen).
		Update("status", model.BetRoundClosed).Error; err != nil {
		return err
	}
	round.Status = model.BetRoundClosed
	return nil
}

func (r *OrderRepo) betSummaries(ctx context.Context, roundID string) ([]BetOptionSummary, error) {
	rows, err := r.db.WithContext(ctx).
		Model(&model.BetWager{}).
		Select("`option`, COUNT(*) AS `count`, COALESCE(SUM(amount), 0) AS total").
		Where("round_id = ?", roundID).
		Group("`option`").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []BetOptionSummary{
		{Option: model.BetOptionWin},
		{Option: model.BetOptionLose},
	}
	byOption := map[string]*BetOptionSummary{
		model.BetOptionWin:  &summaries[0],
		model.BetOptionLose: &summaries[1],
	}
	for rows.Next() {
		var row BetOptionSummary
		if err := rows.Scan(&row.Option, &row.Count, &row.Total); err != nil {
			return nil, err
		}
		if target := byOption[row.Option]; target != nil {
			*target = row
		}
	}
	return summaries, rows.Err()
}

// FindGiftByRequestID returns the existing order for a requestId, or nil.
func (r *OrderRepo) FindGiftByRequestID(ctx context.Context, userID, reqID string) (*model.GiftOrder, error) {
	var o model.GiftOrder
	err := r.db.WithContext(ctx).Where("request_id = ? AND user_id = ?", reqID, userID).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// FindSuperChatByRequestID returns the existing super-chat order, or nil.
func (r *OrderRepo) FindSuperChatByRequestID(ctx context.Context, userID, reqID string) (*model.SuperChatOrder, error) {
	var o model.SuperChatOrder
	err := r.db.WithContext(ctx).Where("request_id = ? AND user_id = ?", reqID, userID).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// PlaceGiftOrder is the canonical write path. Returns (order, replayed, err).
//
// replayed=true means another request with the same requestId got there
// first; we returned that existing order. err=ErrInsufficientFunds means we
// also persisted a `failed` order so future retries with the same requestId
// will replay the same failure. Other errors are rollbacks.
func (r *OrderRepo) PlaceGiftOrder(
	ctx context.Context,
	o *model.GiftOrder,
	outboxPayload []byte,
	fanBadgeMode FanBadgeContributionMode,
) (placed *model.GiftOrder, replayed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		receiverID, err := roomOwnerID(tx, o.RoomID)
		if err != nil {
			return err
		}

		// Decrement balance only if sufficient.
		res := tx.Exec(
			"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND coin_balance >= ?",
			o.TotalCoin, o.UserID, o.TotalCoin,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInsufficientFunds
		}
		if err := tx.Create(o).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateRequest
			}
			return err
		}
		if err := createCoinTransaction(
			tx,
			o.UserID,
			-o.TotalCoin,
			model.CoinTxGiftSpend,
			"礼物消费",
			fmt.Sprintf("%s x%d", o.GiftID, o.Count),
			"gift_order",
			o.OrderID,
			o.RoomID,
			receiverID,
		); err != nil {
			return err
		}
		if receiverID != "" {
			res = tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", o.TotalCoin, receiverID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrRoomOwnerNotFound
			}
			if err := createCoinTransaction(
				tx,
				receiverID,
				o.TotalCoin,
				model.CoinTxCreatorGiftIncome,
				"直播礼物收入",
				fmt.Sprintf("%s x%d", o.GiftID, o.Count),
				"gift_order",
				o.OrderID,
				o.RoomID,
				o.UserID,
			); err != nil {
				return err
			}
		}
		if err := applyFanBadgeContribution(tx, o.UserID, o.RoomID, receiverID, o.TotalCoin, fanBadgeMode); err != nil {
			return err
		}
		msg := &model.LocalMessage{
			BizID:   o.OrderID,
			RoomID:  o.RoomID,
			Topic:   model.OutboxTopicGift,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  time.Now(),
		}
		return tx.Create(msg).Error
	})
	if errors.Is(err, ErrDuplicateRequest) {
		existing, ferr := r.FindGiftByRequestID(ctx, o.UserID, o.RequestID)
		if ferr != nil {
			return nil, false, ferr
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return o, false, nil
}

// PersistGiftFailure records a failed order WITHOUT touching balance or
// writing to the outbox. Idempotent: if a row for requestId already exists
// (whether failed or success), return that.
func (r *OrderRepo) PersistGiftFailure(ctx context.Context, o *model.GiftOrder) (*model.GiftOrder, error) {
	err := r.db.WithContext(ctx).Create(o).Error
	if err == nil {
		return o, nil
	}
	if !isDuplicateKey(err) {
		return nil, err
	}
	return r.FindGiftByRequestID(ctx, o.UserID, o.RequestID)
}

// PlaceSuperChatOrder mirrors PlaceGiftOrder for super chats.
func (r *OrderRepo) PlaceSuperChatOrder(
	ctx context.Context,
	o *model.SuperChatOrder,
	outboxPayload []byte,
	fanBadgeMode FanBadgeContributionMode,
) (placed *model.SuperChatOrder, replayed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		receiverID, err := roomOwnerID(tx, o.RoomID)
		if err != nil {
			return err
		}

		res := tx.Exec(
			"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND coin_balance >= ?",
			o.Amount, o.UserID, o.Amount,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInsufficientFunds
		}
		if err := tx.Create(o).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateRequest
			}
			return err
		}
		if err := createCoinTransaction(
			tx,
			o.UserID,
			-o.Amount,
			model.CoinTxSuperChatSpend,
			"SC 消费",
			o.Text,
			"super_chat_order",
			o.OrderID,
			o.RoomID,
			receiverID,
		); err != nil {
			return err
		}
		if receiverID != "" {
			res = tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", o.Amount, receiverID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrRoomOwnerNotFound
			}
			if err := createCoinTransaction(
				tx,
				receiverID,
				o.Amount,
				model.CoinTxCreatorSuperChatIncome,
				"直播 SC 收入",
				o.Text,
				"super_chat_order",
				o.OrderID,
				o.RoomID,
				o.UserID,
			); err != nil {
				return err
			}
		}
		if err := applyFanBadgeContribution(tx, o.UserID, o.RoomID, receiverID, o.Amount, fanBadgeMode); err != nil {
			return err
		}
		msg := &model.LocalMessage{
			BizID:   o.OrderID,
			RoomID:  o.RoomID,
			Topic:   model.OutboxTopicSuperChat,
			Payload: string(outboxPayload),
			Status:  model.OutboxStatusPending,
			NextAt:  time.Now(),
		}
		return tx.Create(msg).Error
	})
	if errors.Is(err, ErrDuplicateRequest) {
		existing, ferr := r.FindSuperChatByRequestID(ctx, o.UserID, o.RequestID)
		if ferr != nil {
			return nil, false, ferr
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return o, false, nil
}

func (r *OrderRepo) PersistSuperChatFailure(ctx context.Context, o *model.SuperChatOrder) (*model.SuperChatOrder, error) {
	err := r.db.WithContext(ctx).Create(o).Error
	if err == nil {
		return o, nil
	}
	if !isDuplicateKey(err) {
		return nil, err
	}
	return r.FindSuperChatByRequestID(ctx, o.UserID, o.RequestID)
}

// MarshalGiftOutbox / MarshalSuperChatOutbox build the payload that downstream
// (kafka consumer → Redis publish) will broadcast. Field names match
// im-gateway/internal/hub/messages.go.
func MarshalGiftOutbox(id, requestID, userID, username, avatar, giftName, giftIcon string, count, tier, userLevel int, totalCoin, ts int64) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type":      "gift",
		"id":        id,
		"requestId": requestID,
		"userId":    userID,
		"user":      username,
		"avatar":    avatar,
		"giftName":  giftName,
		"giftIcon":  giftIcon,
		"count":     count,
		"tier":      tier,
		"userLevel": userLevel,
		"totalCoin": totalCoin,
		"ts":        ts,
	})
}

func MarshalSuperChatOutbox(id, userID, user, avatar, amount string, tier, userLevel int, text string, ts int64) ([]byte, error) {
	payload := map[string]any{
		"type":      "super_chat",
		"id":        id,
		"userId":    userID,
		"user":      user,
		"amount":    amount,
		"tier":      tier,
		"userLevel": userLevel,
		"text":      text,
		"ts":        ts,
	}
	if avatar != "" {
		payload["avatar"] = avatar
	}
	return json.Marshal(payload)
}

func MarshalBetOutbox(event string, round *model.BetRound, summaries []BetOptionSummary, userID, option string, ts int64) ([]byte, error) {
	if round == nil {
		return nil, fmt.Errorf("round nil")
	}
	payload := map[string]any{
		"type":    "bet",
		"event":   event,
		"round":   round,
		"summary": summaries,
		"ts":      ts,
	}
	if userID != "" {
		payload["userId"] = userID
	}
	if option != "" {
		payload["option"] = option
	}
	return json.Marshal(payload)
}

func roomOwnerID(tx *gorm.DB, roomID string) (string, error) {
	var ownerID string
	err := tx.Raw(`
SELECT COALESCE(NULLIF(owner_id, ''), '')
FROM rooms
WHERE id = ?
`, roomID).Row().Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRoomOwnerNotFound
	}
	if err != nil {
		return "", err
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return "", ErrRoomOwnerNotFound
	}
	return ownerID, nil
}

func applyFanBadgeContribution(tx *gorm.DB, userID, roomID, creatorID string, coin int64, mode FanBadgeContributionMode) error {
	if mode == FanBadgeNoChange || coin <= 0 || userID == "" || creatorID == "" || userID == creatorID {
		return nil
	}

	var badge model.FanBadge
	err := tx.Where("user_id = ? AND creator_id = ?", userID, creatorID).Take(&badge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if mode != FanBadgeCreate {
			return nil
		}
		name, avatar, err := roomCreatorProfile(tx, roomID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(name) == "" {
			name = creatorID
		}
		badge = model.FanBadge{
			UserID:            userID,
			CreatorID:         creatorID,
			CreatorName:       strings.TrimSpace(name),
			CreatorAvatar:     strings.TrimSpace(avatar),
			TotalContribution: coin,
			Level:             FanBadgeLevel(coin),
		}
		return tx.Create(&badge).Error
	}
	if err != nil {
		return err
	}

	total := badge.TotalContribution + coin
	return tx.Model(&model.FanBadge{}).
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Updates(map[string]any{
			"total_contribution": total,
			"level":              FanBadgeLevel(total),
			"updated_at":         time.Now(),
		}).Error
}

func roomCreatorProfile(tx *gorm.DB, roomID string) (string, string, error) {
	var row struct {
		Name   string
		Avatar string
	}
	err := tx.Raw(`
SELECT
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), NULLIF(r.channel, ''), r.owner_id, '') AS name,
  COALESCE(NULLIF(u.avatar, ''), NULLIF(r.avatar, ''), '') AS avatar
FROM rooms r
LEFT JOIN users u ON u.id = r.owner_id
WHERE r.id = ?
`, roomID).Row().Scan(&row.Name, &row.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrRoomOwnerNotFound
	}
	if err != nil {
		return "", "", err
	}
	return row.Name, row.Avatar, nil
}

// isDuplicateKey detects MySQL 1062 / generic unique-violation across
// drivers without coupling us to a specific error type.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "Error 1062") ||
		strings.Contains(s, "Duplicate entry") ||
		strings.Contains(s, "UNIQUE constraint failed")
}
