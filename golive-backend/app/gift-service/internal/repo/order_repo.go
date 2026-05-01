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
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrDuplicateRequest  = errors.New("duplicate request")
	ErrRoomOwnerNotFound = errors.New("room owner not found")
)

type OrderRepo struct{ db *gorm.DB }

func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

func (r *OrderRepo) AutoMigrate() error {
	return r.db.AutoMigrate(&model.GiftOrder{}, &model.SuperChatOrder{}, &model.LocalMessage{}, &model.FanBadge{})
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

func FanBadgeLevel(totalContribution int64) int {
	if totalContribution <= 0 {
		return 1
	}
	level := 1
	threshold := int64(1000)
	for level < 99 && totalContribution >= threshold {
		level++
		threshold += int64(level) * 1000
	}
	return level
}

func (r *OrderRepo) ListFanBadges(ctx context.Context, userID string) ([]model.FanBadge, error) {
	var badges []model.FanBadge
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("level DESC, total_contribution DESC, updated_at DESC").
		Find(&badges).Error
	return badges, err
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
		if receiverID != "" {
			res = tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", o.TotalCoin, receiverID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrRoomOwnerNotFound
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
		if receiverID != "" {
			res = tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", o.Amount, receiverID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrRoomOwnerNotFound
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
func MarshalGiftOutbox(id, requestID, userID, username, avatar, giftName, giftIcon string, count, tier int, totalCoin, ts int64) ([]byte, error) {
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
		"totalCoin": totalCoin,
		"ts":        ts,
	})
}

func MarshalSuperChatOutbox(id, userID, user, avatar, amount string, tier int, text string, ts int64) ([]byte, error) {
	payload := map[string]any{
		"type":   "super_chat",
		"id":     id,
		"userId": userID,
		"user":   user,
		"amount": amount,
		"tier":   tier,
		"text":   text,
		"ts":     ts,
	}
	if avatar != "" {
		payload["avatar"] = avatar
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
