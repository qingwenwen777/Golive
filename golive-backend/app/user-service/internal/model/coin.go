package model

import "github.com/qingwenwen777/golive/pkg/wallet"

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
)

// CoinTransaction is a row of the shared coin_transactions ledger. It is
// written only through pkg/wallet.
type CoinTransaction = wallet.CoinTransaction
