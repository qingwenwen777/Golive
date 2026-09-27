package repo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

func TestBoundFallbackSearch(t *testing.T) {
	_, _, ok := boundFallbackSearch(NewSearchPhrase("a"), 10)
	require.False(t, ok, "one character matches nearly every row")
	_, _, ok = boundFallbackSearch(NewSearchPhrase(" - "), 10)
	require.False(t, ok, "separators alone are not a query")

	// One Chinese, Japanese or Korean character is a word on its own.
	for _, word := range []string{"猫", "歌", "ね", "ネ", "한"} {
		_, limit, ok := boundFallbackSearch(NewSearchPhrase(word), 100)
		require.True(t, ok, word)
		require.Equal(t, maxFallbackSearchLimit, limit, word)
	}
	_, _, ok = boundFallbackSearch(NewSearchPhrase("é"), 10)
	require.False(t, ok, "a single Latin letter is still too broad")
	phrase, limit, ok := boundFallbackSearch(NewSearchPhrase("ab"), 10)
	require.True(t, ok)
	require.Equal(t, 10, limit)
	require.Equal(t, []string{"ab"}, phrase.Tokens)

	phrase, limit, ok = boundFallbackSearch(NewSearchPhrase("歌 枠 雑談 ゲーム 実況 配信"), 100)
	require.True(t, ok)
	require.Equal(t, maxFallbackSearchLimit, limit)
	require.Equal(t, []string{"歌", "枠", "雑談", "ゲーム"}, phrase.Tokens)
	require.Equal(t, "歌枠雑談ゲーム実況配信", phrase.Compact, "the compact match still uses the whole query")
}

func TestSearchLiveRoomsFallbackIsBounded(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, display_name TEXT)").Error)
	rooms := NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	started := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 80; i++ {
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID:           fmt.Sprintf("live-%02d", i),
			Title:        fmt.Sprintf("歌枠 %d", i),
			OwnerID:      fmt.Sprintf("owner-%d", i),
			Status:       model.StatusLive,
			StartedAt:    started.Add(time.Duration(i) * time.Second),
			ReplayStatus: model.ReplayStatusNone,
		}))
	}

	found, err := rooms.SearchLiveRooms(ctx, NewSearchPhrase("歌枠"), 100)
	require.NoError(t, err)
	require.Len(t, found, maxFallbackSearchLimit)
	require.Equal(t, "live-79", found[0].ID, "newest first")

	found, err = rooms.SearchLiveRooms(ctx, NewSearchPhrase("歌"), 100)
	require.NoError(t, err)
	require.Len(t, found, maxFallbackSearchLimit, "one CJK character searches, still bounded")

	found, err = rooms.SearchLiveRooms(ctx, NewSearchPhrase("a"), 100)
	require.NoError(t, err)
	require.Empty(t, found)
}
