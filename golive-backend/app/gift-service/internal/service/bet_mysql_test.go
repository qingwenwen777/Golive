package service_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

// The tests in this file need real row locks, which the single-connection
// SQLite setup cannot exercise. They run only when GOLIVE_TEST_MYSQL_DSN
// points at a MySQL server (no database in the path, CREATE/DROP DATABASE
// privileges), e.g. root:root@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC.
const mysqlTestDSNEnv = "GOLIVE_TEST_MYSQL_DSN"

// newMySQLTestDB creates a uniquely named database for the calling test,
// creates the minimal users/rooms tables plus the gift-service schema, and
// drops the database on cleanup. It returns the connection and database name.
func newMySQLTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(mysqlTestDSNEnv))
	if dsn == "" {
		t.Skipf("%s not set; skipping MySQL-backed test", mysqlTestDSNEnv)
	}
	cfg := &gorm.Config{Logger: glogger.Default.LogMode(glogger.Silent)}
	admin, err := gorm.Open(mysql.Open(dsn), cfg)
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)

	suffix := make([]byte, 6)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	name := "golive_test_giftsvc_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4").Error)
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error
		_ = adminSQL.Close()
	})

	db, err := gorm.Open(mysql.Open(mysqlDSNWithDB(dsn, name)), cfg)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// Registered after the DROP so it runs first (cleanups are LIFO).
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.Exec(`CREATE TABLE users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(64) NOT NULL DEFAULT '',
		display_name VARCHAR(64) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		coin_balance BIGINT NOT NULL DEFAULT 0,
		frozen_coins BIGINT NOT NULL DEFAULT 0,
		banned TINYINT(1) NOT NULL DEFAULT 0
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rooms (
		id VARCHAR(64) PRIMARY KEY,
		channel VARCHAR(128) NOT NULL DEFAULT '',
		avatar VARCHAR(500) NOT NULL DEFAULT '',
		owner_id VARCHAR(36) NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, repo.NewOrderRepo(db).AutoMigrate())
	require.NoError(t, db.Exec("INSERT INTO rooms (id, owner_id) VALUES ('r1', 'u-owner')").Error)
	return db, name
}

// mysqlDSNWithDB swaps the database name into a go-sql-driver DSN of the form
// [user[:pass]@][net[(addr)]]/[dbname][?params].
func mysqlDSNWithDB(dsn, dbName string) string {
	params := ""
	if q := strings.IndexByte(dsn, '?'); q >= 0 {
		dsn, params = dsn[:q], dsn[q:]
	}
	if slash := strings.LastIndexByte(dsn, '/'); slash >= 0 {
		dsn = dsn[:slash]
	}
	return dsn + "/" + dbName + params
}

func insertMySQLUser(t *testing.T, db *gorm.DB, id string, balance int64) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO users (id, username, coin_balance) VALUES (?, ?, ?)", id, id, balance).Error)
}

// waitDoneOrLockWait returns once done is closed or a session on dbName is
// blocked waiting for an InnoDB row lock. Either way the interleaving under
// test has progressed as far as it can while the other transaction is held.
func waitDoneOrLockWait(t *testing.T, db *gorm.DB, dbName string, done <-chan struct{}) {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-done:
			return
		case <-deadline:
			t.Fatal("timed out waiting for the concurrent transaction to finish or block")
		// InnoDB only refreshes the innodb_trx snapshot when it has not been
		// read for 100ms, so polling faster would keep seeing stale rows.
		case <-time.After(150 * time.Millisecond):
		}
		var waiting int64
		require.NoError(t, db.Raw(`
SELECT COUNT(*)
FROM information_schema.innodb_trx t
JOIN information_schema.processlist p ON p.id = t.trx_mysql_thread_id
WHERE t.trx_state = 'LOCK WAIT' AND p.db = ?`, dbName).Scan(&waiting).Error)
		if waiting > 0 {
			return
		}
	}
}

// closeOnce returns an idempotent close of ch, so a failing test can still
// unblock its paused transaction from t.Cleanup before the database is dropped.
func closeOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// requireNoOrphanedWagers asserts the money invariant the settle/cancel races
// used to break: once a round is terminal, none of its wagers is still locked.
func requireNoOrphanedWagers(t *testing.T, db *gorm.DB, roundID string) {
	t.Helper()
	var round model.BetRound
	require.NoError(t, db.Where("id = ?", roundID).Take(&round).Error)
	require.Contains(t, []string{model.BetRoundSettled, model.BetRoundCancelled}, round.Status)
	var locked int64
	require.NoError(t, db.Model(&model.BetWager{}).
		Where("round_id = ? AND status = ?", roundID, model.StatusLocked).
		Count(&locked).Error)
	require.Zero(t, locked, "wagers left locked on a %s round", round.Status)
}

// A wager that has read the round as open and debited the stake must not be
// able to commit after the host's settle, or it is orphaned in `locked`.
func TestMySQLBet_SettleDoesNotOrphanInflightWager(t *testing.T) {
	db, dbName := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-c", 1000)
	insertMySQLUser(t, db, "u-w", 1000)
	orders := repo.NewOrderRepo(db)
	svc := service.NewBetService(orders)
	ctx := context.Background()

	view, err := svc.Open(ctx, "u-owner", "r1", 100, "Will blue win?")
	require.NoError(t, err)
	roundID := view.Round.ID
	_, err = svc.Wager(ctx, "u-c", "r1", roundID, model.BetOptionWin)
	require.NoError(t, err)
	// The host may only settle once betting has closed, so shorten the window:
	// u-w's wager below checks the round before close_at, settle runs after.
	closeAt := time.Now().UTC().Add(2 * time.Second)
	require.NoError(t, db.Model(&model.BetRound{}).Where("id = ?", roundID).
		Update("close_at", closeAt).Error)

	// Pause u-w's wager right before it inserts the bet_wagers row, i.e.
	// after it has checked the round and debited the stake.
	var armed atomic.Bool
	armed.Store(true)
	reached := make(chan struct{})
	release := make(chan struct{})
	unblock := closeOnce(release)
	t.Cleanup(unblock)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:pause_wager", func(tx *gorm.DB) {
		if tx.Statement.Table == "bet_wagers" && armed.CompareAndSwap(true, false) {
			close(reached)
			<-release
		}
	}))
	wagerErr := make(chan error, 1)
	go func() {
		_, err := svc.Wager(ctx, "u-w", "r1", roundID, model.BetOptionLose)
		wagerErr <- err
	}()
	<-reached
	time.Sleep(time.Until(closeAt) + 50*time.Millisecond)

	settleDone := make(chan struct{})
	var settleErr error
	go func() {
		defer close(settleDone)
		_, _, settleErr = orders.SettleBetRound(ctx, roundID, "u-owner", model.BetOptionWin, []byte("{}"))
	}()
	waitDoneOrLockWait(t, db, dbName, settleDone)
	unblock()
	<-settleDone
	require.NoError(t, settleErr)
	werr := <-wagerErr

	requireNoOrphanedWagers(t, db, roundID)
	var total int64
	require.NoError(t, db.Raw("SELECT COALESCE(SUM(coin_balance), 0) FROM users").Scan(&total).Error)
	require.Equal(t, int64(2000), total, "coins must be conserved across the settle")
	if werr == nil {
		// The wager committed first, so settle must have paid it out as a loser.
		require.Equal(t, int64(900), balanceOf(t, db, "u-w"))
		require.Equal(t, int64(1100), balanceOf(t, db, "u-c"))
	} else {
		require.Equal(t, int64(1000), balanceOf(t, db, "u-w"))
	}
}

// A wager that commits between cancel's first read of the round and its
// status UPDATE must either be rejected or refunded by that cancel.
func TestMySQLBet_CancelDoesNotMissConcurrentWager(t *testing.T) {
	db, dbName := newMySQLTestDB(t)
	insertMySQLUser(t, db, "u-owner", 0)
	insertMySQLUser(t, db, "u-w", 1000)
	orders := repo.NewOrderRepo(db)
	svc := service.NewBetService(orders)
	ctx := context.Background()

	view, err := svc.Open(ctx, "u-owner", "r1", 100, "Will blue win?")
	require.NoError(t, err)
	roundID := view.Round.ID

	// Pause cancel right after its first query against bet_rounds.
	var armed atomic.Bool
	reached := make(chan struct{})
	release := make(chan struct{})
	unblock := closeOnce(release)
	t.Cleanup(unblock)
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:pause_cancel", func(tx *gorm.DB) {
		if tx.Statement.Table == "bet_rounds" && armed.CompareAndSwap(true, false) {
			close(reached)
			<-release
		}
	}))
	armed.Store(true)
	cancelErr := make(chan error, 1)
	go func() {
		_, _, err := orders.CancelBetRound(ctx, roundID, "u-owner", []byte("{}"))
		cancelErr <- err
	}()
	<-reached

	wagerDone := make(chan struct{})
	var werr error
	go func() {
		defer close(wagerDone)
		_, werr = svc.Wager(ctx, "u-w", "r1", roundID, model.BetOptionLose)
	}()
	waitDoneOrLockWait(t, db, dbName, wagerDone)
	unblock()
	require.NoError(t, <-cancelErr)
	<-wagerDone

	requireNoOrphanedWagers(t, db, roundID)
	require.Equal(t, int64(1000), balanceOf(t, db, "u-w"), "stake must be refunded or never taken")
	if werr != nil {
		require.True(t, errors.Is(werr, service.ErrBetClosed), "unexpected wager error: %v", werr)
	}
}
