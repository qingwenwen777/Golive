package service

import (
	"context"
	"time"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

var ErrInvalidGiftPrice = repo.ErrInvalidGiftPrice

type AdminService struct {
	admin  *repo.AdminRepo
	orders *repo.OrderRepo
}

func NewAdminService(admin *repo.AdminRepo, orders *repo.OrderRepo) *AdminService {
	return &AdminService{admin: admin, orders: orders}
}

func (s *AdminService) IsAdmin(ctx context.Context, userID string) (bool, error) {
	return s.admin.IsAdmin(ctx, userID)
}

func (s *AdminService) IsBanned(ctx context.Context, userID string) (bool, error) {
	return s.admin.IsBanned(ctx, userID)
}

func (s *AdminService) EconomySummary(ctx context.Context) (repo.AdminEconomySummary, error) {
	return s.admin.EconomySummary(ctx)
}

func (s *AdminService) ListGifts(ctx context.Context) ([]model.Gift, repo.AdminGiftStats, error) {
	return s.admin.ListGifts(ctx)
}

func (s *AdminService) UpdateGift(ctx context.Context, id string, patch repo.AdminGiftPatch) (*model.Gift, error) {
	return s.admin.UpdateGift(ctx, id, patch)
}

// ModerateSuperChat hides a super chat from public history; see
// repo.AdminRepo.ModerateSuperChat for why it does not refund.
func (s *AdminService) ModerateSuperChat(ctx context.Context, orderID, operatorID string) (*model.SuperChatOrder, error) {
	return s.admin.ModerateSuperChat(ctx, orderID, operatorID, time.Now().UTC())
}

func (s *AdminService) ListOrders(ctx context.Context, f repo.AdminOrderFilter) ([]repo.AdminOrderRecord, int64, int, int, error) {
	return s.admin.ListOrders(ctx, f)
}

func (s *AdminService) CoinLedger(ctx context.Context, f repo.AdminCoinFilter) ([]repo.AdminCoinRecord, int64, int, int, repo.AdminCoinStats, error) {
	return s.admin.CoinLedger(ctx, f)
}

func (s *AdminService) ListBetRounds(ctx context.Context, f repo.AdminBetFilter) ([]repo.AdminBetRoundRecord, int64, int, int, repo.AdminBetStats, error) {
	return s.admin.ListBetRounds(ctx, f)
}

func (s *AdminService) SettleBetRound(ctx context.Context, roundID, option string) (*repo.AdminBetRoundRecord, error) {
	if !validBetOption(option) {
		return nil, ErrBetBadOption
	}
	now := time.Now().UTC()
	probe, err := s.orders.GetBetRound(ctx, roundID)
	if err != nil {
		return nil, mapBetErr(err)
	}
	probe.Status = model.BetRoundSettled
	probe.WinningOption = option
	payload, err := repo.MarshalBetOutbox("settled", probe, nil, "", option, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	round, _, err := s.orders.AdminSettleBetRound(ctx, roundID, option, payload)
	if err != nil {
		return nil, mapBetErr(err)
	}
	return s.admin.BetRoundDetail(ctx, round.ID)
}

func (s *AdminService) CancelBetRound(ctx context.Context, roundID string) (*repo.AdminBetRoundRecord, error) {
	now := time.Now().UTC()
	probe, err := s.orders.GetBetRound(ctx, roundID)
	if err != nil {
		return nil, mapBetErr(err)
	}
	probe.Status = model.BetRoundCancelled
	payload, err := repo.MarshalBetOutbox("cancelled", probe, nil, "", "", now.UnixMilli())
	if err != nil {
		return nil, err
	}
	round, _, err := s.orders.AdminCancelBetRound(ctx, roundID, payload)
	if err != nil {
		return nil, mapBetErr(err)
	}
	return s.admin.BetRoundDetail(ctx, round.ID)
}

func (s *AdminService) RevenueReport(ctx context.Context, period string, limit int) ([]repo.AdminRevenueReportRow, error) {
	return s.admin.RevenueReport(ctx, period, limit)
}
