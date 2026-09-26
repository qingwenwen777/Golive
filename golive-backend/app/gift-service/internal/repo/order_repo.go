// Package repo houses persistence for the gift domain.
//
// The hot path uses a single transaction containing:
//
//  1. wallet.Debit: UPDATE users SET coin_balance = coin_balance - amount
//     WHERE id = ? AND available balance >= amount and account is active,
//     plus its coin_transactions row (all balance writes go through pkg/wallet)
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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/userlevel"
	"github.com/qingwenwen777/golive/pkg/wallet"
)

var (
	ErrInsufficientFunds = wallet.ErrInsufficientFunds
	ErrDuplicateRequest  = errors.New("duplicate request")
	ErrRoomOwnerNotFound = errors.New("room owner not found")
	ErrActiveBetExists   = errors.New("active bet round exists")
	ErrBetRoundNotFound  = errors.New("bet round not found")
	ErrBetClosed         = errors.New("bet closed")
	ErrBetAlreadyPlaced  = errors.New("bet already placed")
	ErrBetUnauthorized   = errors.New("bet unauthorized")
	ErrBetNoWinners      = errors.New("bet has no winners")
	ErrBetOwnerWager     = errors.New("bet owner cannot wager")
	ErrBetStillOpen      = errors.New("bet still open")
)

type OrderRepo struct{ db *gorm.DB }

func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

