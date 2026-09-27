package repo_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// Pages past MaxPage used to be clamped to MaxPage, so paging on returned
// that page's items again. They must come back empty, with the real total,
// and without the database scanning a deep OFFSET.
func TestListPagesPastMaxPageAreEmpty(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	started := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for i := range 3 {
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID: fmt.Sprintf("live-%d", i), OwnerID: fmt.Sprintf("owner-%d", i), Status: model.StatusLive,
			StartedAt: started.Add(time.Duration(i) * time.Minute),
		}))
	}

	page, total, err := rooms.List(ctx, repo.ListQuery{Page: 2, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, page, 1)
	require.Equal(t, "live-1", page[0].ID, "newest first")

	page, total, err = rooms.List(ctx, repo.ListQuery{Page: repo.MaxPage + 1, Size: 1})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Empty(t, page)

	// The last query List runs is the page's SELECT.
	var sql string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture", func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
	}))
	_, _, err = rooms.List(ctx, repo.ListQuery{Page: repo.MaxPage + 1, Size: 100})
	require.NoError(t, err)
	require.Contains(t, sql, "1 = 0")
	require.NotContains(t, sql, "OFFSET")
}
