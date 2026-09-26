package repo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

func seedDailyRewardUser(t *testing.T, users *UserRepo, id string, balance int64) {
	t.Helper()
	require.NoError(t, users.Create(context.Background(), &model.User{
		ID:           id,
		Username:     id,
		DisplayName:  id,
		PasswordHash: "hash",
		CoinBalance:  balance,
		Role:         model.RoleUser,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}))
}

// TestClaimDailyCoinRewardConcurrentMySQL fires many parallel claims for the
// same task and day. Under InnoDB REPEATABLE READ they used to all pass the
// "already claimed" check and credit the reward once each.
func TestClaimDailyCoinRewardConcurrentMySQL(t *testing.T) {
	db := openTestMySQL(t)
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(64)

	const (
		userID   = "daily-race-user"
		sourceID = "daily-login-lottery:2026-09-26"
		reward   = int64(10)
		workers  = 32
	)
	seedDailyRewardUser(t, users, userID, 1000)

	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	txIDs := map[string]struct{}{}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, tx, created, err := users.ClaimDailyCoinReward(ctx, userID, sourceID, "Daily", "daily", reward)
			if !assertNoError(t, err) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if created {
				createdCount++
			}
			txIDs[tx.ID] = struct{}{}
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, 1, createdCount, "exactly one claim may be credited")
	require.Len(t, txIDs, 1, "every caller must see the same ledger row")

	var rows int64
	require.NoError(t, db.Model(&model.CoinTransaction{}).
		Where("user_id = ? AND type = ? AND source_id = ?", userID, model.CoinTxDailyTask, sourceID).
		Count(&rows).Error)
	require.Equal(t, int64(1), rows)

	u, err := users.FindByID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int64(1000)+reward, u.CoinBalance)
}

// TestClaimDailyCoinRewardDuplicateLedgerKeyIsAlreadyClaimed covers the
// fallback for a claim that loses the primary-key race: the ledger row for
// this user, task and day already exists (here without a matching source_id,
// so the pre-check misses it), and the claim must roll back its credit and
// report the existing row instead of failing or paying twice.
func TestClaimDailyCoinRewardDuplicateLedgerKeyIsAlreadyClaimed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	users := NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())
	seedDailyRewardUser(t, users, "dup-user", 500)

	const sourceID = "daily-login-lottery:2026-09-26"
	existing := model.CoinTransaction{
		ID:           dailyTaskTxID("dup-user", sourceID),
		UserID:       "dup-user",
		Type:         model.CoinTxDailyTask,
		Amount:       7,
		BalanceAfter: 500,
		Title:        "Daily",
	}
	require.NoError(t, db.Create(&existing).Error)

	u, tx, created, err := users.ClaimDailyCoinReward(context.Background(), "dup-user", sourceID, "Daily", "daily", 10)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, existing.ID, tx.ID)
	require.Equal(t, int64(500), u.CoinBalance)
}

func assertNoError(t *testing.T, err error) bool {
	t.Helper()
	if err != nil {
		t.Errorf("claim failed: %v", err)
		return false
	}
	return true
}
