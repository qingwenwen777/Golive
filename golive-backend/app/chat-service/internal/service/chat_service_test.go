package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/chatfilter"
	"github.com/qingwenwen777/golive/pkg/chatlimit"
)

// History used to fall back to matching fan badges by display name, and to
// render the badge/name/level stored with each row (client-supplied before
// the gateway derived them).
func TestDecorateChat_UsesServerDataByUserID(t *testing.T) {
	stored := model.Public{
		Type:      "chat",
		UserID:    "u-2",
		User:      "Big Fan", // spoofed at send time
		Avatar:    "/evil.png",
		UserLevel: 99,
		FanBadge:  &model.FanBadgePayload{CreatorID: "owner-1", Level: 99},
	}
	badges := map[string]*model.FanBadgePayload{"u-1": {CreatorID: "owner-1", Level: 5}}
	profiles := map[string]repo.UserProfile{"u-2": {Name: "Copycat", Avatar: "/c.png", Level: 3}}

	got := decorateChat(stored, badges, profiles)
	require.Nil(t, got.FanBadge, "stored badge is ignored; u-2 has none")
	require.Equal(t, "Copycat", got.User)
	require.Equal(t, "/c.png", got.Avatar)
	require.Equal(t, 3, got.UserLevel)

	stored.UserID = "u-1"
	got = decorateChat(stored, badges, profiles)
	require.Equal(t, &model.FanBadgePayload{CreatorID: "owner-1", Level: 5}, got.FanBadge)
	require.Equal(t, "Big Fan", got.User, "unknown user keeps the stored name")
}

func newTestService(t *testing.T) *ChatService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chat.db")), &gorm.Config{})
	require.NoError(t, err)
	danmus := repo.NewDanmuRepo(db, 2)
	require.NoError(t, danmus.AutoMigrate())
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	f := chatfilter.New([]string{"fuck"}, chatfilter.WithSkipChars(chatfilter.DefaultSkipChars))
	return New(f, chatlimit.New(rdb, "rl:test:", 100, time.Second), danmus, repo.NewPublisher(rdb))
}

// A chat event without a sender name (the gateway could not load the
// profile) was stored and broadcast with the full user id as the name.
func TestProcess_NeverNamesTheSenderByID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	require.NoError(t, svc.Process(ctx, Event{RoomID: "R1", UserID: "3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f", Text: "hi", Ts: 1}))

	rows, err := svc.danmus.History(ctx, "R1", 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Username)
	require.Equal(t, "3f2a0c1e-9b7d-4c1a-8e2f-0a1b2c3d4e5f", rows[0].UserID)
}

// The Kafka path used the client's clientId as the message primary key.
func TestProcess_MessageIDIsServerGenerated(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	serverID := uuid.NewString()

	require.NoError(t, svc.Process(ctx, Event{ID: serverID, RoomID: "R1", UserID: "u1", Text: "ｆｕｃｋ", Ts: 1}))
	require.NoError(t, svc.Process(ctx, Event{ID: "victim-message-id", RoomID: "R1", UserID: "u1", Text: "hi", Ts: 2}))

	rows, err := svc.danmus.History(ctx, "R1", 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	ids := map[string]string{}
	for _, row := range rows {
		ids[row.Text] = row.ID
	}
	require.Equal(t, serverID, ids["***"], "text is masked and the gateway's uuid kept")
	require.NotEqual(t, "victim-message-id", ids["hi"])
	_, err = uuid.Parse(ids["hi"])
	require.NoError(t, err)
}
