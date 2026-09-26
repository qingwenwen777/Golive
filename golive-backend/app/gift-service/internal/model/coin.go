package model

import "github.com/qingwenwen777/golive/pkg/wallet"

const (
	CoinTxTopup                  = "topup"
	CoinTxGiftSpend              = "gift_spend"
	CoinTxSuperChatSpend         = "super_chat_spend"
	CoinTxBetWager               = "bet_wager"
	CoinTxBetPayout              = "bet_payout"
	CoinTxBetRefund              = "bet_refund"
	CoinTxLuckyBagSend           = "lucky_bag_send"
	CoinTxLuckyBagPayout         = "lucky_bag_payout"
	CoinTxLuckyBagRefund         = "lucky_bag_refund"
	CoinTxCreatorGiftIncome      = "creator_gift_income"
	CoinTxCreatorSuperChatIncome = "creator_super_chat_income"
)

// CoinTransaction is a row of the shared coin_transactions ledger. It is
// written only through pkg/wallet.
type CoinTransaction = wallet.CoinTransaction
