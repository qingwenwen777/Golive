// Package wallet is the only writer of the coin wallet in the shared MySQL
// database: users.coin_balance, users.frozen_coins and the coin_transactions
// ledger. Every service that moves coins calls it instead of issuing its own
// UPDATE / INSERT, so the balance rules and the ledger format live in one
// place.
//
// Every function works inside the caller's GORM transaction (`tx`), so a
// balance change commits or rolls back together with the rest of the
// caller's writes (order, outbox row, ...). Each call changes one user's
// balance and appends exactly one ledger row whose BalanceAfter is read back
// inside the same transaction.
//
// The SQL is kept dialect-neutral so the SQLite-backed tests exercise the
// same statements as MySQL.
package wallet

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrInsufficientFunds means the debit or freeze was refused: the balance
	// is short, or (for Debit) the account is banned or does not exist. The
	// single conditional UPDATE cannot tell these apart.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrInvalidAmount rejects zero or negative amounts, which would turn a
	// debit into a credit (or a credit into a debit).
	ErrInvalidAmount = errors.New("amount must be positive")
	// ErrUserNotFound means no users row matched userID.
	ErrUserNotFound = errors.New("wallet user not found")
)

// CoinTransaction is one row of the coin_transactions ledger. Amount is
// signed (negative for debits, zero for freeze/unfreeze) and BalanceAfter is
// users.coin_balance right after the change.
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

// Entry describes the ledger row written for a balance change. The wallet
// fills in UserID, Amount and BalanceAfter.
type Entry struct {
	// ID is the ledger row's primary key. Leave it empty for a random UUID;
	// pass a deterministic ID to make a write idempotent, since a second
	// write with the same ID fails with a duplicate-key error.
	ID             string
	Type           string
	Title          string
	Description    string
	SourceType     string
	SourceID       string
	RoomID         string
	CounterpartyID string
}

// Debit spends amount from an active account's available balance
// (coin_balance - frozen_coins). It returns ErrInsufficientFunds if the
// available balance is short, the user is banned or the user does not exist.
func Debit(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	res := tx.Exec(
		"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND COALESCE(banned, false) = false AND coin_balance - COALESCE(frozen_coins, 0) >= ?",
		amount, userID, amount,
	)
	if isMissingColumn(res.Error) {
		// Older users tables (and minimal test schemas) have no banned /
		// frozen_coins columns; fall back to the plain balance check.
		res = tx.Exec(
			"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND coin_balance >= ?",
			amount, userID, amount,
		)
	}
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrInsufficientFunds
	}
	return appendEntry(tx, userID, -amount, e)
}

// AdminDebit removes amount for an operator correction. Unlike Debit it
// ignores bans and frozen coins and only requires coin_balance >= amount.
// It returns ErrInsufficientFunds if the balance is short or the user does
// not exist.
func AdminDebit(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	res := tx.Exec(
		"UPDATE users SET coin_balance = coin_balance - ? WHERE id = ? AND coin_balance >= ?",
		amount, userID, amount,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrInsufficientFunds
	}
	return appendEntry(tx, userID, -amount, e)
}

// Credit adds amount to the user's balance. It does not look at bans or
// frozen coins. It returns ErrUserNotFound if the user does not exist.
func Credit(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	res := tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", amount, userID)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrUserNotFound
	}
	return appendEntry(tx, userID, amount, e)
}

// Freeze moves amount of the available balance into frozen_coins, which
// Debit cannot spend. coin_balance is unchanged and the ledger row has
// Amount 0. It returns ErrInsufficientFunds if the available balance is
// short or the user does not exist.
func Freeze(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	res := tx.Exec(
		"UPDATE users SET frozen_coins = COALESCE(frozen_coins, 0) + ? WHERE id = ? AND coin_balance - COALESCE(frozen_coins, 0) >= ?",
		amount, userID, amount,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrInsufficientFunds
	}
	return appendEntry(tx, userID, 0, e)
}

// Unfreeze releases up to amount of frozen_coins, clamping at zero.
// coin_balance is unchanged and the ledger row has Amount 0. It returns
// ErrUserNotFound if the user does not exist.
func Unfreeze(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	// RowsAffected is not checked: MySQL reports 0 when frozen_coins was
	// already 0, so the read-back in appendEntry detects a missing user.
	if err := tx.Exec(
		"UPDATE users SET frozen_coins = CASE WHEN COALESCE(frozen_coins, 0) >= ? THEN COALESCE(frozen_coins, 0) - ? ELSE 0 END WHERE id = ?",
		amount, amount, userID,
	).Error; err != nil {
		return nil, err
	}
	return appendEntry(tx, userID, 0, e)
}

// appendEntry reads the user's balance back and writes the ledger row.
func appendEntry(tx *gorm.DB, userID string, amount int64, e Entry) (*CoinTransaction, error) {
	var balanceAfter int64
	err := tx.Raw("SELECT coin_balance FROM users WHERE id = ?", userID).Row().Scan(&balanceAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	id := e.ID
	if id == "" {
		id = uuid.NewString()
	}
	row := &CoinTransaction{
		ID:             id,
		UserID:         userID,
		Type:           e.Type,
		Amount:         amount,
		BalanceAfter:   balanceAfter,
		Title:          e.Title,
		Description:    e.Description,
		SourceType:     e.SourceType,
		SourceID:       e.SourceID,
		RoomID:         e.RoomID,
		CounterpartyID: e.CounterpartyID,
	}
	if err := tx.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func isMissingColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown column") || strings.Contains(msg, "no such column")
}
