package wallet_test

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/qingwenwen777/golive/pkg/wallet"
)

// mysqlTestDSNEnv names a server-level DSN such as
// root:root@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC. The MySQL variants
// of these tests skip when it is unset.
const mysqlTestDSNEnv = "GOLIVE_TEST_MYSQL_DSN"

type opener func(t *testing.T) *gorm.DB

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	db, err := gorm.Open(sqlite.Open("file:wallet_"+hex.EncodeToString(buf)+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// openMySQL creates a uniquely named database on the GOLIVE_TEST_MYSQL_DSN
// server and drops it on cleanup.
func openMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(mysqlTestDSNEnv))
	if dsn == "" {
		t.Skipf("%s not set; skipping MySQL-backed test", mysqlTestDSNEnv)
	}
	slash := strings.LastIndex(dsn, "/")
	require.GreaterOrEqual(t, slash, 0)
	prefix, params := dsn[:slash+1], ""
	if q := strings.IndexByte(dsn[slash+1:], '?'); q >= 0 {
		params = dsn[slash+1+q:]
	}
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	name := "golive_test_wallet_" + hex.EncodeToString(buf)

	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(mysql.Open(prefix+params), cfg)
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4").Error)
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error
		_ = adminSQL.Close()
	})
	db, err := gorm.Open(mysql.Open(prefix+name+params), cfg)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// forEachDB runs fn against SQLite and, when configured, MySQL.
func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	for _, tc := range []struct {
		name string
		open opener
	}{{"sqlite", openSQLite}, {"mysql", openMySQL}} {
		t.Run(tc.name, func(t *testing.T) {
			fn(t, tc.open(t))
		})
	}
}

type user struct {
	ID      string
	Balance int64
	Frozen  int64
	Banned  bool
}

