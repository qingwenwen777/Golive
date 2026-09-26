package service_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// fakeTextPolicy mirrors ModerationService: restricted users get their
// sanction's reason, and any text containing word is a blocked word.
type fakeTextPolicy struct {
	restricted map[string]string
	word       string
}

func (p *fakeTextPolicy) EnsureTextAllowed(_ context.Context, texts ...string) error {
	for _, text := range texts {
		if p.word != "" && strings.Contains(text, p.word) {
			return errcode.New(http.StatusBadRequest, "content contains blocked word").WithReason("blocked_word")
		}
	}
	return nil
}

func (p *fakeTextPolicy) EnsureUserCanInteract(_ context.Context, userID string) error {
	if reason := p.restricted[userID]; reason != "" {
		return errcode.New(http.StatusForbidden, "user is restricted").WithReason(reason)
	}
	return nil
}

func TestDirectMessagesApplySiteSanctionsAndBlockedWords(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	policy := &fakeTextPolicy{restricted: map[string]string{}, word: "badword"}
	fx.msgSvc.SetTextPolicy(policy)
	require.NoError(t, fx.social.Follow(ctx, "fan-1", "ch-creator-1"))

	policy.restricted["fan-1"] = "site_muted"
	_, err := fx.msgSvc.SendDirect(ctx, "fan-1", service.SendDirectReq{CreatorID: "creator-1", Content: "hi"})
	requireReason(t, err, "site_muted")
	policy.restricted["fan-1"] = "user_banned"
	_, err = fx.msgSvc.SendDirect(ctx, "fan-1", service.SendDirectReq{CreatorID: "creator-1", Content: "hi"})
	requireReason(t, err, "user_banned")
	delete(policy.restricted, "fan-1")
	_, err = fx.msgSvc.SendDirect(ctx, "fan-1", service.SendDirectReq{CreatorID: "creator-1", Content: "a badword here"})
	requireReason(t, err, "blocked_word")

	thread, err := fx.msgSvc.SendDirect(ctx, "fan-1", service.SendDirectReq{CreatorID: "creator-1", Content: "hi"})
	require.NoError(t, err)

	policy.restricted["creator-1"] = "site_muted"
	_, err = fx.msgSvc.SendThreadMessage(ctx, "creator-1", thread.ID, "reply")
	requireReason(t, err, "site_muted")
	delete(policy.restricted, "creator-1")
	_, err = fx.msgSvc.SendThreadMessage(ctx, "creator-1", thread.ID, "badword reply")
	requireReason(t, err, "blocked_word")
	_, err = fx.msgSvc.SendThreadMessage(ctx, "creator-1", thread.ID, "reply")
	require.NoError(t, err)
}

func TestFanGroupMessagesApplySiteSanctionsAndBlockedWords(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	policy := &fakeTextPolicy{restricted: map[string]string{}, word: "badword"}
	fx.msgSvc.SetTextPolicy(policy)
	require.NoError(t, fx.db.Exec(`CREATE TABLE fan_badges (user_id TEXT NOT NULL, creator_id TEXT NOT NULL, level INTEGER DEFAULT 1, created_at DATETIME, updated_at DATETIME)`).Error)
	require.NoError(t, fx.db.Exec(`INSERT INTO fan_badges (user_id, creator_id, created_at, updated_at) VALUES ('fan-1', 'creator-1', ?, ?)`, time.Now(), time.Now()).Error)
	groups, err := fx.msgSvc.SyncFanGroups(ctx, "creator-1")
	require.NoError(t, err)
	require.Len(t, groups.Items, 1)
	groupID := groups.Items[0].ID

	policy.restricted["fan-1"] = "site_muted"
	_, err = fx.msgSvc.SendFanGroupMessage(ctx, "fan-1", groupID, "hello")
	requireReason(t, err, "site_muted")
	policy.restricted["fan-1"] = "user_banned"
	_, err = fx.msgSvc.SendFanGroupMessage(ctx, "fan-1", groupID, "hello")
	requireReason(t, err, "user_banned")
	delete(policy.restricted, "fan-1")
	_, err = fx.msgSvc.SendFanGroupMessage(ctx, "fan-1", groupID, "badword")
	requireReason(t, err, "blocked_word")
	_, err = fx.msgSvc.SendFanGroupMessage(ctx, "fan-1", groupID, "hello")
	require.NoError(t, err)
}

func TestCreatePostRejectsSanctionedCreators(t *testing.T) {
	fx := newBlockFixture(t)
	ctx := context.Background()
	policy := &fakeTextPolicy{restricted: map[string]string{"creator-1": "user_banned", "creator-2": "site_muted"}}
	fx.posts.SetTextPolicy(policy)

	_, err := fx.posts.CreatePost(ctx, "creator-1", service.CreatePostReq{Content: "back again"})
	requireReason(t, err, "user_banned")
	_, err = fx.posts.CreatePost(ctx, "creator-2", service.CreatePostReq{Content: "still here"})
	requireReason(t, err, "site_muted")

	delete(policy.restricted, "creator-1")
	_, err = fx.posts.CreatePost(ctx, "creator-1", service.CreatePostReq{Content: "hello"})
	require.NoError(t, err)
}
