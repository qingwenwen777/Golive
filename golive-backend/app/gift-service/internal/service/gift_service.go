package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/obs"
)

// SendGiftReq is what the HTTP layer hands us after parsing the body.
type SendGiftReq struct {
	UserID    string
	Username  string // for the broadcast "user" field
	RoomID    string
	GiftID    string
	Count     int
	RequestID string
}

type JoinFanClubReq struct {
	UserID    string
	CreatorID string
	RequestID string
}

// SendResp wraps the order plus a replay flag the handler uses to set
// the Idempotent-Replayed header. status=success → HTTP 200. status=failed +
// failReason=insufficient_coin → HTTP 402.
type SendResp struct {
	Order    *model.GiftOrder
	Replayed bool
}

type GiftService struct {
	gifts  *repo.GiftRepo
	orders *repo.OrderRepo
}

func NewGiftService(g *repo.GiftRepo, o *repo.OrderRepo) *GiftService {
	return &GiftService{gifts: g, orders: o}
}

func (s *GiftService) List(ctx context.Context) ([]model.Gift, error) {
	return s.gifts.List(ctx)
}

func (s *GiftService) ListFanBadges(ctx context.Context, userID string) ([]model.FanBadge, error) {
	return s.orders.ListFanBadges(ctx, userID)
}

func (s *GiftService) ListFanClubMembers(ctx context.Context, creatorID string, limit int) (*model.FanClubMembersResponse, error) {
	items, total, err := s.orders.ListFanClubMembers(ctx, creatorID, limit)
	if err != nil {
		return nil, err
	}
	return &model.FanClubMembersResponse{Items: items, Total: total}, nil
}

// Send is the canonical send path.
//
// Pre-conditions checked here (assumes HTTP-level validation already ran):
//
//	req.RoomID, req.GiftID, req.RequestID, req.UserID — non-empty
//
// req.Count is validated here against MaxGiftCount.
//
// Behaviour:
//   - gift unknown → return (nil, false, ErrGiftNotFound)
//   - replay (DB unique hit) → (existing order, true, nil)
//   - balance insufficient → (failed order persisted, false, ErrInsufficientCoin)
//   - success → (success order persisted, false, nil)
//
// Callers MUST treat ErrInsufficientCoin as "render 402 with the order body";
// the order returned alongside the error is the authoritative one to cache.
var (
	ErrGiftNotFound     = errors.New("gift not found")
	ErrInsufficientCoin = errors.New("insufficient coin")
	ErrGiftLevelLocked  = errors.New("gift level locked")
	ErrSelfFanClubJoin  = errors.New("cannot join your own fan club")

	ErrInvalidGiftCount = errcode.New(http.StatusBadRequest, "invalid gift count").WithReason("invalid_gift_count")
)

// MaxGiftCount bounds a single send so price*count can never overflow int64.
const MaxGiftCount = 9999

// giftTotal returns price*count, rejecting counts outside 1..MaxGiftCount and
// any product that would not be a positive int64.
func giftTotal(price int64, count int) (int64, error) {
	if count <= 0 || count > MaxGiftCount || price <= 0 || int64(count) > math.MaxInt64/price {
		return 0, ErrInvalidGiftCount
	}
	return price * int64(count), nil
}

type GiftLevelLockedError struct {
	RequiredLevel int
	UserLevel     int
}

func (e *GiftLevelLockedError) Error() string { return ErrGiftLevelLocked.Error() }
func (e *GiftLevelLockedError) Unwrap() error { return ErrGiftLevelLocked }

