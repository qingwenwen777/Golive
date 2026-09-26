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

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

func newModerationFixture(t *testing.T) (*ModerationService, *gorm.DB, *redis.Client) {
	svc, db, rdb, _ := newModerationFixtureWithOwners(t)
	return svc, db, rdb
}

// newModerationFixtureWithOwners also returns the fake owner services that
// report actions call for bans/mutes, super chats and chat messages.
func newModerationFixtureWithOwners(t *testing.T) (*ModerationService, *gorm.DB, *redis.Client, *fakeOwners) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// One connection: every ":memory:" connection is a separate database,
	// and the fake owner services write through the same handle.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
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
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"bad-user", "bad", "Bad User", "", false, "user", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"creator-1", "creator", "Creator One", "", true, "user", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"mod-1", "mod", "Moderator One", "", false, "moderator", now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified, role, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"mod-2", "mod2", "Moderator Two", "", false, "moderator", now,
	).Error)

	// Reported content lives in tables owned by other repos/services.
	require.NoError(t, repo.NewPostRepo(db).AutoMigrate())
	require.NoError(t, repo.NewReplayCommentRepo(db).AutoMigrate())
	require.NoError(t, db.AutoMigrate(&model.Notification{}))
	require.NoError(t, db.Exec(`
CREATE TABLE danmus_0 (
	id varchar(64) primary key,
	room_id varchar(64),
	user_id varchar(36),
	username varchar(64),
	text varchar(500),
	deleted_at datetime
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE super_chat_orders (
	order_id varchar(64) primary key,
	user_id varchar(36),
	room_id varchar(64),
	text varchar(500),
	status varchar(16),
	fail_reason varchar(32),
	moderated_at datetime
)`).Error)
	seedReportRoom(t, db, "room-1", "creator-1")

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	moderation := repo.NewModerationRepo(db, rdb)
	require.NoError(t, moderation.AutoMigrate())
	svc := NewModerationService(moderation, rooms, repo.NewSocialRepo(rdb))
	owners := newFakeOwners(t, db)
	svc.SetOwnerServices(owners.services())
	return svc, db, rdb, owners
}

func seedReportRoom(t *testing.T, db *gorm.DB, roomID, ownerID string) {
	t.Helper()
	require.NoError(t, db.Create(&model.Room{
		ID:          roomID,
		Title:       "Room " + roomID,
		Description: "Live description",
		Channel:     "Channel " + ownerID,
		ChannelID:   "ch-" + ownerID,
		OwnerID:     ownerID,
		Status:      model.StatusLive,
		StartedAt:   time.Now(),
	}).Error)
}

