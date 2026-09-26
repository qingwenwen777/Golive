package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/miclink"
)

func newMicLinkSvc(t *testing.T) (*service.MicLinkService, *gorm.DB) {
	t.Helper()
	svc, db, _ := newMicLinkSvcWithRedis(t)
	return svc, db
}

func newMicLinkSvcWithRedis(t *testing.T) (*service.MicLinkService, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	db := newTestDB(t, 0)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return service.NewMicLinkService(repo.NewOrderRepo(db), rdb), db, mr
}

// approvedMicGuest enables mic link on room "r" and puts guestID on air.
func approvedMicGuest(t *testing.T, svc *service.MicLinkService, guestID string) {
	t.Helper()
	ctx := context.Background()
	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)
	_, err = svc.Request(ctx, guestID, "r")
	require.NoError(t, err)
	_, err = svc.Approve(ctx, "u-owner", "r", guestID)
	require.NoError(t, err)
}

func TestMicLink_ApproveIssuesPublishTokenOnlyToGuest(t *testing.T) {
	ctx := context.Background()
	svc, _, mr := newMicLinkSvcWithRedis(t)
	approvedMicGuest(t, svc, "u-demo")

	key := miclink.TokenKey(miclink.StreamName("r", "u-demo"))
	stored, err := mr.Get(key)
	require.NoError(t, err)
	require.Len(t, stored, 48)
	require.Greater(t, mr.TTL(key), time.Duration(0))

	view, err := svc.Latest(ctx, "r", "u-demo")
	require.NoError(t, err)
	require.Equal(t, service.MicStatusOnAir, view.MyStatus)
	require.Equal(t, stored, view.MyPublishToken)

	for _, other := range []string{"u-owner", "u-other", ""} {
		view, err := svc.Latest(ctx, "r", other)
		require.NoError(t, err)
		require.Empty(t, view.MyPublishToken, "viewer %q", other)
	}
}

func TestMicLink_PublishTokenRevokedWhenGuestLeavesOrIsRemoved(t *testing.T) {
	ctx := context.Background()
	key := miclink.TokenKey(miclink.StreamName("r", "u-demo"))

	cases := map[string]func(*service.MicLinkService) error{
		"leave": func(svc *service.MicLinkService) error {
			_, err := svc.Leave(ctx, "u-demo", "r")
			return err
		},
		"remove": func(svc *service.MicLinkService) error {
			_, err := svc.Remove(ctx, "u-owner", "r", "u-demo")
			return err
		},
		"disable": func(svc *service.MicLinkService) error {
			_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: false})
			return err
		},
	}
	for name, end := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _, mr := newMicLinkSvcWithRedis(t)
			approvedMicGuest(t, svc, "u-demo")
			require.True(t, mr.Exists(key))

			require.NoError(t, end(svc))
			require.False(t, mr.Exists(key))

			// A former guest's poll must not mint a new token.
			view, err := svc.Latest(ctx, "r", "u-demo")
			require.NoError(t, err)
			require.Empty(t, view.MyPublishToken)
			require.False(t, mr.Exists(key))
		})
	}
}

func TestMicLink_ExpiredPublishTokenReissuedForOnAirGuest(t *testing.T) {
	ctx := context.Background()
	svc, _, mr := newMicLinkSvcWithRedis(t)
	approvedMicGuest(t, svc, "u-demo")
	key := miclink.TokenKey(miclink.StreamName("r", "u-demo"))
	first, err := mr.Get(key)
	require.NoError(t, err)

	mr.FastForward(time.Hour)
	require.False(t, mr.Exists(key))

	view, err := svc.Latest(ctx, "r", "u-demo")
	require.NoError(t, err)
	require.NotEmpty(t, view.MyPublishToken)
	require.NotEqual(t, first, view.MyPublishToken)
	stored, err := mr.Get(key)
	require.NoError(t, err)
	require.Equal(t, stored, view.MyPublishToken)
}

func TestMicLink_RequestRequiresEnabled(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Request(ctx, "u-demo", "r")
	require.ErrorIs(t, err, service.ErrMicLinkDisabled)

	_, err = svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)

	view, err := svc.Request(ctx, "u-demo", "r")
	require.NoError(t, err)
	require.Equal(t, service.MicStatusPending, view.MyStatus)
}

func TestMicLink_OnlyOwnerConfigures(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Config(ctx, "u-demo", service.MicConfigReq{RoomID: "r", Enabled: true})
	require.ErrorIs(t, err, service.ErrMicLinkForbidden)
}

