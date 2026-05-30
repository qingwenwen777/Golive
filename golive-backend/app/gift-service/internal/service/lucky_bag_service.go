package service

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/logger"
	"go.uber.org/zap"
)

const (
	MaxLuckyBagMessageRunes  = 60
	MinLuckyBagDurationSecs  = 30
	MaxLuckyBagDurationSecs  = 600
	DefaultLuckyBagDuration  = 60
	MaxLuckyBagCount         = 500
	luckyBagPresenceTTL      = 6 * time.Hour
	luckyBagSchedulerTick    = time.Second
	luckyBagSchedulerBatch   = 20
)

var (
	ErrLuckyBagActive       = errors.New("active lucky bag exists")
	ErrLuckyBagNotFound     = errors.New("lucky bag not found")
	ErrLuckyBagClosed       = errors.New("lucky bag closed")
	ErrLuckyBagAlreadyJoin  = errors.New("lucky bag already joined")
	ErrLuckyBagUnauthorized = errors.New("lucky bag unauthorized")
	ErrLuckyBagNotEligible  = errors.New("lucky bag not eligible")
	ErrLuckyBagBad          = errors.New("bad lucky bag")
)

type LuckyBagService struct {
	orders *repo.OrderRepo
	rdb    *redis.Client
}

func NewLuckyBagService(o *repo.OrderRepo, rdb *redis.Client) *LuckyBagService {
	return &LuckyBagService{orders: o, rdb: rdb}
}

type LuckyBagView struct {
	Bag              *model.LuckyBag      `json:"bag"`
	MyEntry          *model.LuckyBagEntry `json:"myEntry,omitempty"`
	ParticipantCount int64                `json:"participantCount"`
}

type OpenLuckyBagReq struct {
	RoomID          string `json:"roomId"`
	TotalCoin       int64  `json:"totalCoin"`
	Count           int    `json:"count"`
	AmountMode      string `json:"amountMode"`
	Eligibility     string `json:"eligibility"`
	MinFanLevel     int    `json:"minFanLevel"`
	DurationSeconds int    `json:"durationSeconds"`
	Message         string `json:"message"`
}

