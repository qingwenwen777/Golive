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

func newModerationFixture(t *testing.T) (*ModerationService, *gorm.DB, *redis.Client) {
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
	require.NoError(t, db.Exec(`
CREATE TABLE fan_badges (
	user_id varchar(36),
	creator_id varchar(36),
	creator_name varchar(64),
	creator_avatar varchar(500),
	total_contribution integer,
	level integer,
	created_at datetime,
	updated_at datetime,
	primary key (user_id, creator_id)
)`).Error)
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"admin-1", "admin", "Admin", "", true, "admin", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"admin-2", "second-admin", "Second Admin", "", true, "admin", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"user-1", "reporter", "Reporter", "", false, "user", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"user-2", "reporter2", "Reporter Two", "", false, "user", now,
	).Error)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	moderation := repo.NewModerationRepo(db, rdb)
	require.NoError(t, moderation.AutoMigrate())
	return NewModerationService(moderation, rooms, repo.NewSocialRepo(rdb)), db, rdb
}

func TestRoomModeratorsMustBeFanClubMembers(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	_, err := svc.AddModerator(ctx, "admin-1", "user-1")
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	require.Equal(t, "not_fan_club_member", appErr.Reason)

	require.NoError(t, db.Exec(
		`INSERT INTO fan_badges (user_id, creator_id, creator_name, creator_avatar, total_contribution, level, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-1", "admin-1", "Admin", "", 100, 1, now, now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO fan_badges (user_id, creator_id, creator_name, creator_avatar, total_contribution, level, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-2", "other-creator", "Other", "", 100, 1, now, now,
	).Error)

	members, err := svc.ListFollowers(ctx, "admin-1", "", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), members.Total)
	require.Len(t, members.Items, 1)
	require.Equal(t, "user-1", members.Items[0].ID)

	added, err := svc.AddModerator(ctx, "admin-1", "user-1")
	require.NoError(t, err)
	require.True(t, added.Moderator)
	require.Equal(t, "user-1", added.ID)
}

func TestModerationReportsDeduplicateAndLimit(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newModerationFixture(t)
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

func TestHandledReportCannotBeProcessedAgainAndNewReportsCreateNewGroup(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	req := CreateReportReq{
		TargetType:     "danmu",
		TargetID:       "danmu-1",
		RoomID:         "room-1",
		TargetUserID:   "bad-user",
		TargetUserName: "Bad User",
		Reason:         "harassment",
		TargetText:     "bad message",
	}
	first, err := svc.CreateReport(ctx, "user-1", req)
	require.NoError(t, err)
	second, err := svc.CreateReport(ctx, "user-2", req)
	require.NoError(t, err)
	require.Equal(t, first.GroupID, second.GroupID)

	list, err := svc.ListReports(ctx, "admin-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, int64(2), list.Items[0].ReportCount)

	_, err = svc.UpdateReport(ctx, "admin-1", first.ID, UpdateReportReq{Action: "dismiss", Note: "not a violation"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", first.ID, UpdateReportReq{Action: "dismiss"})
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	require.Equal(t, "report_already_handled", appErr.Reason)

	logs, err := svc.AdminAuditLogs(ctx, "admin-1", "review", 1, 10)
	require.NoError(t, err)
	require.Len(t, logs.Items, 1)
	require.Equal(t, "dismiss", logs.Items[0].Action)

	now = now.Add(25 * time.Hour)
	third, err := svc.CreateReport(ctx, "user-1", req)
	require.NoError(t, err)
	require.NotEqual(t, first.GroupID, third.GroupID)

	list, err = svc.ListReports(ctx, "admin-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.Equal(t, int64(2), list.Stats.Total)
	require.Equal(t, int64(1), list.Stats.Pending)
}

func TestReportCanApplyMultipleActionsAndDismissIsExclusive(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{
		TargetType:     "danmu",
		TargetID:       "danmu-multi",
		RoomID:         "room-1",
		TargetUserID:   "bad-user",
		TargetUserName: "Bad User",
		Reason:         "harassment",
		TargetText:     "bad message",
	})
	require.NoError(t, err)

	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{
		Actions: []string{"delete_content", "dismiss"},
		Note:    "mixed action should fail",
	})
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	require.Equal(t, "exclusive_action", appErr.Reason)

	updated, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{
		Actions:         []string{"delete_content", "site_mute"},
		DurationMinutes: 120,
		Note:            "remove and mute",
	})
	require.NoError(t, err)
	require.Equal(t, "resolved", updated.Status)
	require.Equal(t, "delete_content,site_mute", updated.ResolutionAction)
	require.Equal(t, 120, updated.DurationMinutes)

	logs, err := svc.AdminAuditLogs(ctx, "admin-1", "review", 1, 10)
	require.NoError(t, err)
	require.Len(t, logs.Items, 2)
	actions := []string{logs.Items[0].Action, logs.Items[1].Action}
	require.ElementsMatch(t, []string{"delete_content", "site_mute"}, actions)
}

func TestReportReviewClaimLocksGroup(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{
		TargetType:     "post",
		TargetID:       "post-claim",
		TargetUserID:   "bad-user",
		TargetUserName: "Bad User",
		Reason:         "spam",
		TargetText:     "bad post",
	})
	require.NoError(t, err)

	claimed, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Status: "reviewing"})
	require.NoError(t, err)
	require.Equal(t, "reviewing", claimed.Status)
	require.Equal(t, "admin-1", claimed.ReviewerID)
	require.Equal(t, "Admin", claimed.ReviewerName)
	require.NotEmpty(t, claimed.ReviewStartedAt)
	require.NotEmpty(t, claimed.ReviewExpiresAt)

	second, err := svc.CreateReport(ctx, "user-2", CreateReportReq{
		TargetType:     "post",
		TargetID:       "post-claim",
		TargetUserID:   "bad-user",
		TargetUserName: "Bad User",
		Reason:         "spam",
		TargetText:     "same bad post",
	})
	require.NoError(t, err)
	require.Equal(t, report.GroupID, second.GroupID)
	require.Equal(t, "reviewing", second.Status)
	require.Equal(t, "admin-1", second.ReviewerID)

	_, err = svc.UpdateReport(ctx, "admin-2", report.ID, UpdateReportReq{Action: "dismiss"})
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	require.Equal(t, "report_claimed", appErr.Reason)

	updated, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Action: "dismiss"})
	require.NoError(t, err)
	require.Equal(t, "dismissed", updated.Status)
	require.Equal(t, "admin-1", updated.ReviewerID)
	require.Empty(t, updated.ReviewExpiresAt)
}

func TestExpiredReportReviewClaimReturnsToPending(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{
		TargetType:   "danmu",
		TargetID:     "danmu-expired",
		RoomID:       "room-1",
		TargetUserID: "bad-user",
		Reason:       "harassment",
		TargetText:   "bad message",
	})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Status: "reviewing"})
	require.NoError(t, err)

	now = now.Add(31 * time.Minute)
	list, err := svc.ListReports(ctx, "admin-2", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, "pending", list.Items[0].Status)
	require.Empty(t, list.Items[0].ReviewerID)
	require.Empty(t, list.Items[0].ReviewExpiresAt)
	require.Equal(t, int64(1), list.Stats.Pending)
	require.Equal(t, int64(0), list.Stats.Reviewing)

	claimed, err := svc.UpdateReport(ctx, "admin-2", report.ID, UpdateReportReq{Status: "reviewing"})
	require.NoError(t, err)
	require.Equal(t, "reviewing", claimed.Status)
	require.Equal(t, "admin-2", claimed.ReviewerID)
	require.Equal(t, "Second Admin", claimed.ReviewerName)
}

func TestBlockedWordsRejectTextAndSyncRedis(t *testing.T) {
	ctx := context.Background()
	svc, _, rdb := newModerationFixture(t)

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

	importResp, err := svc.BulkImportBlockedWords(ctx, "admin-1", BulkImportBlockedWordsReq{
		Items: []BulkBlockedWordItem{
			{Word: "Spam", Note: "ads"},
			{Word: "Bad Word", Note: "duplicate"},
			{Word: ""},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, importResp.Created)
	require.Equal(t, 2, importResp.Skipped)
}

func boolPtr(v bool) *bool { return &v }
