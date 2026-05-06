package model

import "time"

const (
	CoinTxTopup                  = "topup"
	CoinTxDailyTask              = "daily_task"
	CoinTxGiftSpend              = "gift_spend"
	CoinTxSuperChatSpend         = "super_chat_spend"
	CoinTxBetWager               = "bet_wager"
	CoinTxBetPayout              = "bet_payout"
	CoinTxBetRefund              = "bet_refund"
	CoinTxCreatorGiftIncome      = "creator_gift_income"
	CoinTxCreatorSuperChatIncome = "creator_super_chat_income"
	CoinTxWithdrawal             = "withdrawal"
	CoinTxAdminAdjust            = "admin_adjust"
	CoinTxAdminFreeze            = "admin_freeze"
	CoinTxAdminUnfreeze          = "admin_unfreeze"

	WithdrawalStatusPending   = "pending"
	WithdrawalStatusSucceeded = "succeeded"
	WithdrawalStatusFailed    = "failed"
)

type CoinTransaction struct {
	ID             string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	UserID         string    `gorm:"type:varchar(36);index;not null" json:"userId"`
	Type           string    `gorm:"type:varchar(40);index;not null" json:"type"`
	Amount         int64     `gorm:"not null" json:"amount"`
	BalanceAfter   int64     `gorm:"not null" json:"balanceAfter"`
	Title          string    `gorm:"type:varchar(80);not null" json:"title"`
	Description    string    `gorm:"type:varchar(255)" json:"description,omitempty"`
	SourceType     string    `gorm:"type:varchar(40);index" json:"sourceType,omitempty"`
	SourceID       string    `gorm:"type:varchar(80);index" json:"sourceId,omitempty"`
	RoomID         string    `gorm:"type:varchar(64);index" json:"roomId,omitempty"`
	CounterpartyID string    `gorm:"type:varchar(36);index" json:"counterpartyId,omitempty"`
	CreatedAt      time.Time `gorm:"index" json:"createdAt"`
}

func (CoinTransaction) TableName() string { return "coin_transactions" }

type CoinWithdrawal struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	UserID           string    `gorm:"type:varchar(36);index;not null" json:"userId"`
	Amount           int64     `gorm:"not null" json:"amount"`
	Fee              int64     `gorm:"not null" json:"fee"`
	NetCoins         int64     `gorm:"not null" json:"netCoins"`
	Currency         string    `gorm:"type:varchar(8);not null" json:"currency"`
	StripeAccountID  string    `gorm:"type:varchar(64);index;not null" json:"stripeAccountId,omitempty"`
	StripeTransferID string    `gorm:"type:varchar(80);uniqueIndex" json:"stripeTransferId,omitempty"`
	Status           string    `gorm:"type:varchar(24);index;not null" json:"status"`
	ErrorMessage     string    `gorm:"type:varchar(500)" json:"errorMessage,omitempty"`
	CreatedAt        time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (CoinWithdrawal) TableName() string { return "coin_withdrawals" }