func (s *LuckyBagService) Latest(ctx context.Context, roomID, userID string) (*LuckyBagView, error) {
	bag, entry, count, err := s.orders.LatestLuckyBag(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if bag == nil {
		return nil, nil
	}
	return &LuckyBagView{Bag: bag, MyEntry: entry, ParticipantCount: count}, nil
}

func (s *LuckyBagService) Open(ctx context.Context, ownerID string, req OpenLuckyBagReq) (*LuckyBagView, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" || req.TotalCoin <= 0 || req.Count <= 0 || req.Count > MaxLuckyBagCount {
		return nil, ErrLuckyBagBad
	}
	if req.TotalCoin < int64(req.Count) {
		// every packet needs at least 1 coin
		return nil, ErrLuckyBagBad
	}
	mode := normalizeAmountMode(req.AmountMode)
	if mode == model.LuckyBagAmountFixed && req.TotalCoin%int64(req.Count) != 0 {
		// fixed mode requires an evenly divisible total so each packet is equal
		return nil, ErrLuckyBagBad
	}
	eligibility := normalizeEligibility(req.Eligibility)
	minFanLevel := 0
	if eligibility == model.LuckyBagEligibilityFansLevel {
		minFanLevel = req.MinFanLevel
		if minFanLevel < 1 {
			minFanLevel = 1
		}
	}
	duration := req.DurationSeconds
	if duration <= 0 {
		duration = DefaultLuckyBagDuration
	}
	if duration < MinLuckyBagDurationSecs {
		duration = MinLuckyBagDurationSecs
	}
	if duration > MaxLuckyBagDurationSecs {
		duration = MaxLuckyBagDurationSecs
	}
	actualOwner, err := s.orders.RoomOwner(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if actualOwner != ownerID {
		return nil, ErrLuckyBagUnauthorized
	}
	now := time.Now().UTC()
	bag := &model.LuckyBag{
		ID:          "lb-" + uuid.NewString(),
		RoomID:      roomID,
		OwnerID:     ownerID,
		Message:     cleanLuckyBagMessage(req.Message),
		TotalCoin:   req.TotalCoin,
		Count:       req.Count,
		AmountMode:  mode,
		Eligibility: eligibility,
		MinFanLevel: minFanLevel,
		Status:      model.LuckyBagOpen,
		CloseAt:     now.Add(time.Duration(duration) * time.Second),
	}
	payload, err := repo.MarshalLuckyBagOutbox("opened", bag, 0, 0, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	if err := s.orders.CreateLuckyBag(ctx, bag, payload); err != nil {
		return nil, mapLuckyBagErr(err)
	}
	return &LuckyBagView{Bag: bag, ParticipantCount: 0}, nil
}

func (s *LuckyBagService) Join(ctx context.Context, userID, roomID, bagID string) (*LuckyBagView, error) {
	bag, err := s.orders.GetLuckyBag(ctx, bagID)
	if err != nil {
		return nil, mapLuckyBagErr(err)
	}
	now := time.Now().UTC()
	if bag.Status != model.LuckyBagOpen || !now.Before(bag.CloseAt) {
		return nil, ErrLuckyBagClosed
	}
	if userID == bag.OwnerID {
		// The host cannot enter their own giveaway.
		return nil, ErrLuckyBagNotEligible
	}
	eligible, err := s.checkEligibility(ctx, userID, bag)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, ErrLuckyBagNotEligible
	}
	entry := &model.LuckyBagEntry{
		ID:        "lbe-" + uuid.NewString(),
		BagID:     bag.ID,
		RoomID:    bag.RoomID,
		UserID:    userID,
		Status:    model.LuckyBagEntryJoined,
		CreatedAt: now,
	}
	payload, err := repo.MarshalLuckyBagOutbox("joined", bag, 0, 0, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	if _, err := s.orders.JoinLuckyBag(ctx, entry, payload); err != nil {
		if !errors.Is(err, repo.ErrLuckyBagAlreadyJoin) {
			return nil, mapLuckyBagErr(err)
		}
	}
	return s.Latest(ctx, bag.RoomID, userID)
}

func (s *LuckyBagService) Cancel(ctx context.Context, ownerID, bagID string) (*LuckyBagView, error) {
	bag, err := s.orders.GetLuckyBag(ctx, bagID)
	if err != nil {
		return nil, mapLuckyBagErr(err)
	}
	if bag.OwnerID != ownerID {
		return nil, ErrLuckyBagUnauthorized
	}
	now := time.Now().UTC()
	bag.Status = model.LuckyBagCancelled
	payload, err := repo.MarshalLuckyBagOutbox("cancelled", bag, 0, 0, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	cancelled, err := s.orders.CancelLuckyBag(ctx, bagID, ownerID, payload)
	if err != nil {
		return nil, mapLuckyBagErr(err)
	}
	return s.Latest(ctx, cancelled.RoomID, "")
}

// RunScheduler draws due lucky bags on a fixed tick until ctx is cancelled.
func (s *LuckyBagService) RunScheduler(ctx context.Context) {
	t := time.NewTicker(luckyBagSchedulerTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.drawDue(ctx)
		}
	}
}

func (s *LuckyBagService) drawDue(ctx context.Context) {
	bags, err := s.orders.DueLuckyBags(ctx, luckyBagSchedulerBatch)
	if err != nil {
		logger.L().Warn("load due lucky bags", zap.Error(err))
		return
	}
	for i := range bags {
		bag := bags[i]
		present := s.presentUsers(ctx, bag.RoomID)
		packets := splitPackets(bag.TotalCoin, bag.Count, bag.AmountMode)
		if _, _, err := s.orders.DrawLuckyBag(ctx, bag.ID, present, packets, time.Now().UTC().UnixMilli()); err != nil {
			if !errors.Is(err, repo.ErrLuckyBagClosed) {
				logger.L().Warn("draw lucky bag", zap.String("bag_id", bag.ID), zap.Error(err))
			}
		}
	}
}

// presentUsers returns the set of userIds currently connected to the room,
// as recorded by im-gateway in Redis.
func (s *LuckyBagService) presentUsers(ctx context.Context, roomID string) map[string]bool {
	out := map[string]bool{}
	if s.rdb == nil {
		return out
	}
	members, err := s.rdb.SMembers(ctx, presenceKey(roomID)).Result()
	if err != nil && err != redis.Nil {
		logger.L().Debug("read room presence", zap.String("room", roomID), zap.Error(err))
		return out
	}
	for _, m := range members {
		if m = strings.TrimSpace(m); m != "" {
			out[m] = true
		}
	}
	return out
}

func presenceKey(roomID string) string { return "room:" + roomID + ":presence" }

func (s *LuckyBagService) checkEligibility(ctx context.Context, userID string, bag *model.LuckyBag) (bool, error) {
	switch bag.Eligibility {
	case model.LuckyBagEligibilityAll:
		return true, nil
	case model.LuckyBagEligibilityFollowers:
		return s.isFollowing(ctx, userID, bag.RoomID)
	case model.LuckyBagEligibilityFans:
		_, ok, err := s.orders.FanBadgeLevelFor(ctx, userID, bag.OwnerID)
		return ok, err
	case model.LuckyBagEligibilityFansLevel:
		level, ok, err := s.orders.FanBadgeLevelFor(ctx, userID, bag.OwnerID)
		if err != nil {
			return false, err
		}
		return ok && level >= bag.MinFanLevel, nil
	default:
		return true, nil
	}
}

func (s *LuckyBagService) isFollowing(ctx context.Context, userID, roomID string) (bool, error) {
	if s.rdb == nil || userID == "" {
		return false, nil
	}
	channelID, err := s.orders.RoomChannelID(ctx, roomID)
	if err != nil {
		return false, err
	}
	if channelID == "" {
		return false, nil
	}
	_, err = s.rdb.ZScore(ctx, "user:"+userID+":follows", channelID).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// splitPackets produces `count` packet amounts summing to total. Fixed mode
// gives equal packets (remainder on the first). Random mode uses the classic
// "double the average" red-packet algorithm so each packet is at least 1.
func splitPackets(total int64, count int, mode string) []int64 {
	if count <= 0 {
		return nil
	}
	packets := make([]int64, count)
	if total < int64(count) {
		// Degenerate guard: not enough to give everyone 1; fill what we can.
		for i := int64(0); i < total; i++ {
			packets[i] = 1
		}
		return packets
	}
	if mode == model.LuckyBagAmountFixed {
		base := total / int64(count)
		for i := range packets {
			packets[i] = base
		}
		packets[0] += total - base*int64(count)
		return packets
	}
	remaining := total
	for i := 0; i < count-1; i++ {
		peopleLeft := int64(count - i)
		// max keeps at least 1 coin for everyone still waiting.
		maxForThis := remaining - (peopleLeft - 1)
		avgTwice := (remaining / peopleLeft) * 2
		if avgTwice < 1 {
			avgTwice = 1
		}
		if maxForThis > avgTwice {
			maxForThis = avgTwice
		}
		if maxForThis < 1 {
			maxForThis = 1
		}
		amount := int64(1)
		if maxForThis > 1 {
			amount = 1 + rand.Int63n(maxForThis)
		}
		packets[i] = amount
		remaining -= amount
	}
	packets[count-1] = remaining
	return packets
}

func normalizeAmountMode(value string) string {
	if strings.ToLower(strings.TrimSpace(value)) == model.LuckyBagAmountRandom {
		return model.LuckyBagAmountRandom
	}
	return model.LuckyBagAmountFixed
}

func normalizeEligibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.LuckyBagEligibilityFollowers:
		return model.LuckyBagEligibilityFollowers
	case model.LuckyBagEligibilityFans:
		return model.LuckyBagEligibilityFans
	case model.LuckyBagEligibilityFansLevel:
		return model.LuckyBagEligibilityFansLevel
	default:
		return model.LuckyBagEligibilityAll
	}
}

func cleanLuckyBagMessage(value string) string {
	text := strings.TrimSpace(value)
	if len([]rune(text)) > MaxLuckyBagMessageRunes {
		text = string([]rune(text)[:MaxLuckyBagMessageRunes])
	}
	return text
}

func mapLuckyBagErr(err error) error {
	switch {
	case errors.Is(err, repo.ErrActiveLuckyBagExists):
		return ErrLuckyBagActive
	case errors.Is(err, repo.ErrLuckyBagNotFound):
		return ErrLuckyBagNotFound
	case errors.Is(err, repo.ErrLuckyBagClosed):
		return ErrLuckyBagClosed
	case errors.Is(err, repo.ErrLuckyBagAlreadyJoin):
		return ErrLuckyBagAlreadyJoin
	case errors.Is(err, repo.ErrLuckyBagUnauthorized):
		return ErrLuckyBagUnauthorized
	case errors.Is(err, repo.ErrInsufficientFunds):
		return ErrInsufficientCoin
	default:
		return err
	}
}