func seedReportDanmu(t *testing.T, db *gorm.DB, roomID, danmuID, userID, text string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO danmus_0 (id, room_id, user_id, username, text) VALUES (?, ?, ?, ?, ?)`,
		danmuID, roomID, userID, userID, text,
	).Error)
}

func seedReportPost(t *testing.T, db *gorm.DB, postID, ownerID, content string) {
	t.Helper()
	require.NoError(t, db.Create(&model.ChannelPost{
		ID:        postID,
		OwnerID:   ownerID,
		ChannelID: "ch-" + ownerID,
		Content:   content,
	}).Error)
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
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	seedReportRoom(t, db, "live-1", "creator-1")
	for i := 2; i <= 20; i++ {
		seedReportRoom(t, db, "live-limit-"+string(rune('a'+i)), "creator-1")
	}
	seedReportRoom(t, db, "live-limit-over", "creator-1")

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
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportDanmu(t, db, "room-1", "danmu-1", "bad-user", "bad message")

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
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	seedReportDanmu(t, db, "room-1", "danmu-multi", "bad-user", "bad message")

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
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportPost(t, db, "post-claim", "bad-user", "bad post")

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
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportDanmu(t, db, "room-1", "danmu-expired", "bad-user", "bad message")

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

func TestAdminSystemSettingsDriveReviewAndMutePolicy(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportPost(t, db, "post-policy", "bad-user", "bad post")
	seedReportDanmu(t, db, "room-1", "danmu-policy", "bad-user", "bad message")

	defaults, err := svc.AdminSystemSettings(ctx, "admin-1")
	require.NoError(t, err)
	require.Equal(t, 30, defaults.ReportReviewTimeoutMinutes)
	require.Equal(t, 1440, defaults.DefaultSiteMuteMinutes)
	require.Equal(t, "invite_only", defaults.RegistrationPolicy)

	updated, err := svc.UpdateAdminSystemSettings(ctx, "admin-1", UpdateAdminSystemSettingsReq{
		ReportReviewTimeoutMinutes: intPtr(10),
		DefaultSiteMuteMinutes:     intPtr(120),
		Note:                       "faster reviews",
	})
	require.NoError(t, err)
	require.Equal(t, 10, updated.ReportReviewTimeoutMinutes)
	require.Equal(t, 120, updated.DefaultSiteMuteMinutes)

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{
		TargetType:   "post",
		TargetID:     "post-policy",
		TargetUserID: "bad-user",
		Reason:       "spam",
		TargetText:   "bad post",
	})
	require.NoError(t, err)
	claimed, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Status: "reviewing"})
	require.NoError(t, err)
	startedAt, err := time.Parse(time.RFC3339, claimed.ReviewStartedAt)
	require.NoError(t, err)
	expiresAt, err := time.Parse(time.RFC3339, claimed.ReviewExpiresAt)
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute, expiresAt.Sub(startedAt))

	muteReport, err := svc.CreateReport(ctx, "user-2", CreateReportReq{
		TargetType:     "danmu",
		TargetID:       "danmu-policy",
		RoomID:         "room-1",
		TargetUserID:   "bad-user",
		TargetUserName: "Bad User",
		Reason:         "harassment",
		TargetText:     "bad message",
	})
	require.NoError(t, err)
	resolved, err := svc.UpdateReport(ctx, "admin-1", muteReport.ID, UpdateReportReq{
		Actions: []string{"site_mute"},
		Note:    "default duration",
	})
	require.NoError(t, err)
	require.Equal(t, 120, resolved.DurationMinutes)

	logs, err := svc.AdminAuditLogs(ctx, "admin-1", "system", 1, 10)
	require.NoError(t, err)
	require.Len(t, logs.Items, 1)
	require.Equal(t, "system_settings_update", logs.Items[0].Action)
}

func requireAppErrReason(t *testing.T, err error, status int, reason string) {
	t.Helper()
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr), "expected %s, got %v", reason, err)
	require.Equal(t, status, appErr.HTTPStatus)
	require.Equal(t, reason, appErr.Reason)
}

func TestCreateReportResolvesTargetServerSide(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	seedReportDanmu(t, db, "room-1", "danmu-real", "bad-user", "real bad message")
	seedReportPost(t, db, "post-real", "bad-user", "real bad post")
	require.NoError(t, db.Create(&model.PostComment{ID: "comment-real", PostID: "post-real", UserID: "user-2", Content: "real comment"}).Error)
	require.NoError(t, db.Create(&model.ReplayComment{ID: "replay-comment-real", RoomID: "room-1", UserID: "bad-user", Content: "real replay comment"}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO super_chat_orders (order_id, user_id, room_id, text, status) VALUES (?, ?, ?, ?, ?)`,
		"sc-real", "bad-user", "room-1", "real super chat", "success",
	).Error)

	// Everything but type/id (and the room used to locate danmu) is ignored.
	spoof := CreateReportReq{
		TargetURL:       "https://evil.example/login",
		ChannelID:       "ch-admin-1",
		TargetOwnerID:   "admin-1",
		TargetOwnerName: "Admin",
		TargetUserID:    "admin-1",
		TargetUserName:  "Admin",
		TargetTitle:     "fake title",
		TargetText:      "fake evidence",
		Reason:          "harassment",
	}
	cases := []struct {
		targetType, targetID, roomID                              string
		wantID, wantRoom, wantOwner, wantUser, wantText, wantLink string
	}{
		{"room", "room-1", "room-2", "room-1", "room-1", "creator-1", "", "Live description", "/live/room-1"},
		{"channel", "creator", "", "ch-creator-1", "", "creator-1", "", "", "/channel/creator-1"},
		{"danmu", "danmu-real", "room-1", "danmu-real", "room-1", "creator-1", "bad-user", "real bad message", "/live/room-1"},
		{"post", "post-real", "room-1", "post-real", "", "bad-user", "bad-user", "real bad post", "/channel/bad-user#post-post-real"},
		{"post_comment", "comment-real", "", "comment-real", "", "bad-user", "user-2", "real comment", "/channel/bad-user#comment-comment-real"},
		{"post_comment", "replay-comment-real", "", "replay-comment-real", "room-1", "creator-1", "bad-user", "real replay comment", "/live/room-1#replay-comment-replay-comment-real"},
		{"super_chat", "sc-real", "room-9", "sc-real", "room-1", "creator-1", "bad-user", "real super chat", "/live/room-1"},
	}
	for _, tc := range cases {
		req := spoof
		req.TargetType, req.TargetID, req.RoomID = tc.targetType, tc.targetID, tc.roomID
		report, err := svc.CreateReport(ctx, "user-1", req)
		require.NoError(t, err, tc.targetType+"/"+tc.targetID)
		require.Equal(t, tc.wantID, report.TargetID)
		require.Equal(t, tc.wantRoom, report.RoomID, tc.targetID)
		require.Equal(t, tc.wantOwner, report.TargetOwnerID, tc.targetID)
		require.Equal(t, tc.wantUser, report.TargetUserID, tc.targetID)
		require.Equal(t, tc.wantText, report.TargetText, tc.targetID)
		require.Equal(t, tc.wantLink, report.TargetURL, tc.targetID)
		require.NotEqual(t, "fake title", report.TargetTitle)
	}

	for _, missing := range []CreateReportReq{
		{TargetType: "room", TargetID: "no-such-room", Reason: "spam"},
		{TargetType: "danmu", TargetID: "danmu-real", RoomID: "room-2", Reason: "spam"},
		{TargetType: "danmu", TargetID: "danmu-real", Reason: "spam"},
		{TargetType: "post", TargetID: "no-such-post", Reason: "spam"},
		{TargetType: "channel", TargetID: "ch-nobody", Reason: "spam"},
		{TargetType: "super_chat", TargetID: "sc-missing", Reason: "spam"},
	} {
		_, err := svc.CreateReport(ctx, "user-2", missing)
		requireAppErrReason(t, err, http.StatusNotFound, "target_not_found")
	}
}

