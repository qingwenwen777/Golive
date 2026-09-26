package service

import (
	"context"
	"strings"
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
	svc.unpublishGrace = 0
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

// srsReq builds the hook body SRS sends when OBS publishes with obsKey
// ("<roomID>?key=<secret>"): SRS splits the query string into Param, and a
// transcoded variant appends its suffix to the stream name.
func srsReq(obsKey, variant string) SRSPublishReq {
	return srsClientReq(obsKey, variant, "")
}

func srsClientReq(obsKey, variant, clientID string) SRSPublishReq {
	stream, query, _ := strings.Cut(obsKey, "?")
	param := ""
	if query != "" {
		param = "?" + query
	}
	return SRSPublishReq{App: "live", Stream: stream + variant, Param: param, ClientID: clientID}
}

// publishSecret returns the secret half of an OBS stream key.
func publishSecret(obsKey string) string {
	return publishKeyFromParam(srsReq(obsKey, "").Param)
}

func publishTestLive(t *testing.T, svc *LiveService, streamKey string) {
	t.Helper()
	require.NoError(t, svc.OnPublish(context.Background(), srsReq(streamKey, "")))
}

func publishTestLiveClient(t *testing.T, svc *LiveService, streamKey, clientID string) {
	t.Helper()
	require.NoError(t, svc.OnPublish(context.Background(), srsClientReq(streamKey, "", clientID)))
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

func TestRoomServiceListUsesRealtimeViewerMetrics(t *testing.T) {
	ctx := context.Background()
	_, rooms, live, rdb := newLiveServiceTestDepsWithRedis(t)
	startedAt := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)

	require.NoError(t, rooms.Upsert(ctx, &model.Room{
		ID:          "live-metrics",
		Title:       "Viewer count check",
		Channel:     "kabun",
		ChannelID:   "ch-owner-metrics",
		Category:    "Gaming",
		Cover:       "/uploads/covers/live.webp",
		Viewers:     0,
		PeakViewers: 1,
		StartedAt:   startedAt,
		Status:      model.StatusLive,
		OwnerID:     "owner-metrics",
		StreamKey:   "lk_metrics",
	}))
	require.NoError(t, rdb.HSet(ctx, "roommetrics:live-metrics", "viewers", "7", "peak", "9").Err())

	roomSvc := NewRoomService(rooms, "http://srs/live")
	roomSvc.SetLiveRepo(live)
	roomSvc.now = func() time.Time { return startedAt.Add(3 * time.Minute) }

	resp, err := roomSvc.List(ctx, "", "", 1, 10)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, int64(7), resp.Items[0].Viewers)
	require.Equal(t, int64(9), resp.Items[0].PeakViewers)
	require.Equal(t, "http://srs/live/live-metrics.flv", resp.Items[0].PlaybackURL)
}

func TestUpdateLiveMetadataEditsActiveRoomAndBroadcasts(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _, rdb := newLiveServiceTestDepsWithRedis(t)
	st := startTestLive(t, svc, "owner-edit")

	sub := rdb.Subscribe(ctx, "room:"+st.ID)
	defer func() { require.NoError(t, sub.Close()) }()
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	updated, err := svc.UpdateLiveMetadata(ctx, "owner-edit", UpdateLiveReq{
		Title:       " Better title ",
		Description: "Updated description",
		Cover:       " /uploads/covers/live-edit.webp ",
	})
	require.NoError(t, err)
	require.Equal(t, "Better title", updated.Title)
	require.Equal(t, "Updated description", updated.Description)
	require.Equal(t, "/uploads/covers/live-edit.webp", updated.Cover)
	require.Equal(t, st.StreamKey, updated.StreamKey)

	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, "Better title", room.Title)
	require.Equal(t, "Updated description", room.Description)
	require.Equal(t, "/uploads/covers/live-edit.webp", room.Cover)

	select {
	case msg := <-sub.Channel():
		require.Contains(t, msg.Payload, `"type":"room_updated"`)
		require.Contains(t, msg.Payload, `"title":"Better title"`)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for room_updated event")
	}
}

func TestOnPublishIsIdempotentAndDoesNotResetStartedAt(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	st := startTestLive(t, svc, "owner-publish")

	firstPublish := base.Add(time.Minute)
	svc.now = func() time.Time { return firstPublish }
	require.NoError(t, svc.OnPublish(ctx, srsReq(st.StreamKey, "")))

	secondPublish := firstPublish.Add(time.Hour)
	svc.now = func() time.Time { return secondPublish }
	require.NoError(t, svc.OnPublish(ctx, srsReq(st.StreamKey, "")))

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
	require.NoError(t, svc.OnPublish(ctx, srsReq(st.StreamKey, "")))
	require.NoError(t, svc.OnPublish(ctx, srsReq(st.StreamKey, "_q720")))
	require.NoError(t, svc.OnPublish(ctx, srsReq(st.StreamKey, "_q480")))

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

	err := svc.OnPublish(ctx, srsReq(st.StreamKey, "_q360"))
	require.Error(t, err)
}

func TestStopLiveDeletesStreamKeyAndEndsRoom(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-1")
	publishTestLive(t, svc, st.StreamKey)

	_, err := live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)

	require.NoError(t, svc.StopLive(ctx, "owner-1"))

	_, err = live.Resolve(ctx, publishSecret(st.StreamKey))
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
	require.NoError(t, live.Save(ctx, publishSecret(st.StreamKey), st.ID, time.Hour))

	err := svc.OnPublish(ctx, srsReq(st.StreamKey, ""))
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
	require.NoError(t, svc.OnUnpublish(ctx, srsReq(st.StreamKey, "")))
	svc.now = func() time.Time { return secondEnd }
	require.NoError(t, svc.OnUnpublish(ctx, srsReq(st.StreamKey, "")))
	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{Stream: ""}))

	_, err := live.Resolve(ctx, publishSecret(st.StreamKey))
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.NotNil(t, room.EndedAt)
	require.True(t, firstEnd.Equal(*room.EndedAt))
}