func (s *GiftService) Send(ctx context.Context, req SendGiftReq) (*model.GiftOrder, bool, error) {
	// End-to-end span: this is the parent for everything that happens during
	// the send — balance debit, ledger insert, outbox enqueue. The kafka
	// outbox worker emits its own span and links back via the order id.
	ctx, span := obs.Tracer("gift-service/service").Start(ctx, "gift.send") // trace cardinality stays bounded — we don't put userId / requestId
	// as labels on Prom metrics, but spans can carry them for debugging.

	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", req.UserID),
		attribute.String("room.id", req.RoomID),
		attribute.String("gift.id", req.GiftID),
		attribute.Int("gift.count", req.Count),
		attribute.String("request.id", req.RequestID),
	)

	gift, err := s.gifts.Get(ctx, req.GiftID)
	if err != nil {
		if errors.Is(err, repo.ErrGiftNotFound) {
			return nil, false, ErrGiftNotFound
		}
		return nil, false, fmt.Errorf("get gift: %w", err)
	}
	if !gift.Enabled {
		return nil, false, ErrGiftNotFound
	}
	userLevel, err := s.orders.UserLevel(ctx, req.UserID)
	if err != nil {
		return nil, false, fmt.Errorf("user level: %w", err)
	}
	if gift.UnlockLevel > 1 && userLevel < gift.UnlockLevel {
		return nil, false, &GiftLevelLockedError{
			RequiredLevel: gift.UnlockLevel,
			UserLevel:     userLevel,
		}
	}

	totalCoin, err := giftTotal(gift.PriceCoin, req.Count)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	orderID := "gift-" + uuid.NewString()
	username := s.broadcastName(ctx, req)
	avatar, _ := s.orders.AvatarForUser(ctx, req.UserID)

	payload, err := repo.MarshalGiftOutbox(
		orderID,
		req.RequestID,
		req.UserID,
		username,
		avatar,
		gift.Name,
		gift.Icon,
		req.Count,
		gift.Tier,
		userLevel,
		totalCoin,
		now.UnixMilli(),
	)
	if err != nil {
		return nil, false, err
	}

	order := &model.GiftOrder{
		OrderID:   orderID,
		RequestID: req.RequestID,
		UserID:    req.UserID,
		RoomID:    req.RoomID,
		GiftID:    gift.ID,
		Count:     req.Count,
		TotalCoin: totalCoin,
		Status:    model.StatusSuccess,
		CreatedAt: now,
	}

	badgeMode := repo.FanBadgeIfExists
	if gift.ID == "fan_light" {
		badgeMode = repo.FanBadgeCreate
	}
	placed, replayed, err := s.orders.PlaceGiftOrder(ctx, order, payload, badgeMode)
	if err == nil {
		span.SetAttributes(
			attribute.Bool("idempotent.replayed", replayed),
			attribute.String("order.id", placed.OrderID),
			attribute.Int64("order.total_coin", placed.TotalCoin),
		)
		obs.GiftRevenueTotal.WithLabelValues(model.StatusSuccess).Add(float64(placed.TotalCoin))
		return placed, replayed, nil
	}
	if !errors.Is(err, repo.ErrInsufficientFunds) {
		span.RecordError(err)
		return nil, false, err
	}
	span.SetAttributes(attribute.String("fail.reason", model.FailInsufficientCoin))

	// Insufficient funds → persist a `failed` order so subsequent retries
	// for the same requestId replay this same outcome (no further DB work).
	failed := &model.GiftOrder{
		OrderID:    "gift-" + uuid.NewString(),
		RequestID:  req.RequestID,
		UserID:     req.UserID,
		RoomID:     req.RoomID,
		GiftID:     gift.ID,
		Count:      req.Count,
		TotalCoin:  totalCoin,
		Status:     model.StatusFailed,
		FailReason: model.FailInsufficientCoin,
		CreatedAt:  now,
	}
	persisted, perr := s.orders.PersistGiftFailure(ctx, failed)
	if perr != nil {
		span.RecordError(perr)
		return nil, false, fmt.Errorf("persist failure: %w", perr)
	}
	obs.GiftRevenueTotal.WithLabelValues(model.StatusFailed).Add(0) // count failures w/o revenue
	return persisted, false, ErrInsufficientCoin
}

func (s *GiftService) JoinFanClub(ctx context.Context, req JoinFanClubReq) (*model.GiftOrder, bool, error) {
	if req.CreatorID == "" || req.UserID == "" {
		return nil, false, ErrGiftNotFound
	}
	if req.CreatorID == req.UserID {
		return nil, false, ErrSelfFanClubJoin
	}
	gift, err := s.gifts.Get(ctx, "fan_light")
	if err != nil {
		if errors.Is(err, repo.ErrGiftNotFound) {
			return nil, false, ErrGiftNotFound
		}
		return nil, false, fmt.Errorf("get fan light: %w", err)
	}
	if !gift.Enabled {
		return nil, false, ErrGiftNotFound
	}
	userLevel, err := s.orders.UserLevel(ctx, req.UserID)
	if err != nil {
		return nil, false, fmt.Errorf("user level: %w", err)
	}
	if gift.UnlockLevel > 1 && userLevel < gift.UnlockLevel {
		return nil, false, &GiftLevelLockedError{
			RequiredLevel: gift.UnlockLevel,
			UserLevel:     userLevel,
		}
	}

	totalCoin := gift.PriceCoin
	now := time.Now().UTC()
	order := &model.GiftOrder{
		OrderID:   "gift-" + uuid.NewString(),
		RequestID: req.RequestID,
		UserID:    req.UserID,
		RoomID:    "",
		GiftID:    gift.ID,
		Count:     1,
		TotalCoin: totalCoin,
		Status:    model.StatusSuccess,
		CreatedAt: now,
	}
	placed, replayed, err := s.orders.PlaceFanClubJoinOrder(ctx, order, req.CreatorID)
	if err == nil {
		obs.GiftRevenueTotal.WithLabelValues(model.StatusSuccess).Add(float64(placed.TotalCoin))
		return placed, replayed, nil
	}
	if errors.Is(err, repo.ErrRoomOwnerNotFound) {
		return nil, false, ErrGiftNotFound
	}
	if !errors.Is(err, repo.ErrInsufficientFunds) {
		return nil, false, err
	}
	failed := &model.GiftOrder{
		OrderID:    "gift-" + uuid.NewString(),
		RequestID:  req.RequestID,
		UserID:     req.UserID,
		RoomID:     "",
		GiftID:     gift.ID,
		Count:      1,
		TotalCoin:  totalCoin,
		Status:     model.StatusFailed,
		FailReason: model.FailInsufficientCoin,
		CreatedAt:  now,
	}
	persisted, perr := s.orders.PersistGiftFailure(ctx, failed)
	if perr != nil {
		return nil, false, fmt.Errorf("persist failure: %w", perr)
	}
	obs.GiftRevenueTotal.WithLabelValues(model.StatusFailed).Add(0)
	return persisted, false, ErrInsufficientCoin
}

func (s *GiftService) broadcastName(ctx context.Context, req SendGiftReq) string {
	if name, err := s.orders.DisplayNameForUser(ctx, req.UserID); err == nil && name != "" {
		return cleanBroadcastName(name, req.UserID)
	}
	return cleanBroadcastName(req.Username, req.UserID)
}