// A reporter adding one more report with a forged targetUserId/roomId to a
// legit report group must not redirect the ban or force-end.
func TestReportActionsIgnoreClientSuppliedTargets(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportDanmu(t, db, "room-1", "danmu-legit", "bad-user", "real bad message")
	seedReportRoom(t, db, "room-victim", "user-2")

	legit, err := svc.CreateReport(ctx, "user-1", CreateReportReq{
		TargetType: "danmu", TargetID: "danmu-legit", RoomID: "room-1", Reason: "harassment",
	})
	require.NoError(t, err)
	now = now.Add(time.Minute)
	forged, err := svc.CreateReport(ctx, "user-2", CreateReportReq{
		TargetType:    "danmu",
		TargetID:      "danmu-legit",
		RoomID:        "room-1",
		TargetUserID:  "admin-1",
		TargetOwnerID: "user-2",
		TargetText:    "fabricated evidence",
		TargetURL:     "https://evil.example/phish",
		Reason:        "harassment",
	})
	require.NoError(t, err)
	require.Equal(t, legit.GroupID, forged.GroupID)

	list, err := svc.ListReports(ctx, "admin-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, "bad-user", list.Items[0].TargetUserID)
	require.Equal(t, "real bad message", list.Items[0].TargetText)
	require.Equal(t, "/live/room-1", list.Items[0].TargetURL)

	_, err = svc.UpdateReport(ctx, "admin-1", list.Items[0].ID, UpdateReportReq{Actions: []string{"force_end_live"}})
	requireAppErrReason(t, err, http.StatusBadRequest, "action_not_allowed_for_target")

	_, err = svc.UpdateReport(ctx, "admin-1", list.Items[0].ID, UpdateReportReq{Actions: []string{"ban_user"}, Note: "abuse"})
	require.NoError(t, err)

	banned, err := svc.moderation.UserRestriction(ctx, "bad-user", now)
	require.NoError(t, err)
	require.True(t, banned.Banned)
	admin, err := svc.moderation.UserRestriction(ctx, "admin-1", now)
	require.NoError(t, err)
	require.False(t, admin.Banned)

	var notes []model.Notification
	require.NoError(t, db.Where("type = ?", "moderation_ban").Find(&notes).Error)
	require.Len(t, notes, 1)
	require.Equal(t, "bad-user", notes[0].UserID)
	require.Equal(t, "/live/room-1", notes[0].Link)
}

// Danmu ids are client-chosen, so the same id in two rooms is two targets.
func TestDanmuReportsGroupPerRoom(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	seedReportRoom(t, db, "room-2", "user-2")
	require.NoError(t, db.Exec(`CREATE TABLE danmus_1 AS SELECT * FROM danmus_0 WHERE 0`).Error)
	seedReportDanmu(t, db, "room-1", "same-id", "bad-user", "abuse")
	require.NoError(t, db.Exec(
		`INSERT INTO danmus_1 (id, room_id, user_id, username, text) VALUES (?, ?, ?, ?, ?)`,
		"same-id", "room-2", "user-2", "user-2", "harmless",
	).Error)

	first, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "danmu", TargetID: "same-id", RoomID: "room-1", Reason: "spam"})
	require.NoError(t, err)
	second, err := svc.CreateReport(ctx, "admin-1", CreateReportReq{TargetType: "danmu", TargetID: "same-id", RoomID: "room-2", Reason: "spam"})
	require.NoError(t, err)
	require.NotEqual(t, first.GroupID, second.GroupID)
	require.Equal(t, "bad-user", first.TargetUserID)
	require.Equal(t, "user-2", second.TargetUserID)
}