func TestOnUnpublishGraceAllowsPublisherReconnect(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	svc.unpublishGrace = 20 * time.Millisecond
	st := startTestLive(t, svc, "owner-reconnect-grace")
	publishTestLiveClient(t, svc, st.StreamKey, "old-client")

	require.NoError(t, svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "old-client")))

	_, err := live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)

	publishTestLiveClient(t, svc, st.StreamKey, "new-client")
	time.Sleep(60 * time.Millisecond)

	_, err = live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	room, err = rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.Nil(t, room.EndedAt)
}

func TestOnUnpublishGraceEndsWhenPublisherDoesNotReconnect(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	svc.unpublishGrace = 20 * time.Millisecond
	st := startTestLive(t, svc, "owner-no-reconnect")
	publishTestLiveClient(t, svc, st.StreamKey, "gone-client")

	require.NoError(t, svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "gone-client")))
	time.Sleep(60 * time.Millisecond)

	_, err := live.Resolve(ctx, publishSecret(st.StreamKey))
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
	room, err := rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
	require.NotNil(t, room.EndedAt)
}

func TestOnUnpublishVariantDoesNotEndRoom(t *testing.T) {
	ctx := context.Background()
	svc, rooms, live := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-unpublish-variant")
	publishTestLive(t, svc, st.StreamKey)

	require.NoError(t, svc.OnUnpublish(ctx, srsReq(st.StreamKey, "_q720")))

	_, err := live.Resolve(ctx, publishSecret(st.StreamKey))
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
	require.NoError(t, live.Save(ctx, publishSecret(old.StreamKey), old.ID, time.Hour))

	require.NoError(t, svc.OnUnpublish(ctx, srsReq(old.StreamKey, "")))

	room, err := rooms.GetByID(ctx, next.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.Equal(t, publishSecret(next.StreamKey), room.StreamKey)
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

func TestRoomGetExposesStartingAppointmentToViewers(t *testing.T) {
	ctx := context.Background()
	_, rooms, _ := newLiveServiceTestDeps(t)
	roomSvc := NewRoomService(rooms, "http://srs/live")
	room := &model.Room{
		ID:          "appt-owner-appt-starting-test",
		OwnerID:     "owner-appt-starting",
		ChannelID:   "ch-owner-appt-starting",
		Channel:     "Appointment Creator",
		Title:       "Starting appointment",
		Description: "Appointment room entering the start window",
		Category:    "Gaming",
		Cover:       "/uploads/appointment.jpg",
		StartedAt:   time.Now(),
		Status:      model.StatusPublishing,
	}
	require.NoError(t, rooms.Upsert(ctx, room))

	viewerView, err := roomSvc.Get(ctx, room.ID, "viewer-appt-starting")
	require.NoError(t, err)
	require.False(t, viewerView.IsLive)
	require.Equal(t, model.StatusPublishing, viewerView.Status)
	require.Empty(t, viewerView.StreamKey)
	require.Empty(t, viewerView.PlaybackURL)
}

func TestPlaybackURLDoesNotExposePublishKey(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	st := startTestLive(t, svc, "owner-secret")
	publishTestLive(t, svc, st.StreamKey)
	secret := publishSecret(st.StreamKey)
	require.NotEmpty(t, secret)
	require.Equal(t, st.ID+"?key="+secret, st.StreamKey)

	roomSvc := NewRoomService(rooms, "http://srs/live")
	viewerView, err := roomSvc.Get(ctx, st.ID, "")
	require.NoError(t, err)
	require.Equal(t, "http://srs/live/"+st.ID+".flv", viewerView.PlaybackURL)
	require.NotContains(t, viewerView.PlaybackURL, secret)
	require.Empty(t, viewerView.StreamKey)

	list, err := roomSvc.List(ctx, "", "", 1, 10)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.NotContains(t, list.Items[0].PlaybackURL, secret)
}

func TestOnPublishRequiresMatchingKey(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	victim := startTestLive(t, svc, "owner-victim")
	other := startTestLive(t, svc, "owner-other")

	cases := map[string]SRSPublishReq{
		"no key":            {App: "live", Stream: victim.ID},
		"wrong key":         {App: "live", Stream: victim.ID, Param: "?key=lk_guess"},
		"other room's key":  srsReq(victim.ID+"?key="+publishSecret(other.StreamKey), ""),
		"key as stream":     {App: "live", Stream: publishSecret(victim.StreamKey)},
		"variant, no key":   {App: "live", Stream: victim.ID + "_q720"},
		"empty stream name": {App: "live", Param: "?key=" + publishSecret(victim.StreamKey)},
	}
	for name, req := range cases {
		require.Error(t, svc.OnPublish(ctx, req), name)
	}

	room, err := rooms.GetByID(ctx, victim.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPublishing, room.Status)
}

func TestOnUnpublishIgnoresMissingOrMismatchedKey(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _ := newLiveServiceTestDeps(t)
	victim := startTestLive(t, svc, "owner-victim-unpub")
	other := startTestLive(t, svc, "owner-other-unpub")
	publishTestLive(t, svc, victim.StreamKey)

	require.NoError(t, svc.OnUnpublish(ctx, SRSPublishReq{App: "live", Stream: victim.ID}))
	require.NoError(t, svc.OnUnpublish(ctx, srsReq(victim.ID+"?key="+publishSecret(other.StreamKey), "")))

	room, err := rooms.GetByID(ctx, victim.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusLive, room.Status)
	require.Nil(t, room.EndedAt)
}