func TestMicLink_ApproveMovesToRosterAndCapsAtThree(t *testing.T) {
	ctx := context.Background()
	svc, db := newMicLinkSvc(t)
	seedMicUsers(t, db, "g1", "g2", "g3", "g4")

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)

	for _, g := range []string{"g1", "g2", "g3", "g4"} {
		_, err := svc.Request(ctx, g, "r")
		require.NoError(t, err)
	}

	for _, g := range []string{"g1", "g2", "g3"} {
		_, err := svc.Approve(ctx, "u-owner", "r", g)
		require.NoError(t, err)
	}

	// Fourth approval exceeds the cap of 3.
	_, err = svc.Approve(ctx, "u-owner", "r", "g4")
	require.ErrorIs(t, err, service.ErrMicLinkSlotFull)

	view, err := svc.Latest(ctx, "r", "u-owner")
	require.NoError(t, err)
	require.Len(t, view.Roster, service.MicLinkSlotMax)
	require.True(t, view.IsOwner)
}

func TestMicLink_DuplicateRequestRejected(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)
	_, err = svc.Request(ctx, "u-demo", "r")
	require.NoError(t, err)
	_, err = svc.Request(ctx, "u-demo", "r")
	require.ErrorIs(t, err, service.ErrMicLinkRequestExists)
}

func TestMicLink_GuestMuteAndLeave(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)
	_, err = svc.Request(ctx, "u-demo", "r")
	require.NoError(t, err)
	_, err = svc.Approve(ctx, "u-owner", "r", "u-demo")
	require.NoError(t, err)

	view, err := svc.Mute(ctx, "u-demo", "r", true)
	require.NoError(t, err)
	require.True(t, view.MyMuted)
	require.Equal(t, service.MicStatusOnAir, view.MyStatus)

	view, err = svc.Leave(ctx, "u-demo", "r")
	require.NoError(t, err)
	require.Equal(t, service.MicStatusNone, view.MyStatus)
	require.Empty(t, view.Roster)
}

func TestMicLink_OwnerRemoveGuest(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)
	_, err = svc.Request(ctx, "u-demo", "r")
	require.NoError(t, err)
	_, err = svc.Approve(ctx, "u-owner", "r", "u-demo")
	require.NoError(t, err)

	_, err = svc.Remove(ctx, "u-owner", "r", "u-demo")
	require.NoError(t, err)

	view, err := svc.Latest(ctx, "r", "u-demo")
	require.NoError(t, err)
	require.Equal(t, service.MicStatusNone, view.MyStatus)
	require.Empty(t, view.Roster)
}

func TestMicLink_DisableClearsRosterAndRequests(t *testing.T) {
	ctx := context.Background()
	svc, _ := newMicLinkSvc(t)

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "all"})
	require.NoError(t, err)
	_, err = svc.Request(ctx, "u-demo", "r")
	require.NoError(t, err)
	_, err = svc.Approve(ctx, "u-owner", "r", "u-demo")
	require.NoError(t, err)

	_, err = svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: false})
	require.NoError(t, err)

	view, err := svc.Latest(ctx, "r", "u-owner")
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.Empty(t, view.Roster)
	require.Empty(t, view.Requests)
}

func TestMicLink_FollowersOnlyEligibility(t *testing.T) {
	ctx := context.Background()
	svc, db := newMicLinkSvc(t)
	seedMicUsers(t, db, "g-follow")
	// The shared test schema models rooms.channel but not channel_id (which
	// RoomChannelID reads); add it so the followers check can run.
	_ = db.Exec("ALTER TABLE rooms ADD COLUMN channel_id VARCHAR(64) NOT NULL DEFAULT ''").Error
	require.NoError(t, db.Exec("UPDATE rooms SET channel_id = ? WHERE id = ?", "ch-owner", "r").Error)

	_, err := svc.Config(ctx, "u-owner", service.MicConfigReq{RoomID: "r", Enabled: true, Eligibility: "followers"})
	require.NoError(t, err)

	// Not following yet → rejected.
	_, err = svc.Request(ctx, "g-follow", "r")
	require.ErrorIs(t, err, service.ErrMicLinkNotEligible)
}

func seedMicUsers(t *testing.T, db *gorm.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, username, display_name, avatar, coin_balance) VALUES (?, ?, ?, ?, ?)",
			id, id, id, "", int64(0),
		).Error)
	}
}