func setup(t *testing.T, db *gorm.DB, users ...user) {
	t.Helper()
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id VARCHAR(36) PRIMARY KEY,
		coin_balance BIGINT NOT NULL DEFAULT 0,
		frozen_coins BIGINT NOT NULL DEFAULT 0,
		banned BOOLEAN NOT NULL DEFAULT false
	)`).Error)
	require.NoError(t, db.AutoMigrate(&wallet.CoinTransaction{}))
	for _, u := range users {
		require.NoError(t, db.Exec("INSERT INTO users (id, coin_balance, frozen_coins, banned) VALUES (?, ?, ?, ?)",
			u.ID, u.Balance, u.Frozen, u.Banned).Error)
	}
}

func balances(t *testing.T, db *gorm.DB, userID string) (int64, int64) {
	t.Helper()
	var balance, frozen int64
	require.NoError(t, db.Raw("SELECT coin_balance, frozen_coins FROM users WHERE id = ?", userID).Row().Scan(&balance, &frozen))
	return balance, frozen
}

func ledger(t *testing.T, db *gorm.DB, userID string) []wallet.CoinTransaction {
	t.Helper()
	var rows []wallet.CoinTransaction
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at ASC, id ASC").Find(&rows).Error)
	return rows
}

func inTx(db *gorm.DB, fn func(tx *gorm.DB) error) error { return db.Transaction(fn) }

var sampleEntry = wallet.Entry{
	Type:           "gift_spend",
	Title:          "Gift",
	Description:    "rose x3",
	SourceType:     "gift_order",
	SourceID:       "gift-1",
	RoomID:         "room-1",
	CounterpartyID: "creator",
}

func TestDebitAndCreditWriteBalanceAndLedger(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "fan", Balance: 100}, user{ID: "creator", Balance: 5})

		var debit, credit *wallet.CoinTransaction
		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			var err error
			if debit, err = wallet.Debit(tx, "fan", 30, sampleEntry); err != nil {
				return err
			}
			income := sampleEntry
			income.Type, income.CounterpartyID = "creator_gift_income", "fan"
			credit, err = wallet.Credit(tx, "creator", 30, income)
			return err
		}))

		balance, _ := balances(t, db, "fan")
		require.EqualValues(t, 70, balance)
		balance, _ = balances(t, db, "creator")
		require.EqualValues(t, 35, balance)

		rows := ledger(t, db, "fan")
		require.Len(t, rows, 1)
		require.Equal(t, debit.ID, rows[0].ID)
		require.NotEmpty(t, rows[0].ID)
		require.EqualValues(t, -30, rows[0].Amount)
		require.EqualValues(t, 70, rows[0].BalanceAfter)
		require.Equal(t, "gift_spend", rows[0].Type)
		require.Equal(t, "Gift", rows[0].Title)
		require.Equal(t, "rose x3", rows[0].Description)
		require.Equal(t, "gift_order", rows[0].SourceType)
		require.Equal(t, "gift-1", rows[0].SourceID)
		require.Equal(t, "room-1", rows[0].RoomID)
		require.Equal(t, "creator", rows[0].CounterpartyID)
		require.False(t, rows[0].CreatedAt.IsZero())

		rows = ledger(t, db, "creator")
		require.Len(t, rows, 1)
		require.Equal(t, credit.ID, rows[0].ID)
		require.EqualValues(t, 30, rows[0].Amount)
		require.EqualValues(t, 35, rows[0].BalanceAfter)
		require.Equal(t, "creator_gift_income", rows[0].Type)
		require.Equal(t, "fan", rows[0].CounterpartyID)
	})
}

func TestBalanceAfterFollowsEachCallInOneTransaction(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "host", Balance: 50})
		// A host gifting their own room is debited and credited in one tx.
		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			if _, err := wallet.Debit(tx, "host", 20, sampleEntry); err != nil {
				return err
			}
			_, err := wallet.Credit(tx, "host", 20, sampleEntry)
			return err
		}))
		rows := ledger(t, db, "host")
		require.Len(t, rows, 2)
		byAmount := map[int64]int64{rows[0].Amount: rows[0].BalanceAfter, rows[1].Amount: rows[1].BalanceAfter}
		require.EqualValues(t, 30, byAmount[-20])
		require.EqualValues(t, 50, byAmount[20])
	})
}

func TestDebitRefusals(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db,
			user{ID: "poor", Balance: 10},
			user{ID: "frozen", Balance: 100, Frozen: 80},
			user{ID: "banned", Balance: 100, Banned: true},
		)
		cases := []struct {
			userID string
			amount int64
		}{
			{"poor", 11},
			{"frozen", 21}, // only 20 available
			{"banned", 1},
			{"missing", 1},
		}
		for _, tc := range cases {
			err := inTx(db, func(tx *gorm.DB) error {
				_, err := wallet.Debit(tx, tc.userID, tc.amount, sampleEntry)
				return err
			})
			require.ErrorIs(t, err, wallet.ErrInsufficientFunds, tc.userID)
			require.Empty(t, ledger(t, db, tc.userID), tc.userID)
		}
		balance, _ := balances(t, db, "poor")
		require.EqualValues(t, 10, balance)

		// Exactly the available balance is allowed.
		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Debit(tx, "frozen", 20, sampleEntry)
			return err
		}))
		balance, frozen := balances(t, db, "frozen")
		require.EqualValues(t, 80, balance)
		require.EqualValues(t, 80, frozen)
	})
}

func TestDebitFallsBackWithoutUserControlColumns(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.Exec(`CREATE TABLE users (
			id VARCHAR(36) PRIMARY KEY,
			coin_balance BIGINT NOT NULL
		)`).Error)
		require.NoError(t, db.AutoMigrate(&wallet.CoinTransaction{}))
		require.NoError(t, db.Exec("INSERT INTO users (id, coin_balance) VALUES ('legacy', 10)").Error)

		err := inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Debit(tx, "legacy", 11, sampleEntry)
			return err
		})
		require.ErrorIs(t, err, wallet.ErrInsufficientFunds)
		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Debit(tx, "legacy", 10, sampleEntry)
			return err
		}))
		rows := ledger(t, db, "legacy")
		require.Len(t, rows, 1)
		require.EqualValues(t, 0, rows[0].BalanceAfter)
	})
}

func TestAdminDebitIgnoresBansAndFrozenCoins(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "u", Balance: 100, Frozen: 90, Banned: true})
		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.AdminDebit(tx, "u", 60, wallet.Entry{Type: "admin_adjust", Title: "Admin"})
			return err
		}))
		balance, frozen := balances(t, db, "u")
		require.EqualValues(t, 40, balance)
		require.EqualValues(t, 90, frozen)

		err := inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.AdminDebit(tx, "u", 41, wallet.Entry{Type: "admin_adjust", Title: "Admin"})
			return err
		})
		require.ErrorIs(t, err, wallet.ErrInsufficientFunds)
		rows := ledger(t, db, "u")
		require.Len(t, rows, 1)
		require.EqualValues(t, -60, rows[0].Amount)
		require.EqualValues(t, 40, rows[0].BalanceAfter)
	})
}

func TestCreditMissingUser(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db)
		err := inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Credit(tx, "missing", 5, sampleEntry)
			return err
		})
		require.ErrorIs(t, err, wallet.ErrUserNotFound)
		require.Empty(t, ledger(t, db, "missing"))
	})
}

func TestNonPositiveAmountsAreRejected(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "u", Balance: 100, Frozen: 10})
		ops := map[string]func(*gorm.DB, string, int64, wallet.Entry) (*wallet.CoinTransaction, error){
			"Debit":      wallet.Debit,
			"AdminDebit": wallet.AdminDebit,
			"Credit":     wallet.Credit,
			"Freeze":     wallet.Freeze,
			"Unfreeze":   wallet.Unfreeze,
		}
		for name, op := range ops {
			for _, amount := range []int64{0, -5} {
				err := inTx(db, func(tx *gorm.DB) error {
					_, err := op(tx, "u", amount, sampleEntry)
					return err
				})
				require.ErrorIs(t, err, wallet.ErrInvalidAmount, "%s(%d)", name, amount)
			}
		}
		balance, frozen := balances(t, db, "u")
		require.EqualValues(t, 100, balance)
		require.EqualValues(t, 10, frozen)
		require.Empty(t, ledger(t, db, "u"))
	})
}

func TestFreezeAndUnfreeze(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "u", Balance: 100, Frozen: 30})
		freeze := wallet.Entry{Type: "admin_freeze", Title: "Freeze"}
		unfreeze := wallet.Entry{Type: "admin_unfreeze", Title: "Unfreeze"}

		err := inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Freeze(tx, "u", 71, freeze)
			return err
		})
		require.ErrorIs(t, err, wallet.ErrInsufficientFunds)

		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Freeze(tx, "u", 70, freeze)
			return err
		}))
		balance, frozen := balances(t, db, "u")
		require.EqualValues(t, 100, balance)
		require.EqualValues(t, 100, frozen)

		require.NoError(t, inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Unfreeze(tx, "u", 40, unfreeze)
			return err
		}))
		_, frozen = balances(t, db, "u")
		require.EqualValues(t, 60, frozen)

		// Unfreezing more than is frozen clamps at zero, and unfreezing an
		// already-zero amount (0 rows changed on MySQL) still succeeds.
		for i := 0; i < 2; i++ {
			require.NoError(t, inTx(db, func(tx *gorm.DB) error {
				_, err := wallet.Unfreeze(tx, "u", 500, unfreeze)
				return err
			}))
		}
		balance, frozen = balances(t, db, "u")
		require.EqualValues(t, 100, balance)
		require.EqualValues(t, 0, frozen)

		rows := ledger(t, db, "u")
		require.Len(t, rows, 4)
		for _, row := range rows {
			require.EqualValues(t, 0, row.Amount)
			require.EqualValues(t, 100, row.BalanceAfter)
		}

		err = inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Unfreeze(tx, "missing", 1, unfreeze)
			return err
		})
		require.ErrorIs(t, err, wallet.ErrUserNotFound)
		err = inTx(db, func(tx *gorm.DB) error {
			_, err := wallet.Freeze(tx, "missing", 1, freeze)
			return err
		})
		require.ErrorIs(t, err, wallet.ErrInsufficientFunds)
	})
}

func TestCallerSuppliedIDRejectsDuplicates(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "u", Balance: 0})
		entry := wallet.Entry{ID: "daily_abc", Type: "daily_task", Title: "Daily"}
		credit := func() error {
			return inTx(db, func(tx *gorm.DB) error {
				_, err := wallet.Credit(tx, "u", 10, entry)
				return err
			})
		}
		require.NoError(t, credit())
		require.Error(t, credit())

		balance, _ := balances(t, db, "u")
		require.EqualValues(t, 10, balance, "the duplicate's credit must roll back with its ledger row")
		rows := ledger(t, db, "u")
		require.Len(t, rows, 1)
		require.Equal(t, "daily_abc", rows[0].ID)
	})
}

func TestCallerRollbackUndoesBalanceAndLedger(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		setup(t, db, user{ID: "fan", Balance: 100}, user{ID: "creator"})
		boom := errors.New("order insert failed")
		err := inTx(db, func(tx *gorm.DB) error {
			if _, err := wallet.Debit(tx, "fan", 40, sampleEntry); err != nil {
				return err
			}
			if _, err := wallet.Credit(tx, "creator", 40, sampleEntry); err != nil {
				return err
			}
			return boom
		})
		require.ErrorIs(t, err, boom)
		balance, _ := balances(t, db, "fan")
		require.EqualValues(t, 100, balance)
		balance, _ = balances(t, db, "creator")
		require.EqualValues(t, 0, balance)
		require.Empty(t, ledger(t, db, "fan"))
		require.Empty(t, ledger(t, db, "creator"))
	})
}
