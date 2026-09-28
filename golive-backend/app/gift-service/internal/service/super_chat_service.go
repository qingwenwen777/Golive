package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
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

// SuperChatMaxText mirrors src/types/gift.ts SC_MAX_TEXT_BY_TIER: how many
// characters (runes, after trimming surrounding whitespace) a super chat of
// the tier may carry.
func SuperChatMaxText(tier int) int {
	switch tier {
	case 1:
		return 50
	case 2:
		return 100
	case 3:
		return 150
	case 4, 5:
		return 200
	default:
		return 0
	}
}

// ErrInvalidAmount is returned when amount maps to tier 0. Tier 0 super
// chats are not allowed (the frontend won't send them but we double-check).
var ErrInvalidAmount = errors.New("amount below minimum tier")
var ErrContentBlocked = errors.New("content contains blocked word")
var ErrUserRestricted = errors.New("user is restricted from interactions")

// ErrSuperChatTextTooLong is returned when the text exceeds SuperChatMaxText
// for the amount's tier.
var ErrSuperChatTextTooLong = errors.New("super chat text too long")

type SuperChatService struct {
	orders *repo.OrderRepo
	rdb    *redis.Client
}

func NewSuperChatService(o *repo.OrderRepo, rdb ...*redis.Client) *SuperChatService {
	var client *redis.Client
	if len(rdb) > 0 {
		client = rdb[0]
	}
	return &SuperChatService{orders: o, rdb: client}
}

func (s *SuperChatService) Send(ctx context.Context, req SendSuperChatReq) (*model.SuperChatOrder, bool, error) {
	tier := AmountToTier(req.Amount)
	if tier == 0 {
		return nil, false, ErrInvalidAmount
	}
	// The text is stored in the order row, both ledger descriptions and the
	// broadcast; the largest tier limit fits the narrowest of those columns
	// (coin_transactions.description, varchar(255)).
	text := strings.TrimSpace(req.Text)
	if utf8.RuneCountInString(text) > SuperChatMaxText(tier) {
		return nil, false, ErrSuperChatTextTooLong
	}
	if restricted, err := s.userRestricted(ctx, req.UserID); err != nil {
		return nil, false, err
	} else if restricted {
		return nil, false, ErrUserRestricted
	}
	if blocked, err := s.containsBlockedWord(ctx, text); err != nil {
		return nil, false, err
	} else if blocked {
		return nil, false, ErrContentBlocked
	}

	now := time.Now().UTC()
	orderID := "sc-" + uuid.NewString()
	username := s.broadcastName(ctx, req)
	avatar, _ := s.orders.AvatarForUser(ctx, req.UserID)
	userLevel, err := s.orders.UserLevel(ctx, req.UserID)
	if err != nil {
		return nil, false, fmt.Errorf("user level: %w", err)
	}

	payload, err := repo.MarshalSuperChatOutbox(orderID, req.UserID, username, avatar,
		strconv.FormatInt(req.Amount, 10), tier, userLevel, text, now.UnixMilli())
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
		Text:      text,
		Status:    model.StatusSuccess,
		CreatedAt: now,
	}

	placed, replayed, err := s.orders.PlaceSuperChatOrder(ctx, order, payload, repo.FanBadgeIfExists)
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
		Text:       text,
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

func (s *SuperChatService) userRestricted(ctx context.Context, userID string) (bool, error) {
	if s == nil || s.rdb == nil || strings.TrimSpace(userID) == "" {
		return false, nil
	}
	pipe := s.rdb.Pipeline()
	banCmd := pipe.Exists(ctx, contentpolicy.RedisSiteBanPrefix+userID)
	muteCmd := pipe.TTL(ctx, contentpolicy.RedisSiteMutePrefix+userID)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return false, err
	}
	banned, _ := banCmd.Result()
	mutedTTL, _ := muteCmd.Result()
	return banned > 0 || mutedTTL > 0, nil
}

func (s *SuperChatService) containsBlockedWord(ctx context.Context, text string) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, nil
	}
	words, err := s.rdb.SMembers(ctx, contentpolicy.RedisBlockedWordsKey).Result()
	if err != nil && err != redis.Nil {
		return false, err
	}
	return contentpolicy.Contains(text, words), nil
}

func (s *SuperChatService) broadcastName(ctx context.Context, req SendSuperChatReq) string {
	if name, err := s.orders.DisplayNameForUser(ctx, req.UserID); err == nil && name != "" {
		return cleanBroadcastName(name)
	}
	return cleanBroadcastName(req.Username)
}

var uuidLike = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// cleanBroadcastName is the sender name broadcast with a gift or Super Chat,
// or "" when there is no usable one (a bare uuid is not a name); the apps
// then label the sender in the viewer's language.
func cleanBroadcastName(name string) string {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 64 || strings.ContainsAny(name, "\r\n\t") || uuidLike.MatchString(name) {
		return ""
	}
	return name
}
