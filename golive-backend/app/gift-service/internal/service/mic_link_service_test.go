package service_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

func newMicLinkSvc(t *testing.T) (*service.MicLinkService, *gorm.DB) {
	t.Helper()
	db := newTestDB(t, 0)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return service.NewMicLinkService(repo.NewOrderRepo(db), rdb), db
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