// Rows written before targets were resolved server-side keep client-supplied
// ids; they must not drive sanctions or leak external links.
func TestLegacyUnverifiedReportCannotDriveSanctions(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	require.NoError(t, db.Create(&model.ContentReport{
		ID:           "legacy-1",
		GroupID:      "legacy-group",
		ReporterID:   "user-1",
		TargetType:   "post",
		TargetID:     "post-gone",
		TargetURL:    "https://evil.example/phish",
		TargetUserID: "admin-2",
		Reason:       "spam",
		Status:       model.ReportStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error)

	detail, err := svc.ReportDetail(ctx, "admin-1", "legacy-1")
	require.NoError(t, err)
	require.Empty(t, detail.TargetURL)
	// The content is gone, so the row cannot be re-verified and says so.
	require.False(t, detail.TargetVerified)
	list, err := svc.ListReports(ctx, "admin-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.False(t, list.Items[0].TargetVerified)
	require.Equal(t, "admin-2", list.Items[0].TargetUserID)

	_, err = svc.UpdateReport(ctx, "admin-1", "legacy-1", UpdateReportReq{Actions: []string{"ban_user"}})
	requireAppErrReason(t, err, http.StatusConflict, "target_unverified")
	restriction, err := svc.moderation.UserRestriction(ctx, "admin-2", now)
	require.NoError(t, err)
	require.False(t, restriction.Banned)

	_, err = svc.UpdateReport(ctx, "admin-1", "legacy-1", UpdateReportReq{Action: "dismiss"})
	require.NoError(t, err)
}

// Legacy rows whose content still exists carry the reporter's client-side
// snapshot. Before moderators see or act on one, it is replaced by the
// content's real author, text and link, so the user shown is the user a
// sanction hits and the audit log records.
func TestLegacyReportIsReverifiedBeforeListingAndActing(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportPost(t, db, "post-victim", "user-2", "a perfectly fine post")
	seedReportPost(t, db, "post-detail", "creator-1", "a creator post")
	seedReportPost(t, db, "post-act", "creator-1", "another creator post")
	legacy := func(id, postID string) {
		t.Helper()
		require.NoError(t, db.Create(&model.ContentReport{
			ID: id, GroupID: "group-" + id, ReporterID: "user-1",
			TargetType: "post", TargetID: postID, TargetURL: "https://evil.example/phish",
			TargetOwnerID: "bad-user", TargetOwnerName: "Bad User",
			TargetUserID: "bad-user", TargetUserName: "Bad User",
			TargetTitle: "Bad User", TargetText: "BUY STOLEN CARDS",
			Reason: "scam", Description: "reporter's own words",
			Status: model.ReportStatusPending, CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	requireVerified := func(dto ContentReportDTO, userID, userName, text, link string) {
		t.Helper()
		require.True(t, dto.TargetVerified, dto.ID)
		require.Equal(t, userID, dto.TargetUserID, dto.ID)
		require.Equal(t, userID, dto.TargetOwnerID, dto.ID)
		require.Equal(t, userName, dto.TargetUserName, dto.ID)
		require.Equal(t, userName, dto.TargetTitle, dto.ID)
		require.Equal(t, text, dto.TargetText, dto.ID)
		require.Equal(t, link, dto.TargetURL, dto.ID)
		var stored model.ContentReport
		require.NoError(t, db.Where("id = ?", dto.ID).Take(&stored).Error)
		require.True(t, stored.TargetVerified, dto.ID)
		require.Equal(t, userID, stored.TargetUserID, dto.ID)
		require.Equal(t, text, stored.TargetText, dto.ID)
		require.Equal(t, "reporter's own words", stored.Description, dto.ID)
	}
	requireAudited := func(action, reportID, userID, userName string) {
		t.Helper()
		var logs []model.AdminAuditLog
		require.NoError(t, db.Where("action = ? AND target_id = ?", action, reportID).Find(&logs).Error)
		require.Len(t, logs, 1)
		require.Equal(t, userID, logs[0].TargetUserID)
		require.Equal(t, userName, logs[0].TargetUserName)
	}

	legacy("legacy-listed", "post-victim")
	list, err := svc.ListReports(ctx, "admin-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	requireVerified(list.Items[0], "user-2", "Reporter Two", "a perfectly fine post", "/channel/user-2#post-post-victim")
	_, err = svc.UpdateReport(ctx, "admin-1", "legacy-listed", UpdateReportReq{Actions: []string{"ban_user"}})
	require.NoError(t, err)
	for userID, banned := range map[string]bool{"user-2": true, "bad-user": false} {
		restriction, err := svc.moderation.UserRestriction(ctx, userID, now)
		require.NoError(t, err)
		require.Equal(t, banned, restriction.Banned, userID)
	}
	requireAudited("ban_user", "legacy-listed", "user-2", "Reporter Two")

	// Rows that were not listed first are re-verified by detail and update.
	legacy("legacy-detail", "post-detail")
	detail, err := svc.ReportDetail(ctx, "admin-1", "legacy-detail")
	require.NoError(t, err)
	requireVerified(*detail, "creator-1", "Creator One", "a creator post", "/channel/creator-1#post-post-detail")
	legacy("legacy-act", "post-act")
	_, err = svc.UpdateReport(ctx, "admin-1", "legacy-act", UpdateReportReq{Actions: []string{"warn_user"}})
	require.NoError(t, err)
	acted, err := svc.ReportDetail(ctx, "admin-1", "legacy-act")
	require.NoError(t, err)
	requireVerified(*acted, "creator-1", "Creator One", "another creator post", "/channel/creator-1#post-post-act")
	requireAudited("warn_user", "legacy-act", "creator-1", "Creator One")
	var warned int64
	require.NoError(t, db.Model(&model.Notification{}).Where("type = ? AND user_id = ?", "moderation_warning", "creator-1").Count(&warned).Error)
	require.EqualValues(t, 1, warned)
}

// Platform moderators only get the content-review surface (reports, blocked
// words); dashboard, audit logs and system settings stay admin-only.
func TestPlatformModeratorIsLimitedToContentReview(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }

	_, err := svc.AdminOverview(ctx, "mod-1")
	requireAppErrStatus(t, err, http.StatusForbidden)
	_, err = svc.AdminSystemSettings(ctx, "mod-1")
	requireAppErrStatus(t, err, http.StatusForbidden)
	_, err = svc.UpdateAdminSystemSettings(ctx, "mod-1", UpdateAdminSystemSettingsReq{ReportReviewTimeoutMinutes: intPtr(10)})
	requireAppErrStatus(t, err, http.StatusForbidden)
	_, err = svc.AdminAuditLogs(ctx, "mod-1", "review", 1, 10)
	requireAppErrStatus(t, err, http.StatusForbidden)

	_, err = svc.ListReports(ctx, "user-1", repo.ReportListFilter{Page: 1, Size: 10})
	requireAppErrStatus(t, err, http.StatusForbidden)
	_, err = svc.ListBlockedWords(ctx, "user-1", 1, 10)
	requireAppErrStatus(t, err, http.StatusForbidden)

	word, err := svc.CreateBlockedWord(ctx, "mod-1", CreateBlockedWordReq{Word: "scam link"})
	require.NoError(t, err)
	_, err = svc.ListBlockedWords(ctx, "mod-1", 1, 10)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteBlockedWord(ctx, "mod-1", word.ID))

	seedReportPost(t, db, "post-mod", "bad-user", "spam post")
	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "post", TargetID: "post-mod", Reason: "spam"})
	require.NoError(t, err)
	_, err = svc.ListReports(ctx, "mod-1", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	_, err = svc.ReportDetail(ctx, "mod-1", report.ID)
	require.NoError(t, err)
	resolved, err := svc.UpdateReport(ctx, "mod-1", report.ID, UpdateReportReq{Actions: []string{"ban_user"}})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Status)
}

// Banned staff keep their role and can still sign in (to appeal), so the
// staff guards must check the ban themselves. user-service records a ban in
// users.banned and user_moderation_states; either one counts.
func TestBannedStaffLoseModerationAndAdminAccess(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportPost(t, db, "post-x", "user-2", "harmless post")
	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "post", TargetID: "post-x", Reason: "spam"})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN banned boolean`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN ban_reason text`).Error)
	require.NoError(t, db.Exec(`UPDATE users SET banned = ?, ban_reason = ? WHERE id = ?`, true, "compromised", "admin-2").Error)
	require.NoError(t, db.Create(&model.UserModerationState{UserID: "mod-1", Banned: true, BanReason: "rogue", UpdatedAt: now, CreatedAt: now}).Error)

	for _, staff := range []string{"mod-1", "admin-2"} {
		_, err = svc.ListReports(ctx, staff, repo.ReportListFilter{Page: 1, Size: 10})
		requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
		_, err = svc.ReportDetail(ctx, staff, report.ID)
		requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
		_, err = svc.UpdateReport(ctx, staff, report.ID, UpdateReportReq{Actions: []string{"delete_content", "ban_user"}})
		requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
		_, err = svc.ListBlockedWords(ctx, staff, 1, 10)
		requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
		_, err = svc.CreateBlockedWord(ctx, staff, CreateBlockedWordReq{Word: "scam link"})
		requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
	}
	_, err = svc.AdminOverview(ctx, "admin-2")
	requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
	_, err = svc.AdminSystemSettings(ctx, "admin-2")
	requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
	_, err = svc.UpdateAdminSystemSettings(ctx, "admin-2", UpdateAdminSystemSettingsReq{ReportReviewTimeoutMinutes: intPtr(10)})
	requireAppErrReason(t, err, http.StatusForbidden, "user_banned")
	_, err = svc.AdminAuditLogs(ctx, "admin-2", "review", 1, 10)
	requireAppErrReason(t, err, http.StatusForbidden, "user_banned")

	restriction, err := svc.moderation.UserRestriction(ctx, "user-2", now)
	require.NoError(t, err)
	require.False(t, restriction.Banned)
	var posts int64
	require.NoError(t, db.Model(&model.ChannelPost{}).Where("id = ?", "post-x").Count(&posts).Error)
	require.EqualValues(t, 1, posts)

	// Staff who are not banned keep their access.
	_, err = svc.ListReports(ctx, "mod-2", repo.ReportListFilter{Page: 1, Size: 10})
	require.NoError(t, err)
	_, err = svc.AdminAuditLogs(ctx, "admin-1", "review", 1, 10)
	require.NoError(t, err)
}