func (r *OrderRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.GiftOrder{},
		&model.SuperChatOrder{},
		&model.BetRound{},
		&model.BetWager{},
		&model.LuckyBag{},
		&model.LuckyBagEntry{},
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

// UserProfile returns the display name (falling back to username then userID)
// and avatar in a single query. Used by mic-link to render the request queue
// and on-air roster.
func (r *OrderRepo) UserProfile(ctx context.Context, userID string) (string, string, error) {
	var row struct {
		Name   string
		Avatar string
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT
  COALESCE(NULLIF(display_name, ''), NULLIF(username, ''), id) AS name,
  COALESCE(avatar, '') AS avatar
FROM users
WHERE id = ?
`, userID).Row().Scan(&row.Name, &row.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return userID, "", nil
	}
	if err != nil {
		return "", "", err
	}
	name := strings.TrimSpace(row.Name)
	if name == "" {
		name = userID
	}
	return name, strings.TrimSpace(row.Avatar), nil
}

// creditRoomOwner pays a gift's or super chat's receiver. A missing receiver
// row is reported as ErrRoomOwnerNotFound.
func creditRoomOwner(tx *gorm.DB, receiverID string, amount int64, e wallet.Entry) error {
	_, err := wallet.Credit(tx, receiverID, amount, e)
	if errors.Is(err, wallet.ErrUserNotFound) {
		return ErrRoomOwnerNotFound
	}
	return err
}

func FanBadgeLevel(totalContribution int64) int {
	if totalContribution <= 0 {
		return 1
	}
	level := int(totalContribution / 1000)
	if level < 1 {
		return 1
	}
	if level > 99 {
		return 99
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
	if err := r.refreshFanBadgeCreatorProfiles(ctx, badges); err != nil {
		return nil, err
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

func (r *OrderRepo) ListFanClubMembers(ctx context.Context, creatorID string, limit int) ([]model.FanClubMember, int64, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return []model.FanClubMember{}, 0, nil
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	base := r.db.WithContext(ctx).
		Table("fan_badges AS fb").
		Joins("JOIN users AS u ON u.id = fb.user_id").
		Where("fb.creator_id = ? AND fb.user_id <> ?", creatorID, creatorID)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		if isMissingTable(err) {
			return []model.FanClubMember{}, 0, nil
		}
		return nil, 0, err
	}
	var members []model.FanClubMember
	err := base.Session(&gorm.Session{}).
		Select(`
fb.user_id,
COALESCE(u.username, '') AS username,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
fb.total_contribution,
fb.level,
fb.updated_at
`).
		Order("fb.total_contribution DESC, fb.updated_at DESC, fb.user_id ASC").
		Limit(limit).
		Scan(&members).Error
	if isMissingTable(err) {
		return []model.FanClubMember{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	for i := range members {
		members[i].Level = FanBadgeLevel(members[i].TotalContribution)
	}
	if members == nil {
		members = []model.FanClubMember{}
	}
	return members, total, nil
}

func (r *OrderRepo) refreshFanBadgeCreatorProfiles(ctx context.Context, badges []model.FanBadge) error {
	if len(badges) == 0 {
		return nil
	}
	ids := make([]string, 0, len(badges))
	seen := make(map[string]struct{}, len(badges))
	for _, badge := range badges {
		creatorID := strings.TrimSpace(badge.CreatorID)
		if creatorID == "" {
			continue
		}
		if _, ok := seen[creatorID]; ok {
			continue
		}
		seen[creatorID] = struct{}{}
		ids = append(ids, creatorID)
	}
	if len(ids) == 0 {
		return nil
	}
	var profiles []struct {
		ID     string
		Name   string
		Avatar string
	}
	err := r.db.WithContext(ctx).Raw(`
SELECT
  id,
  COALESCE(NULLIF(display_name, ''), NULLIF(username, ''), id) AS name,
  COALESCE(NULLIF(avatar, ''), '') AS avatar
FROM users
WHERE id IN ?
`, ids).Scan(&profiles).Error
	if isMissingTable(err) {
		return nil
	}
	if err != nil {
		return err
	}
	byID := make(map[string]struct {
		Name   string
		Avatar string
	}, len(profiles))
	for _, profile := range profiles {
		byID[profile.ID] = struct {
			Name   string
			Avatar string
		}{Name: strings.TrimSpace(profile.Name), Avatar: strings.TrimSpace(profile.Avatar)}
	}
	for i := range badges {
		if profile, ok := byID[badges[i].CreatorID]; ok {
			if profile.Name != "" {
				badges[i].CreatorName = profile.Name
			}
			badges[i].CreatorAvatar = profile.Avatar
		}
	}
	return nil
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
		// Lock the round until commit: settle/cancel lock it too, so they
		// either see this wager or this wager sees the terminal status. A
		// plain read let a wager commit after settle and stay locked forever.
		var round model.BetRound
		if err := lockBetRound(tx, wager.RoundID, &round); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		// The host decides the outcome, so they must not hold a stake in it.
		if wager.UserID == round.OwnerID {
			return ErrBetOwnerWager
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
		if _, err := wallet.Debit(tx, wager.UserID, wager.Amount, wallet.Entry{
			Type:           model.CoinTxBetWager,
			Title:          "竞猜消费",
			Description:    round.Question,
			SourceType:     "bet_wager",
			SourceID:       wager.ID,
			RoomID:         wager.RoomID,
			CounterpartyID: round.OwnerID,
		}); err != nil {
			return err
		}
		if err := tx.Create(wager).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrBetAlreadyPlaced
			}
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
	return r.settleBetRound(ctx, roundID, ownerID, winningOption, outboxPayload, true)
}

func (r *OrderRepo) AdminSettleBetRound(ctx context.Context, roundID, winningOption string, outboxPayload []byte) (*model.BetRound, []model.BetWager, error) {
	return r.settleBetRound(ctx, roundID, "", winningOption, outboxPayload, false)
}

func (r *OrderRepo) settleBetRound(ctx context.Context, roundID, ownerID, winningOption string, outboxPayload []byte, requireOwner bool) (*model.BetRound, []model.BetWager, error) {
	var settledRound model.BetRound
	var settledWagers []model.BetWager
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var round model.BetRound
		if err := lockBetRound(tx, roundID, &round); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		if requireOwner && round.OwnerID != ownerID {
			return ErrBetUnauthorized
		}
		if round.Status != model.BetRoundOpen && round.Status != model.BetRoundClosed {
			return ErrBetClosed
		}
		// The host may only pick the result once betting has closed, so they
		// cannot watch the pool and settle while viewers are still wagering.
		if requireOwner && round.Status == model.BetRoundOpen && now.Before(round.CloseAt) {
			return ErrBetStillOpen
		}
		var wagers []model.BetWager
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("round_id = ? AND status = ?", roundID, model.StatusLocked).
			Order("created_at ASC, id ASC").
			Find(&wagers).Error; err != nil {
			return err
		}

		var winnerPool, loserPool, winners int64
		for _, wager := range wagers {
			if wager.Option == winningOption {
				winnerPool += wager.Amount
				winners++
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
			// Every stake equals round.Amount (PlaceBetWager sets it), so the
			// proportional share loserPool*Amount/winnerPool is loserPool/winners,
			// without the product that could overflow int64.
			bonus := loserPool / winners
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
				if _, err := wallet.Credit(tx, w.UserID, payout, wallet.Entry{
					Type:           model.CoinTxBetPayout,
					Title:          "竞猜获得",
					Description:    round.Question,
					SourceType:     "bet_wager",
					SourceID:       w.ID,
					RoomID:         w.RoomID,
					CounterpartyID: round.OwnerID,
				}); err != nil {
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
	return r.cancelBetRound(ctx, roundID, ownerID, outboxPayload, true)
}

func (r *OrderRepo) AdminCancelBetRound(ctx context.Context, roundID string, outboxPayload []byte) (*model.BetRound, []model.BetWager, error) {
	return r.cancelBetRound(ctx, roundID, "", outboxPayload, false)
}

func (r *OrderRepo) cancelBetRound(ctx context.Context, roundID, ownerID string, outboxPayload []byte, requireOwner bool) (*model.BetRound, []model.BetWager, error) {
	var cancelledRound model.BetRound
	var refunded []model.BetWager
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var round model.BetRound
		if err := lockBetRound(tx, roundID, &round); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBetRoundNotFound
			}
			return err
		}
		if requireOwner && round.OwnerID != ownerID {
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("round_id = ? AND status = ?", roundID, model.StatusLocked).
			Find(&refunded).Error; err != nil {
			return err
		}
		for i := range refunded {
			w := &refunded[i]
			if _, err := wallet.Credit(tx, w.UserID, w.Amount, betRefundEntry(&round, w)); err != nil {
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

// StaleBetRounds returns up to limit rounds still awaiting a result whose
// betting closed at or before cutoff.
func (r *OrderRepo) StaleBetRounds(ctx context.Context, cutoff time.Time, limit int) ([]model.BetRound, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rounds []model.BetRound
	err := r.db.WithContext(ctx).
		Where("status IN ? AND close_at <= ?", []string{model.BetRoundOpen, model.BetRoundClosed}, cutoff).
		Order("close_at ASC").
		Limit(limit).
		Find(&rounds).Error
	return rounds, err
}

// RefundOrphanedBetWagers refunds up to limit wagers still `locked` on a
// settled or cancelled round. Such wagers were never part of the payout or
// refund (they could slip in before rounds were locked), so without this the
// stake is lost. Each refund is its own transaction and is idempotent.
func (r *OrderRepo) RefundOrphanedBetWagers(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var orphans []model.BetWager
	if err := r.db.WithContext(ctx).
		Select("bet_wagers.*").
		Joins("JOIN bet_rounds ON bet_rounds.id = bet_wagers.round_id").
		Where("bet_wagers.status = ? AND bet_rounds.status IN ?", model.StatusLocked,
			[]string{model.BetRoundSettled, model.BetRoundCancelled}).
		Order("bet_wagers.created_at ASC").
		Limit(limit).
		Find(&orphans).Error; err != nil {
		return 0, err
	}
	refunded := 0
	var firstErr error
	for i := range orphans {
		w := orphans[i]
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Same lock order as wager/settle/cancel: round, then wager, then user.
			var round model.BetRound
			if err := lockBetRound(tx, w.RoundID, &round); err != nil {
				return err
			}
			if round.Status != model.BetRoundSettled && round.Status != model.BetRoundCancelled {
				return ErrBetClosed
			}
			res := tx.Model(&model.BetWager{}).
				Where("id = ? AND status = ?", w.ID, model.StatusLocked).
				Updates(map[string]any{"status": model.StatusRefunded, "payout": w.Amount})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrBetClosed
			}
			_, err := wallet.Credit(tx, w.UserID, w.Amount, betRefundEntry(&round, &w))
			return err
		})
		if errors.Is(err, ErrBetClosed) {
			continue
		}
		// Keep going so one bad row cannot hold up every later refund.
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		refunded++
	}
	return refunded, firstErr
}

// betRefundEntry is the ledger entry for returning a wager's stake.
func betRefundEntry(round *model.BetRound, w *model.BetWager) wallet.Entry {
	return wallet.Entry{
		Type:           model.CoinTxBetRefund,
		Title:          "竞猜返还",
		Description:    round.Question,
		SourceType:     "bet_wager",
		SourceID:       w.ID,
		RoomID:         w.RoomID,
		CounterpartyID: round.OwnerID,
	}
}

// lockBetRound loads a round with SELECT ... FOR UPDATE. SQLite (tests) has
// no row locks and its dialect drops the clause; it serialises writers anyway.
func lockBetRound(tx *gorm.DB, roundID string, round *model.BetRound) error {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roundID).Take(round).Error
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
		if _, err := wallet.Debit(tx, o.UserID, o.TotalCoin, wallet.Entry{
			Type:           model.CoinTxGiftSpend,
			Title:          "礼物消费",
			Description:    fmt.Sprintf("%s x%d", o.GiftID, o.Count),
			SourceType:     "gift_order",
			SourceID:       o.OrderID,
			RoomID:         o.RoomID,
			CounterpartyID: receiverID,
		}); err != nil {
			return err
		}
		if err := tx.Create(o).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateRequest
			}
			return err
		}
		if receiverID != "" {
			creatorIncomeTitle := "直播礼物收入"
			if o.GiftID == "fan_light" {
				creatorIncomeTitle = "加入粉丝团收入"
			}
			if err := creditRoomOwner(tx, receiverID, o.TotalCoin, wallet.Entry{
				Type:           model.CoinTxCreatorGiftIncome,
				Title:          creatorIncomeTitle,
				Description:    fmt.Sprintf("%s x%d", o.GiftID, o.Count),
				SourceType:     "gift_order",
				SourceID:       o.OrderID,
				RoomID:         o.RoomID,
				CounterpartyID: o.UserID,
			}); err != nil {
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

// ErrAlreadyFanClubMember means the user already holds a fan badge for the
// creator, i.e. is in their fan club. Badges do not expire.
var ErrAlreadyFanClubMember = errors.New("already a fan club member")

// PlaceFanClubJoinOrder charges o.TotalCoin to join creatorID's fan club and
// creates the user's badge for that creator. A user who already holds the
// badge gets ErrAlreadyFanClubMember and is not charged, unless o.RequestID
// is the join that created it: that retry replays the existing order.
func (r *OrderRepo) PlaceFanClubJoinOrder(
	ctx context.Context,
	o *model.GiftOrder,
	creatorID string,
) (placed *model.GiftOrder, replayed bool, err error) {
	creatorID = strings.TrimSpace(creatorID)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the fan's users row before reading anything. Every write that
		// creates a badge debits the fan and so holds this lock until commit,
		// so a second join (the client sends a new requestId per click) waits
		// here for the first. Locking first also makes the membership check
		// this transaction's first consistent read: under REPEATABLE READ its
		// snapshot is taken after the lock and sees the other join's badge.
		// (A locking read of the missing badge would take a gap lock instead,
		// which deadlocks concurrent joins by other fans inserting into it.)
		if err := lockUser(tx, o.UserID); err != nil {
			return err
		}
		member, err := hasFanBadge(tx, o.UserID, creatorID)
		if err != nil {
			return err
		}
		if member {
			return ErrAlreadyFanClubMember
		}

		creatorName, _, err := creatorProfile(tx, creatorID)
		if err != nil {
			return err
		}
		if creatorName == "" {
			creatorName = creatorID
		}

		if _, err := wallet.Debit(tx, o.UserID, o.TotalCoin, wallet.Entry{
			Type:           model.CoinTxGiftSpend,
			Title:          "加入粉丝团",
			Description:    creatorName,
			SourceType:     "gift_order",
			SourceID:       o.OrderID,
			RoomID:         o.RoomID,
			CounterpartyID: creatorID,
		}); err != nil {
			return err
		}
		if err := tx.Create(o).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateRequest
			}
			return err
		}
		fanName, _, _ := creatorProfile(tx, o.UserID)
		if fanName == "" {
			fanName = o.UserID
		}
		if err := creditRoomOwner(tx, creatorID, o.TotalCoin, wallet.Entry{
			Type:           model.CoinTxCreatorGiftIncome,
			Title:          "加入粉丝团收入",
			Description:    fanName,
			SourceType:     "gift_order",
			SourceID:       o.OrderID,
			RoomID:         o.RoomID,
			CounterpartyID: o.UserID,
		}); err != nil {
			return err
		}
		return applyFanBadgeContribution(tx, o.UserID, o.RoomID, creatorID, o.TotalCoin, FanBadgeCreate)
	})
	if errors.Is(err, ErrDuplicateRequest) || errors.Is(err, ErrAlreadyFanClubMember) {
		// A member may be retrying the very join that made them one.
		existing, ferr := r.FindGiftByRequestID(ctx, o.UserID, o.RequestID)
		if ferr != nil {
			return nil, false, ferr
		}
		if existing == nil && errors.Is(err, ErrAlreadyFanClubMember) {
			return nil, false, err
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return o, false, nil
}

// lockUser locks userID's users row (SELECT ... FOR UPDATE) until the
// transaction ends. A missing user is left for wallet.Debit to report. SQLite
// (tests) drops the clause, as for lockBetRound.
func lockUser(tx *gorm.DB, userID string) error {
	var ids []string
	return tx.Table("users").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", userID).Pluck("id", &ids).Error
}

func hasFanBadge(tx *gorm.DB, userID, creatorID string) (bool, error) {
	var n int64
	err := tx.Model(&model.FanBadge{}).
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Count(&n).Error
	return n > 0, err
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

		if _, err := wallet.Debit(tx, o.UserID, o.Amount, wallet.Entry{
			Type:           model.CoinTxSuperChatSpend,
			Title:          "SC 消费",
			Description:    o.Text,
			SourceType:     "super_chat_order",
			SourceID:       o.OrderID,
			RoomID:         o.RoomID,
			CounterpartyID: receiverID,
		}); err != nil {
			return err
		}
		if err := tx.Create(o).Error; err != nil {
			if isDuplicateKey(err) {
				return ErrDuplicateRequest
			}
			return err
		}
		if receiverID != "" {
			if err := creditRoomOwner(tx, receiverID, o.Amount, wallet.Entry{
				Type:           model.CoinTxCreatorSuperChatIncome,
				Title:          "直播 SC 收入",
				Description:    o.Text,
				SourceType:     "super_chat_order",
				SourceID:       o.OrderID,
				RoomID:         o.RoomID,
				CounterpartyID: o.UserID,
			}); err != nil {
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

// applyFanBadgeContribution adds coin to userID's badge for creatorID. The
// total is incremented in SQL (an upsert when the badge may be created)
// rather than read, added to and written back: under REPEATABLE READ a plain
// SELECT here sees the snapshot from the transaction's first query, which can
// predate a concurrent gift's commit, so that gift's contribution was lost or
// the second first-time Fan Light failed on the duplicate key.
func applyFanBadgeContribution(tx *gorm.DB, userID, roomID, creatorID string, coin int64, mode FanBadgeContributionMode) error {
	if mode == FanBadgeNoChange || coin <= 0 || userID == "" || creatorID == "" || userID == creatorID {
		return nil
	}

	now := time.Now()
	if mode == FanBadgeCreate {
		name, avatar, err := creatorProfileForFanBadge(tx, roomID, creatorID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(name) == "" {
			name = creatorID
		}
		badge := model.FanBadge{
			UserID:            userID,
			CreatorID:         creatorID,
			CreatorName:       strings.TrimSpace(name),
			CreatorAvatar:     strings.TrimSpace(avatar),
			TotalContribution: coin,
			Level:             FanBadgeLevel(coin),
		}
		// The bare column in ON DUPLICATE KEY UPDATE (MySQL) and DO UPDATE
		// (SQLite) is the stored value, so this adds to the existing total.
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "creator_id"}},
			DoUpdates: clause.Set{
				{Column: clause.Column{Name: "total_contribution"}, Value: gorm.Expr("total_contribution + ?", coin)},
				{Column: clause.Column{Name: "updated_at"}, Value: now},
			},
		}).Create(&badge).Error; err != nil {
			return err
		}
	} else {
		res := tx.Model(&model.FanBadge{}).
			Where("user_id = ? AND creator_id = ?", userID, creatorID).
			Updates(map[string]any{
				"total_contribution": gorm.Expr("total_contribution + ?", coin),
				"updated_at":         now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
	}

	// Re-derive the level from the stored total. The write above already
	// holds the row lock; the locking read makes this a current read rather
	// than a snapshot one.
	var badge model.FanBadge
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("total_contribution", "level").
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Take(&badge).Error; err != nil {
		return err
	}
	level := FanBadgeLevel(badge.TotalContribution)
	if level == badge.Level {
		return nil
	}
	return tx.Model(&model.FanBadge{}).
		Where("user_id = ? AND creator_id = ?", userID, creatorID).
		Update("level", level).Error
}

func creatorProfileForFanBadge(tx *gorm.DB, roomID, creatorID string) (string, string, error) {
	if strings.TrimSpace(roomID) != "" {
		return roomCreatorProfile(tx, roomID)
	}
	return creatorProfile(tx, creatorID)
}

func creatorProfile(tx *gorm.DB, creatorID string) (string, string, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return "", "", ErrRoomOwnerNotFound
	}
	var row struct {
		Name   string
		Avatar string
	}
	err := tx.Raw(`
SELECT
  COALESCE(NULLIF(display_name, ''), NULLIF(username, ''), id, '') AS name,
  COALESCE(NULLIF(avatar, ''), '') AS avatar
FROM users
WHERE id = ?
`, creatorID).Row().Scan(&row.Name, &row.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrRoomOwnerNotFound
	}
	if err != nil {
		return "", "", err
	}
	return row.Name, row.Avatar, nil
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

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}
