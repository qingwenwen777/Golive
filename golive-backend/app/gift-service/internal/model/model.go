package model

import "time"

// Gift is the catalog entry. JSON tags match src/types/gift.ts.
type Gift struct {
	ID        string `gorm:"primaryKey;type:varchar(64)"      json:"id"`
	Name      string `gorm:"type:varchar(64);not null"        json:"name"`
	NameJa    string `gorm:"type:varchar(64)"                 json:"nameJa,omitempty"`
	Icon      string `gorm:"type:varchar(255);not null"       json:"icon"`
	PriceCoin int64  `gorm:"not null"                         json:"priceCoin"`
	Category  string `gorm:"type:varchar(16);not null"        json:"category"`
	Animation string `gorm:"type:varchar(16)"                 json:"animation,omitempty"`
	Tier      int    `gorm:"not null;default:0"               json:"tier"`
}

func (Gift) TableName() string { return "gifts" }

// Order statuses.
const (
	StatusPending = "pending"
	StatusSuccess = "success"
	StatusFailed  = "failed"

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
}

func (SuperChatOrder) TableName() string { return "super_chat_orders" }

// Outbox / "local message" topics.
const (
	OutboxTopicGift      = "gift"
	OutboxTopicSuperChat = "super_chat"

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