func TestReportsCannotSanctionStaff(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportPost(t, db, "post-by-admin", "admin-2", "admin post")
	seedReportPost(t, db, "post-by-mod", "mod-2", "moderator post")
	seedReportRoom(t, db, "room-admin", "admin-2")

	adminPost, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "post", TargetID: "post-by-admin", Reason: "spam"})
	require.NoError(t, err)
	for _, actor := range []string{"mod-1", "admin-1"} {
		for _, action := range []string{"ban_user", "site_mute", "warn_user"} {
			_, err = svc.UpdateReport(ctx, actor, adminPost.ID, UpdateReportReq{Actions: []string{action}})
			requireAppErrReason(t, err, http.StatusForbidden, "target_is_admin")
		}
	}
	adminRoom, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "room", TargetID: "room-admin", Reason: "spam"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", adminRoom.ID, UpdateReportReq{Actions: []string{"force_end_live"}})
	requireAppErrReason(t, err, http.StatusForbidden, "target_is_admin")
	restriction, err := svc.moderation.UserRestriction(ctx, "admin-2", now)
	require.NoError(t, err)
	require.False(t, restriction.Banned)
	require.False(t, restriction.Muted)

	modPost, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "post", TargetID: "post-by-mod", Reason: "spam"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "mod-1", modPost.ID, UpdateReportReq{Actions: []string{"ban_user"}})
	requireAppErrReason(t, err, http.StatusForbidden, "target_is_moderator")
	_, err = svc.UpdateReport(ctx, "admin-1", modPost.ID, UpdateReportReq{Actions: []string{"ban_user"}})
	require.NoError(t, err)
	restriction, err = svc.moderation.UserRestriction(ctx, "mod-2", now)
	require.NoError(t, err)
	require.True(t, restriction.Banned)
}

