package model

import "time"

// Gift is the catalog entry. JSON tags match src/types/gift.ts.
type Gift struct {
	ID          string `gorm:"primaryKey;type:varchar(64)"      json:"id"`
	Name        string `gorm:"type:varchar(64);not null"        json:"name"`
	NameJa      string `gorm:"type:varchar(64)"                 json:"nameJa,omitempty"`
	Icon        string `gorm:"type:varchar(255);not null"       json:"icon"`
	PriceCoin   int64  `gorm:"not null"                         json:"priceCoin"`
	Category    string `gorm:"type:varchar(16);not null"        json:"category"`
	Animation   string `gorm:"type:varchar(16)"                 json:"animation,omitempty"`
	Tier        int    `gorm:"not null;default:0"               json:"tier"`
	UnlockLevel int    `gorm:"not null;default:1"               json:"unlockLevel"`
	Enabled     bool   `gorm:"not null;default:true;index"      json:"enabled"`
}

func (Gift) TableName() string { return "gifts" }

// Order statuses.
const (
	StatusPending  = "pending"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusLocked   = "locked"
	StatusWon      = "won"
	StatusLost     = "lost"
	StatusRefunded = "refunded"

	FailInsufficientCoin = "insufficient_coin"
)

// GiftOrder mirrors src/types/gift.ts GiftOrder. UserID/RoomID are server-only.
type GiftOrder struct {
	OrderID    string    `gorm:"primaryKey;type:varchar(64)"      json:"orderId"`
	RequestID  string    `gorm:"uniqueIndex;type:varchar(64);not null" json:"requestId"`
	UserID     string    `gorm:"type:varchar(36);index;not null"  json:"-"`
	RoomID     string    `gorm:"type:varchar(64);index;not null"  json:"-"`
	GiftID     string    `gorm:"type:varchar(64);not null"        json:"giftId"`
	Count      int       `gorm:"not null"                         json:"count"`
	TotalCoin  int64     `gorm:"not null"                         json:"totalCoin"`
	Status     string    `gorm:"type:varchar(16);not null"        json:"status"`
	FailReason string    `gorm:"type:varchar(32)"                 json:"failReason,omitempty"`
	CreatedAt  time.Time `                                        json:"createdAt"`
}

func (GiftOrder) TableName() string { return "gift_orders" }

// SuperChatOrder mirrors src/types/gift.ts SuperChatOrder.
type SuperChatOrder struct {
	OrderID    string    `gorm:"primaryKey;type:varchar(64)"      json:"orderId"`
	RequestID  string    `gorm:"uniqueIndex;type:varchar(64);not null" json:"requestId"`
	UserID     string    `gorm:"type:varchar(36);index;not null"  json:"-"`
	RoomID     string    `gorm:"type:varchar(64);index;not null"  json:"-"`
	Amount     int64     `gorm:"not null"                         json:"amount"`
	Tier       int       `gorm:"not null"                         json:"tier"`
	Text       string    `gorm:"type:varchar(500)"                json:"text"`
	Status     string    `gorm:"type:varchar(16);not null"        json:"status"`
	FailReason string    `gorm:"type:varchar(32)"                 json:"failReason,omitempty"`
	CreatedAt  time.Time `                                        json:"createdAt"`

	// ModeratedAt marks a super chat removed by moderation. It only hides
	// the message from public history; Status keeps the financial outcome,
	// so a paid super chat stays "success" in the ledger and revenue.
	ModeratedAt *time.Time `gorm:"index"            json:"moderatedAt,omitempty"`
	ModeratedBy string     `gorm:"type:varchar(36)" json:"-"`
}

func (SuperChatOrder) TableName() string { return "super_chat_orders" }

const (
	BetOptionWin  = "win"
	BetOptionLose = "lose"

	BetRoundOpen      = "open"
	BetRoundClosed    = "closed"
	BetRoundSettled   = "settled"
	BetRoundCancelled = "cancelled"
)

type BetRound struct {
	ID            string     `gorm:"primaryKey;type:varchar(64)" json:"id"`
	RoomID        string     `gorm:"type:varchar(64);index;not null" json:"roomId"`
	OwnerID       string     `gorm:"type:varchar(36);index;not null" json:"ownerId"`
	Question      string     `gorm:"type:varchar(255);not null" json:"question"`
	Amount        int64      `gorm:"not null" json:"amount"`
	Status        string     `gorm:"type:varchar(16);index;not null" json:"status"`
	WinningOption string     `gorm:"type:varchar(16)" json:"winningOption,omitempty"`
	CloseAt       time.Time  `gorm:"index;not null" json:"closeAt"`
	SettledAt     *time.Time `json:"settledAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (BetRound) TableName() string { return "bet_rounds" }

type BetWager struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	RoundID   string    `gorm:"type:varchar(64);index;not null;uniqueIndex:idx_bet_round_user" json:"roundId"`
	RoomID    string    `gorm:"type:varchar(64);index;not null" json:"roomId"`
	UserID    string    `gorm:"type:varchar(36);index;not null;uniqueIndex:idx_bet_round_user" json:"-"`
	Option    string    `gorm:"type:varchar(16);index;not null" json:"option"`
	Amount    int64     `gorm:"not null" json:"amount"`
	Payout    int64     `gorm:"not null;default:0" json:"payout"`
	Status    string    `gorm:"type:varchar(16);index;not null" json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (BetWager) TableName() string { return "bet_wagers" }

// FanBadge is a viewer's creator-specific fan plate. It is awarded by the
// Fan Light gift, then leveled by that viewer's lifetime contribution to the
// same creator.
type FanBadge struct {
	UserID            string    `gorm:"primaryKey;type:varchar(36);column:user_id" json:"userId"`
	CreatorID         string    `gorm:"primaryKey;type:varchar(36);column:creator_id" json:"creatorId"`
	CreatorName       string    `gorm:"type:varchar(64);not null" json:"creatorName"`
	CreatorAvatar     string    `gorm:"type:varchar(500)" json:"creatorAvatar,omitempty"`
	TotalContribution int64     `gorm:"not null;default:0" json:"totalContribution"`
	Level             int       `gorm:"not null;default:1" json:"level"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (FanBadge) TableName() string { return "fan_badges" }

type FanClubMember struct {
	UserID            string    `json:"userId"`
	Username          string    `json:"username,omitempty"`
	Name              string    `json:"name"`
	Avatar            string    `json:"avatar,omitempty"`
	TotalContribution int64     `json:"totalContribution"`
	Level             int       `json:"level"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type FanClubMembersResponse struct {
	Items []FanClubMember `json:"items"`
	Total int64           `json:"total"`
}

// Outbox / "local message" topics.
const (
	OutboxTopicGift      = "gift"
	OutboxTopicSuperChat = "super_chat"
	OutboxTopicBet       = "bet"
	OutboxTopicLuckyBag  = "lucky_bag"

	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusDead    = "dead"
)

// LocalMessage is the transactional outbox row. Payload is the broadcast-
// ready JSON we'll publish to Kafka (and downstream to Redis room:<id>).
type LocalMessage struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	BizID     string    `gorm:"type:varchar(64);index;not null"`
	RoomID    string    `gorm:"type:varchar(64);index"`
	Topic     string    `gorm:"type:varchar(32);index;not null"`
	Payload   string    `gorm:"type:text;not null"`
	Status    string    `gorm:"type:varchar(16);index;not null;default:'pending'"`
	Retries   int       `gorm:"not null;default:0"`
	NextAt    time.Time `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (LocalMessage) TableName() string { return "local_messages" }
