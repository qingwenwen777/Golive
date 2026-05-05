package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

func newModerationFixture(t *testing.T) (*ModerationService, *redis.Client) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE users (
	id varchar(36) primary key,
	username varchar(64),
	display_name varchar(64),
	avatar varchar(500),
	verified boolean,
	role varchar(16),
	updated_at datetime
)`).Error)
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"admin-1", "admin", "Admin", "", true, "admin", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"user-1", "reporter", "Reporter", "", false, "user", now,
	).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	moderation := repo.NewModerationRepo(db, rdb)
	require.NoError(t, moderation.AutoMigrate())
	return NewModerationService(moderation, rooms, repo.NewSocialRepo(rdb)), rdb
}

func TestModerationReportsDeduplicateAndLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }

	req := CreateReportReq{
		TargetType:  "room",
		TargetID:    "live-1",
		Reason:      "spam",
		TargetTitle: "Bad live",
	}
	_, err := svc.CreateReport(ctx, "user-1", req)
	require.NoError(t, err)

	_, err = svc.CreateReport(ctx, "user-1", req)
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	require.Equal(t, "report_duplicate", appErr.Reason)

	for i := 2; i <= 20; i++ {
		req.TargetID = "live-limit-" + string(rune('a'+i))
		_, err = svc.CreateReport(ctx, "user-1", req)
		require.NoError(t, err)
	}
	req.TargetID = "live-limit-over"
	_, err = svc.CreateReport(ctx, "user-1", req)
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusTooManyRequests, appErr.HTTPStatus)
	require.Equal(t, "report_daily_limit", appErr.Reason)
}

func TestBlockedWordsRejectTextAndSyncRedis(t *testing.T) {
	ctx := context.Background()
	svc, rdb := newModerationFixture(t)

	word, err := svc.CreateBlockedWord(ctx, "admin-1", CreateBlockedWordReq{Word: "Bad Word"})
	require.NoError(t, err)
	require.True(t, word.Enabled)

	require.Error(t, svc.EnsureTextAllowed(ctx, "hello bad word"))
	redisWords, err := rdb.SMembers(ctx, contentpolicy.RedisBlockedWordsKey).Result()
	require.NoError(t, err)
	require.Contains(t, redisWords, "bad word")

	_, err = svc.UpdateBlockedWord(ctx, "admin-1", word.ID, UpdateBlockedWordReq{Enabled: boolPtr(false)})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureTextAllowed(ctx, "hello bad word"))
}

func boolPtr(v bool) *bool { return &v }