// A ban from a report must take the user off air, not only block the next
// GoLive.
func TestReportBanEndsBannedUsersLiveRooms(t *testing.T) {
	ctx := context.Background()
	svc, db, rdb := newModerationFixture(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	liveRepo := repo.NewLiveRepo(rdb)
	svc.SetLiveService(NewLiveService(svc.rooms, liveRepo, "test-secret", time.Hour, "http://srs/live"))

	require.NoError(t, db.Create(&model.Room{
		ID:        "room-bad",
		Title:     "Bad stream",
		OwnerID:   "bad-user",
		ChannelID: "ch-bad-user",
		Status:    model.StatusLive,
		StreamKey: "lk_bad",
		StartedAt: now.Add(-time.Hour),
	}).Error)
	require.NoError(t, liveRepo.Save(ctx, "lk_bad", "room-bad", time.Hour))
	seedReportDanmu(t, db, "room-1", "danmu-ban", "bad-user", "abuse")

	require.NoError(t, svc.EnsureUserNotBanned(ctx, "bad-user"))
	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "danmu", TargetID: "danmu-ban", RoomID: "room-1", Reason: "harassment"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"ban_user"}})
	require.NoError(t, err)

	requireAppErrReason(t, svc.EnsureUserNotBanned(ctx, "bad-user"), http.StatusForbidden, "user_banned")
	room, err := svc.rooms.GetByID(ctx, "room-bad")
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
	_, err = liveRepo.Resolve(ctx, "lk_bad")
	require.Error(t, err)

	// The room the danmu was posted in belongs to someone else and stays live.
	other, err := svc.rooms.GetByID(ctx, "room-1")
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, other.Status)
}

