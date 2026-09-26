package model

import "time"

const (
	LuckyBagAmountFixed  = "fixed"
	LuckyBagAmountRandom = "random"

	LuckyBagEligibilityAll       = "all"
	LuckyBagEligibilityFollowers = "followers"
	LuckyBagEligibilityFans      = "fans"
	LuckyBagEligibilityFansLevel = "fans_level"

	LuckyBagOpen      = "open"
	LuckyBagDrawn     = "drawn"
	LuckyBagCancelled = "cancelled"

	LuckyBagEntryJoined = "joined"
	LuckyBagEntryWon    = "won"
	LuckyBagEntryMissed = "missed"
)

// LuckyBag is a streamer-funded giveaway: total_coin is escrowed from the
// owner when opened, split into `count` packets, and drawn among viewers who
// joined and are present in the room at close_at.
type LuckyBag struct {
	ID          string     `gorm:"primaryKey;type:varchar(64)" json:"id"`
	RoomID      string     `gorm:"type:varchar(64);index;not null" json:"roomId"`
	OwnerID     string     `gorm:"type:varchar(36);index;not null" json:"ownerId"`
	Message     string     `gorm:"type:varchar(255)" json:"message,omitempty"`
	TotalCoin   int64      `gorm:"not null" json:"totalCoin"`
	Count       int        `gorm:"not null" json:"count"`
	AmountMode  string     `gorm:"type:varchar(16);not null" json:"amountMode"`
	Eligibility string     `gorm:"type:varchar(16);not null" json:"eligibility"`
	MinFanLevel int        `gorm:"not null;default:0" json:"minFanLevel"`
	Status      string     `gorm:"type:varchar(16);index;not null" json:"status"`
	CloseAt     time.Time  `gorm:"index;not null" json:"closeAt"`
	DrawnAt     *time.Time `json:"drawnAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (LuckyBag) TableName() string { return "lucky_bags" }

type LuckyBagEntry struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	BagID     string    `gorm:"type:varchar(64);index;not null;uniqueIndex:idx_lucky_bag_user" json:"bagId"`
	RoomID    string    `gorm:"type:varchar(64);index;not null" json:"roomId"`
	UserID    string    `gorm:"type:varchar(36);index;not null;uniqueIndex:idx_lucky_bag_user" json:"-"`
	Status    string    `gorm:"type:varchar(16);index;not null" json:"status"`
	Payout    int64     `gorm:"not null;default:0" json:"payout"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (LuckyBagEntry) TableName() string { return "lucky_bag_entries" }
