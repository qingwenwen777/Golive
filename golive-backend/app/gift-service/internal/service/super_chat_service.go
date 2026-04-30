package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

type SendSuperChatReq struct {
	UserID    string
	Username  string
	RoomID    string
	Amount    int64
	Text      string
	RequestID string
}

// AmountToTier mirrors src/types/gift.ts amountToTier exactly.
func AmountToTier(amount int64) int {
	switch {
	case amount >= 10000:
		return 5
	case amount >= 5000:
		return 4
	case amount >= 2000:
		return 3
	case amount >= 1000:
		return 2
	case amount >= 200:
		return 1
	default:
		return 0
	}
}

// ErrInvalidAmount is returned when amount maps to tier 0. Tier 0 super
// chats are not allowed (the frontend won't send them but we double-check).
var ErrInvalidAmount = errors.New("amount below minimum tier")

type SuperChatService struct {
	orders *repo.OrderRepo
}

func NewSuperChatService(o *repo.OrderRepo) *SuperChatService {
	return &SuperChatService{orders: o}
}

func (s *SuperChatService) Send(ctx context.Context, req SendSuperChatReq) (*model.SuperChatOrder, bool, error) {
	tier := AmountToTier(req.Amount)
	if tier == 0 {
		return nil, false, ErrInvalidAmount
	}

	now := time.Now().UTC()
	orderID := "sc-" + uuid.NewString()
	username := s.broadcastName(ctx, req)

	payload, err := repo.MarshalSuperChatOutbox(orderID, username,
		strconv.FormatInt(req.Amount, 10), tier, req.Text, now.UnixMilli())
	if err != nil {
		return nil, false, err
	}

	order := &model.SuperChatOrder{
		OrderID:   orderID,
		RequestID: req.RequestID,
		UserID:    req.UserID,
		RoomID:    req.RoomID,
		Amount:    req.Amount,
		Tier:      tier,
		Text:      req.Text,
		Status:    model.StatusSuccess,
		CreatedAt: now,
	}

	placed, replayed, err := s.orders.PlaceSuperChatOrder(ctx, order, payload)
	if err == nil {
		return placed, replayed, nil
	}
	if !errors.Is(err, repo.ErrInsufficientFunds) {
		return nil, false, fmt.Errorf("place sc: %w", err)
	}

	failed := &model.SuperChatOrder{
		OrderID:    "sc-" + uuid.NewString(),
		RequestID:  req.RequestID,
		UserID:     req.UserID,
		RoomID:     req.RoomID,
		Amount:     req.Amount,
		Tier:       tier,
		Text:       req.Text,
		Status:     model.StatusFailed,
		FailReason: model.FailInsufficientCoin,
		CreatedAt:  now,
	}
	persisted, perr := s.orders.PersistSuperChatFailure(ctx, failed)
	if perr != nil {
		return nil, false, fmt.Errorf("persist sc failure: %w", perr)
	}
	return persisted, false, ErrInsufficientCoin
}

func (s *SuperChatService) broadcastName(ctx context.Context, req SendSuperChatReq) string {
	if name, err := s.orders.DisplayNameForUser(ctx, req.UserID); err == nil && name != "" {
		return cleanBroadcastName(name, req.UserID)
	}
	return cleanBroadcastName(req.Username, req.UserID)
}

func cleanBroadcastName(name, userID string) string {
	name = strings.TrimSpace(name)
	if name != "" && len([]rune(name)) <= 64 && !strings.ContainsAny(name, "\r\n\t") {
		return name
	}

	id := strings.TrimSpace(userID)
	if id == "" {
		return "Creator"
	}
	runes := []rune(id)
	if len(runes) > 8 {
		id = string(runes[:8])
	}
	return "Creator " + id
}