func requireAppErrStatus(t *testing.T, err error, status int) {
	t.Helper()
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr), "expected HTTP %d, got %v", status, err)
	require.Equal(t, status, appErr.HTTPStatus)
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// Deleting a reported super chat asks gift-service to hide it; the order
// keeps status=success (no refund, ledger intact) and room-service no
// longer writes super_chat_orders itself.
func TestReportDeleteSuperChatHidesViaGiftService(t *testing.T) {
	ctx := context.Background()
	svc, db, _, owners := newModerationFixtureWithOwners(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	require.NoError(t, db.Exec(
		`INSERT INTO super_chat_orders (order_id, user_id, room_id, text, status) VALUES (?, ?, ?, ?, ?)`,
		"sc-bad", "bad-user", "room-1", "paid abuse", "success",
	).Error)

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "super_chat", TargetID: "sc-bad", Reason: "harassment"})
	require.NoError(t, err)
	resolved, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"delete_content"}})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Status)
	require.Contains(t, owners.called(), "POST /internal/super-chats/sc-bad/moderation")

	var row struct {
		Status      string
		FailReason  *string
		ModeratedAt *time.Time
	}
	require.NoError(t, db.Raw(`SELECT status, fail_reason, moderated_at FROM super_chat_orders WHERE order_id = ?`, "sc-bad").Scan(&row).Error)
	require.Equal(t, "success", row.Status)
	require.Nil(t, row.FailReason)
	require.NotNil(t, row.ModeratedAt)
}

