package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func newLiveServiceTestDeps(t *testing.T) (*LiveService, *repo.RoomRepo, *repo.LiveRepo) {
	t.Helper()
	svc, rooms, live, _ := newLiveServiceTestDepsWithRedis(t)
	return svc, rooms, live
}

func newLiveServiceTestDepsWithRedis(t *testing.T) (*LiveService, *repo.RoomRepo, *repo.LiveRepo, *redis.Client) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	live := repo.NewLiveRepo(rdb)
	svc := NewLiveService(rooms, live, "test-secret", time.Hour, "http://srs/live")
	return svc, rooms, live, rdb
}

func startTestLive(t *testing.T, svc *LiveService, ownerID string) *model.Stream {
	t.Helper()

	st, err := svc.GoLive(context.Background(), ownerID, GoLiveReq{
		Title:       "Bug bash",
		Description: "Testing stream shutdown",
		Category:    "Engineering",
		ChannelName: "Tester",
	})
	require.NoError(t, err)
	require.NotEmpty(t, st.StreamKey)
	return st
}

func publishTestLive(t *testing.T, svc *LiveService, streamKey string) {
	t.Helper()
	require.NoError(t, svc.OnPublish(context.Background(), SRSPublishReq{Stream: streamKey}))
}

func TestGoLiveCreatesPublishingSessionOnly(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-publishing")

	require.False(t, st.IsLive)
	require.Empty(t, st.PlaybackURL)
	require.Equal(t, model.StatusPublishing, st.Status)

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPublishing, room.Status)
}

func TestOnPublishIsIdempotentAndDoesNotResetStartedAt(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	st := startTestLive(t, svc, "owner-publish")

	firstPublish := base.Add(time.Minute)
	svc.now = func() time.Time { return firstPublish }
	require.NoError(t, svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey}))

	secondPublish := firstPublish.Add(time.Hour)
	svc.now = func() time.Time { return secondPublish }
	require.NoError(t, svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey}))

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.True(t, firstPublish.Equal(room.StartedAt))
}

func TestOnPublishAcceptsTranscodedVariants(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	st := startTestLive(t, svc, "owner-variants")

	firstPublish := base.Add(time.Minute)
	svc.now = func() time.Time { return firstPublish }
	require.NoError(t, svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey}))
	require.NoError(t, svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey + "_q720"}))
	require.NoError(t, svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey + "_q480"}))

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.True(t, firstPublish.Equal(room.StartedAt))
}

func TestOnPublishRejectsUnknownTranscodedVariant(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-bad-variant")
	publishTestLive(t, svc, st.StreamKey)

	err := svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey + "_q360"})
	require.Error(t, err)
}

func TestStopLiveDeletesStreamKeyAndEndsRoom(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-1")
	publishTestLive(t, svc, st.StreamKey)

	_, err := live.Resolve(ctx, st.StreamKey)
	require.NoError(t, err)

	require.NoError(t, svc.StopLive(ctx, "owner-1"))

	_, err = live.Resolve(ctx, st.StreamKey)
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
	require.NotNil(t, room.EndedAt)
}

func TestStopLiveBroadcastsEndedEvent(t *testing.T) {
	ctx := context.Background()
	svc, _, _, rdb := newLiveServiceTestDepsWithRedis(t)
	st := startTestLive(t, svc, "owner-broadcast-stop")
	publishTestLive(t, svc, st.StreamKey)

	sub := rdb.Subscribe(ctx, "room:"+st.ID)
	defer func() { require.NoError(t, sub.Close()) }()
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.StopLive(ctx, "owner-broadcast-stop"))

	select {
	case msg := <-sub.Channel():
		require.Contains(t, msg.Payload, `"type":"live_status"`)
		require.Contains(t, msg.Payload, `"status":"ended"`)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ended event")
	}
}

func TestStopLiveIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	firstEnd := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)
	secondEnd := firstEnd.Add(time.Hour)
	st := startTestLive(t, svc, "owner-idem-stop")

	svc.now = func() time.Time { return firstEnd }
	require.NoError(t, svc.StopLive(ctx, "owner-idem-stop"))
	svc.now = func() time.Time { return secondEnd }
	require.NoError(t, svc.StopLive(ctx, "owner-idem-stop"))
	require.NoError(t, svc.StopLive(ctx, "missing-owner"))

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.NotNil(t, room.EndedAt)
	require.True(t, firstEnd.Equal(*room.EndedAt))
}

func TestOnPublishRejectsEndedRoomEvenWithStaleKey(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-2")

	require.NoError(t, svc.StopLive(ctx, "owner-2"))
	require.NoError(t, live.Save(ctx, st.StreamKey, st.ID, time.Hour))

	err := svc.OnPublish(ctx, SRSPublishReq{Stream: st.StreamKey})
	require.Error(t, err)

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
}

func TestOnUnpublishIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	firstEnd := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	secondEnd := firstEnd.Add(time.Hour)
	st := startTestLive(t, svc, "owner-unpublish")
	publishTestLive(t, svc, st.StreamKey)

	svc.now = func() time.Time { return firstEnd }
	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: st.StreamKey}))
	svc.now = func() time.Time { return secondEnd }
	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: st.StreamKey}))
	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: ""}))

	_, err := live.Resolve(ctx, st.StreamKey)
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.NotNil(t, room.EndedAt)
	require.True(t, firstEnd.Equal(*room.EndedAt))
}

func TestOnUnpublishVariantDoesNotEndRoom(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-unpublish-variant")
	publishTestLive(t, svc, st.StreamKey)

	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: st.StreamKey + "_q720"}))

	_, err := live.Resolve(ctx, st.StreamKey)
	require.NoError(t, err)
	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.Nil(t, room.EndedAt)
}

func TestStaleUnpublishDoesNotEndNewSession(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	old := startTestLive(t, svc, "owner-restart")
	publishTestLive(t, svc, old.StreamKey)

	next := startTestLive(t, svc, "owner-restart")
	require.NotEqual(t, old.StreamKey, next.StreamKey)
	publishTestLive(t, svc, next.StreamKey)
	require.NoError(t, live.Save(ctx, old.StreamKey, old.ID, time.Hour))

	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: old.StreamKey}))

	room, err := rooms.GetByID(ctx, next.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.Equal(t, next.StreamKey, room.StreamKey)
}

func TestRoomGetDoesNotExposePublishingOrEndedToViewers(t *testing.T) {
	ctx := context.Background()
	liveSvc, rooms, _ := newLiveServiceTestDeps(t)
	roomSvc := NewRoomService(rooms, "http://srs/live")
	st := startTestLive(t, liveSvc, "owner-get")

	_, err := roomSvc.Get(ctx, st.ID, "")
	require.ErrorIs(t, err, ErrRoomNotFound)

	ownerView, err := roomSvc.Get(ctx, st.ID, "owner-get")
	require.NoError(t, err)
	require.False(t, ownerView.IsLive)
	require.Equal(t, model.StatusPublishing, ownerView.Status)
	require.Equal(t, st.StreamKey, ownerView.StreamKey)
	require.Empty(t, ownerView.PlaybackURL)

	require.NoError(t, liveSvc.StopLive(ctx, "owner-get"))
	_, err = roomSvc.Get(ctx, st.ID, "owner-get")
	require.ErrorIs(t, err, ErrRoomNotFound)
}
