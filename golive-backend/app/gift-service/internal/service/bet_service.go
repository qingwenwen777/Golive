package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

const MaxBetQuestionRunes = 80

var (
	ErrBetActive       = errors.New("active bet round exists")
	ErrBetNotFound     = errors.New("bet round not found")
	ErrBetClosed       = errors.New("bet closed")
	ErrBetAlready      = errors.New("bet already placed")
	ErrBetUnauthorized = errors.New("bet unauthorized")
	ErrBetNoWinners    = errors.New("bet has no winners")
	ErrBetBadOption    = errors.New("bad bet option")
	ErrBetBadQuestion  = errors.New("bad bet question")
)

type BetService struct {
	orders *repo.OrderRepo
}

func NewBetService(o *repo.OrderRepo) *BetService {
	return &BetService{orders: o}
}

type BetRoundView struct {
	Round   *model.BetRound         `json:"round"`
	Summary []repo.BetOptionSummary `json:"summary"`
	MyWager *model.BetWager         `json:"myWager,omitempty"`
}

func (s *BetService) Latest(ctx context.Context, roomID, userID string) (*BetRoundView, error) {
	round, summary, wager, err := s.orders.LatestBetRound(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if round == nil {
		return nil, nil
	}
	return &BetRoundView{Round: round, Summary: summary, MyWager: wager}, nil
}

func (s *BetService) Open(ctx context.Context, ownerID, roomID string, amount int64, question string) (*BetRoundView, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	question = strings.TrimSpace(question)
	if question == "" || len([]rune(question)) > MaxBetQuestionRunes {
		return nil, ErrBetBadQuestion
	}
	actualOwner, err := s.orders.RoomOwner(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if actualOwner != ownerID {
		return nil, ErrBetUnauthorized
	}
	now := time.Now().UTC()
	round := &model.BetRound{
		ID:       "bet-" + uuid.NewString(),
		RoomID:   roomID,
		OwnerID:  ownerID,
		Question: question,
		Amount:   amount,
		Status:   model.BetRoundOpen,
		CloseAt:  now.Add(60 * time.Second),
	}
	summary := emptyBetSummary()
	payload, err := repo.MarshalBetOutbox("opened", round, summary, "", "", now.UnixMilli())
	if err != nil {
		return nil, err
	}
	if err := s.orders.CreateBetRound(ctx, round, payload); err != nil {
		return nil, mapBetErr(err)
	}
	return &BetRoundView{Round: round, Summary: summary}, nil
}

func (s *BetService) Wager(ctx context.Context, userID, roomID, roundID, option string) (*model.BetWager, error) {
	if !validBetOption(option) {
		return nil, ErrBetBadOption
	}
	now := time.Now().UTC()
	wager := &model.BetWager{
		ID:        "bw-" + uuid.NewString(),
		RoundID:   roundID,
		RoomID:    roomID,
		UserID:    userID,
		Option:    option,
		Status:    model.StatusLocked,
		CreatedAt: now,
	}
	payload, err := repo.MarshalBetOutbox("wagered", &model.BetRound{ID: roundID, RoomID: roomID}, nil, userID, option, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	placed, err := s.orders.PlaceBetWager(ctx, wager, payload)
	if err != nil {
		return placed, mapBetErr(err)
	}
	return placed, nil
}

func (s *BetService) Settle(ctx context.Context, ownerID, roundID, option string) (*BetRoundView, error) {
	if !validBetOption(option) {
		return nil, ErrBetBadOption
	}
	now := time.Now().UTC()
	// Probe the round so the outbox payload carries the correct roomId; the
	// authoritative status check still happens inside SettleBetRound.
	probe, err := s.orders.GetBetRound(ctx, roundID)
	if err != nil {
		return nil, mapBetErr(err)
	}
	if probe.OwnerID != ownerID {
		return nil, ErrBetUnauthorized
	}
	probe.Status = model.BetRoundSettled
	probe.WinningOption = option
	payload, err := repo.MarshalBetOutbox("settled", probe, nil, "", option, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	round, _, err := s.orders.SettleBetRound(ctx, roundID, ownerID, option, payload)
	if err != nil {
		return nil, mapBetErr(err)
	}
	latest, err := s.Latest(ctx, round.RoomID, "")
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (s *BetService) Cancel(ctx context.Context, ownerID, roundID string) (*BetRoundView, error) {
	now := time.Now().UTC()
	probe, err := s.orders.GetBetRound(ctx, roundID)
	if err != nil {
		return nil, mapBetErr(err)
	}
	if probe.OwnerID != ownerID {
		return nil, ErrBetUnauthorized
	}
	probe.Status = model.BetRoundCancelled
	payload, err := repo.MarshalBetOutbox("cancelled", probe, nil, "", "", now.UnixMilli())
	if err != nil {
		return nil, err
	}
	round, _, err := s.orders.CancelBetRound(ctx, roundID, ownerID, payload)
	if err != nil {
		return nil, mapBetErr(err)
	}
	latest, err := s.Latest(ctx, round.RoomID, "")
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func emptyBetSummary() []repo.BetOptionSummary {
	return []repo.BetOptionSummary{
		{Option: model.BetOptionWin},
		{Option: model.BetOptionLose},
	}
}

func validBetOption(option string) bool {
	return option == model.BetOptionWin || option == model.BetOptionLose
}

func mapBetErr(err error) error {
	switch {
	case errors.Is(err, repo.ErrActiveBetExists):
		return ErrBetActive
	case errors.Is(err, repo.ErrBetRoundNotFound):
		return ErrBetNotFound
	case errors.Is(err, repo.ErrBetClosed):
		return ErrBetClosed
	case errors.Is(err, repo.ErrBetAlreadyPlaced):
		return ErrBetAlready
	case errors.Is(err, repo.ErrBetUnauthorized):
		return ErrBetUnauthorized
	case errors.Is(err, repo.ErrBetNoWinners):
		return ErrBetNoWinners
	case errors.Is(err, repo.ErrInsufficientFunds):
		return ErrInsufficientCoin
	default:
		return err
	}
}
