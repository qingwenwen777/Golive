package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// SRS 5 calls on_publish before it refuses a second publisher of a stream
// that is already published ("stream busy"), so the hook also sees attempts
// that never go on air: a second encoder, or OBS retrying, with the key of a
// live that is on air. SRS 5.0.213 then reports the refused attempt's
// unpublish; 5.0.225 reports nothing.

// busyLive starts a live whose publisher client-A SRS lists as on air.
func busyLive(t *testing.T, ownerID string) (*lifecycleTestEnv, *fakeSRS, *model.Stream) {
	t.Helper()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	env.svc.SetSRSAPIBase(base)
	st := startTestLive(t, env.svc, ownerID)
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-A")
	srs.setPublisher(publishStream(st.StreamKey), "client-A")
	return env, srs, st
}

// publishWhileSRSDown has client-B's on_publish arrive while SRS's API does
// not answer, so the hook cannot tell the stream is busy and takes it.
func publishWhileSRSDown(t *testing.T, env *lifecycleTestEnv, srs *fakeSRS, st *model.Stream) {
	t.Helper()
	srs.setDown(true)
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-B")
	srs.setDown(false)
}

func requirePublishSession(t *testing.T, env *lifecycleTestEnv, st *model.Stream, clientID string) {
	t.Helper()
	got, err := env.live.PublishSession(context.Background(), publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, clientID, got)
}

func TestPublishAttemptOnBusyStreamIsRefused(t *testing.T) {
	ctx := context.Background()
	env, srs, st := busyLive(t, "owner-busy")
	pending := repo.PublishDisconnect{ClientID: "client-A", At: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), Hook: true}
	require.NoError(t, env.live.SaveDisconnect(ctx, st.ID, pending, time.Hour))

	require.Error(t, env.svc.OnPublish(ctx, srsClientReq(st.StreamKey, "", "client-B")))
	requirePublishSession(t, env, st, "client-A")
	got, err := env.live.Disconnect(ctx, st.ID)
	require.NoError(t, err)
	require.True(t, pending.At.Equal(got.At), "the refused attempt leaves the pending disconnect alone")

	require.NoError(t, env.svc.StopLive(ctx, "owner-busy"))
	require.Equal(t, []string{"client-A"}, srs.kickedClients())
}

func TestUnpublishOfRefusedPublisherKeepsLiveOnAir(t *testing.T) {
	ctx := context.Background()
	env, srs, st := busyLive(t, "owner-busy-213") // grace 0: finalized right away
	publishWhileSRSDown(t, env, srs, st)
	// SRS 5.0.213 refused B as busy and reports its unpublish; A is on air.
	require.NoError(t, env.svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "client-B")))

	requireRoomStatus(t, env, st.ID, model.StatusLive)
	_, err := env.live.Disconnect(ctx, st.ID)
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
}

func TestStopKicksThePublisherSRSLists(t *testing.T) {
	ctx := context.Background()
	env, srs, st := busyLive(t, "owner-busy-225")
	// SRS 5.0.225 refused B as busy without an unpublish: the session is B's.
	publishWhileSRSDown(t, env, srs, st)
	requirePublishSession(t, env, st, "client-B")

	require.NoError(t, env.svc.ForceStopRoom(ctx, st.ID))
	requireRoomStatus(t, env, st.ID, model.StatusEnded)
	require.Contains(t, srs.kickedClients(), "client-A")
}

func TestReconcilerStoresThePublisherSRSLists(t *testing.T) {
	ctx := context.Background()
	env, srs, st := busyLive(t, "owner-busy-session")
	publishWhileSRSDown(t, env, srs, st)

	env.svc.Reconcile(ctx)
	requirePublishSession(t, env, st, "client-A")
}

// prePlayNameLive stores a live that went live before play names: OBS
// publishes it under the raw key (live/lk_...), without ?key=, and SRS lists
// it under that name. Its playbackUrl now names a play name nobody publishes.
func prePlayNameLive(t *testing.T, env *lifecycleTestEnv, srs *fakeSRS, ownerID string) *model.Room {
	t.Helper()
	ctx := context.Background()
	room := &model.Room{
		ID: "live-" + ownerID, Title: "Old scheme", Status: model.StatusLive, OwnerID: ownerID,
		StreamKey: "lk_0123456789abcdef0123456789abcdef", StartedAt: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		ReplayStatus: model.ReplayStatusNone, ReplayVisibility: model.PostVisibilityPublic,
	}
	require.NoError(t, env.rooms.Upsert(ctx, room))
	require.NoError(t, env.live.Save(ctx, room.StreamKey, room.ID, time.Hour))
	srs.setPublisher(room.StreamKey, "client-old")
	return room
}

func TestReconcilerEndsPrePlayNameLiveAndKicksItsPublisher(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	room := prePlayNameLive(t, env, srs, "owner-old-scheme")
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	// OBS reconnecting with the old stream key is refused and changes nothing.
	old := SRSPublishReq{App: "live", Stream: room.StreamKey, ClientID: "client-old-2"}
	require.Error(t, env.restarted(20*time.Second, t0, base).OnPublish(ctx, old))
	requireRoomStatus(t, env, room.ID, model.StatusLive)
	_, err := env.live.PublishSession(ctx, room.StreamKey)
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)

	env.restarted(20*time.Second, t0, base).Reconcile(ctx)
	env.restarted(20*time.Second, t0.Add(time.Minute), base).Reconcile(ctx)
	requireRoomStatus(t, env, room.ID, model.StatusEnded)
	// Disconnected, OBS stops streaming into nothing and SRS closes the
	// recording the replay upload waits for.
	require.Equal(t, []string{"client-old"}, srs.kickedClients())
}

func TestStopKicksPrePlayNamePublisher(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	env.svc.SetSRSAPIBase(base)
	room := prePlayNameLive(t, env, srs, "owner-old-scheme-stop")

	require.NoError(t, env.svc.ForceStopRoom(ctx, room.ID))
	requireRoomStatus(t, env, room.ID, model.StatusEnded)
	require.Equal(t, []string{"client-old"}, srs.kickedClients())
}