// The web client reports replay comments as post_comment too, so deleting
// the reported content must remove the replay comment and its replies, not
// only resolve the report. Post comments are still deleted as before.
func TestReportDeleteRemovesReportedComments(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newModerationFixture(t)
	svc.now = func() time.Time { return time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC) }
	require.NoError(t, db.Create(&model.ReplayComment{ID: "rc-1", RoomID: "room-1", UserID: "bad-user", Content: "abusive replay comment", ReplyCount: 1}).Error)
	require.NoError(t, db.Create(&model.ReplayComment{ID: "rc-2", RoomID: "room-1", UserID: "user-2", ParentID: "rc-1", RootID: "rc-1", Depth: 1, Content: "reply"}).Error)
	require.NoError(t, db.Create(&model.ReplayComment{ID: "rc-3", RoomID: "room-1", UserID: "user-2", Content: "unrelated"}).Error)
	seedReportPost(t, db, "post-1", "creator-1", "post")
	require.NoError(t, db.Create(&model.PostComment{ID: "pc-1", PostID: "post-1", UserID: "bad-user", Content: "abusive post comment"}).Error)

	for _, targetID := range []string{"rc-1", "pc-1"} {
		report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "post_comment", TargetID: targetID, Reason: "harassment"})
		require.NoError(t, err)
		resolved, err := svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"delete_content"}})
		require.NoError(t, err)
		require.Equal(t, "resolved", resolved.Status)
	}

	var replays []string
	require.NoError(t, db.Model(&model.ReplayComment{}).Order("id").Pluck("id", &replays).Error)
	require.Equal(t, []string{"rc-3"}, replays)
	var postComments int64
	require.NoError(t, db.Model(&model.PostComment{}).Where("id = ?", "pc-1").Count(&postComments).Error)
	require.Zero(t, postComments)
	var notes int64
	require.NoError(t, db.Model(&model.Notification{}).Where("user_id = ? AND type = ?", "bad-user", "moderation_content_deleted").Count(&notes).Error)
	require.EqualValues(t, 2, notes)
}

// Danmu deletion and site mutes go to chat-service / user-service.
func TestReportDanmuDeleteAndMuteGoThroughOwners(t *testing.T) {
	ctx := context.Background()
	svc, db, rdb, owners := newModerationFixtureWithOwners(t)
	now := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return now }
	seedReportDanmu(t, db, "room-1", "danmu-owned", "bad-user", "bad message")

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "danmu", TargetID: "danmu-owned", RoomID: "room-1", Reason: "spam"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"delete_content", "site_mute"}, DurationMinutes: 120})
	require.NoError(t, err)
	require.Equal(t, []string{
		"DELETE /internal/rooms/room-1/danmus/danmu-owned",
		"POST /internal/users/bad-user/restriction",
	}, owners.called())

	var deletedAt *time.Time
	require.NoError(t, db.Raw(`SELECT deleted_at FROM danmus_0 WHERE id = ?`, "danmu-owned").Scan(&deletedAt).Error)
	require.NotNil(t, deletedAt)
	restriction, err := svc.moderation.UserRestriction(ctx, "bad-user", now)
	require.NoError(t, err)
	require.True(t, restriction.Muted)
	require.Equal(t, now.Add(2*time.Hour), restriction.MuteExpiresAt.UTC())
	require.Positive(t, rdb.TTL(ctx, contentpolicy.RedisSiteMutePrefix+"bad-user").Val())

	var logs int64
	require.NoError(t, db.Model(&model.UserSanctionLog{}).Where("target_user_id = ? AND action = ?", "bad-user", model.UserSanctionSiteMute).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
}

// If the owning service fails, the action fails visibly: the report stays
// open and nothing is recorded as done.
func TestReportActionFailsVisiblyWhenOwnerServiceFails(t *testing.T) {
	ctx := context.Background()
	svc, db, _, owners := newModerationFixtureWithOwners(t)
	now := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	seedReportDanmu(t, db, "room-1", "danmu-down", "bad-user", "abuse")
	owners.fail("/internal/users/bad-user/restriction", http.StatusServiceUnavailable)

	report, err := svc.CreateReport(ctx, "user-1", CreateReportReq{TargetType: "danmu", TargetID: "danmu-down", RoomID: "room-1", Reason: "harassment"})
	require.NoError(t, err)
	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"ban_user"}})
	require.ErrorContains(t, err, "user-service ban user bad-user")

	detail, err := svc.ReportDetail(ctx, "admin-1", report.ID)
	require.NoError(t, err)
	require.NotEqual(t, "resolved", detail.Status)
	restriction, err := svc.moderation.UserRestriction(ctx, "bad-user", now)
	require.NoError(t, err)
	require.False(t, restriction.Banned)
	var logs int64
	require.NoError(t, db.Model(&model.UserSanctionLog{}).Where("target_user_id = ?", "bad-user").Count(&logs).Error)
	require.Zero(t, logs)

	// Without clients configured the action is refused rather than skipped.
	svc.SetOwnerServices(OwnerServices{})
	_, err = svc.UpdateReport(ctx, "admin-1", report.ID, UpdateReportReq{Actions: []string{"delete_content"}})
	require.ErrorContains(t, err, "chat-service client is not configured")
}
