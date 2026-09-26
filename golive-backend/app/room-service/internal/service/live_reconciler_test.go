package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// fakeSRS serves SRS 5's GET /api/v1/streams/ (paged by start/count, count
// at least 10, every stream listed with its publish state) and DELETE
// /api/v1/clients/{id}.
type fakeSRS struct {
	mu          sync.Mutex
	publishers  map[string]string // stream name -> publisher client id
	idle        []string          // streams listed without a publisher
	down        bool
	ignoreStart bool
	listCalls   int
}

func newFakeSRS(t *testing.T) (*fakeSRS, string) {
	t.Helper()
	f := &fakeSRS{publishers: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeSRS) setPublisher(stream, clientID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if clientID == "" {
		delete(f.publishers, stream)
		return
	}
	f.publishers[stream] = clientID
}

func (f *fakeSRS) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *fakeSRS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		http.Error(w, "down", http.StatusBadGateway)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/streams/":
		f.listCalls++
		type publish struct {
			Active bool   `json:"active"`
			CID    string `json:"cid,omitempty"`
		}
		type stream struct {
			ID      string  `json:"id"`
			Name    string  `json:"name"`
			App     string  `json:"app"`
			Clients int     `json:"clients"`
			Publish publish `json:"publish"`
		}
		var all []stream
		for name, cid := range f.publishers {
			all = append(all, stream{ID: "vid-" + name, Name: name, App: "live", Clients: 1, Publish: publish{Active: true, CID: cid}})
		}
		for _, name := range f.idle {
			all = append(all, stream{ID: "vid-" + name, Name: name, App: "live", Clients: 1})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
		start, _ := strconv.Atoi(r.URL.Query().Get("start"))
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		count = max(count, 10)
		if f.ignoreStart || start < 0 {
			start = 0
		}
		page := []stream{}
		for i := start; i < len(all) && len(page) < count; i++ {
			page = append(page, all[i])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "server": "vid-test", "streams": page})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/clients/"):
		_, _ = w.Write([]byte(`{"code":0}`))
	default:
		http.NotFound(w, r)
	}
}

// restarted returns a second LiveService over env's stores, like the process
// after a restart: pending grace timers of env.svc are not carried over.
func (e *lifecycleTestEnv) restarted(grace time.Duration, now time.Time, srsBase string) *LiveService {
	svc := NewLiveService(e.rooms, e.live, "test-secret", time.Hour, "http://srs/live")
	svc.unpublishGrace = grace
	svc.now = func() time.Time { return now }
	svc.SetSRSAPIBase(srsBase)
	return svc
}

func requireRoomStatus(t *testing.T, env *lifecycleTestEnv, roomID, status string) *model.Room {
	t.Helper()
	room, err := env.rooms.GetByID(context.Background(), roomID)
	require.NoError(t, err)
	require.Equal(t, status, room.Status)
	return room
}

func TestOnUnpublishEndsRoomAfterStreamKeyExpired(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	st := startTestLive(t, env.svc, "owner-long-live")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-long")

	// Streaming for longer than stream_key_ttl without the reconciler.
	env.mr.FastForward(2 * time.Hour)
	require.NoError(t, env.svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "client-long")))

	requireRoomStatus(t, env, st.ID, model.StatusEnded)
	_, err := env.live.Resolve(ctx, publishSecret(st.StreamKey))
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
}

func TestPublisherReconnectsAfterStreamKeyExpired(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	st := startTestLive(t, env.svc, "owner-reconnect-late")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-before")

	env.mr.FastForward(2 * time.Hour)
	require.NoError(t, env.svc.OnPublish(ctx, srsClientReq(st.StreamKey, "", "client-after")))

	requireRoomStatus(t, env, st.ID, model.StatusLive)
	roomID, err := env.live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, st.ID, roomID)
	client, err := env.live.PublishSession(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, "client-after", client)
}

func TestExpiredKeyDoesNotAuthorizePublishingRoom(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	st := startTestLive(t, env.svc, "owner-never-published")

	env.mr.FastForward(2 * time.Hour)
	require.Error(t, env.svc.OnPublish(ctx, srsReq(st.StreamKey, "")))
	requireRoomStatus(t, env, st.ID, model.StatusPublishing)
}

func TestReconcilerKeepsLongLiveKeysAlive(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	env.svc.SetSRSAPIBase(base)
	st := startTestLive(t, env.svc, "owner-marathon")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-marathon")
	srs.setPublisher(st.ID, "client-marathon")

	for range 5 {
		env.mr.FastForward(50 * time.Minute)
		env.svc.Reconcile(ctx)
	}

	requireRoomStatus(t, env, st.ID, model.StatusLive)
	roomID, err := env.live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, st.ID, roomID)
	client, err := env.live.PublishSession(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, "client-marathon", client)
}

func TestReconcilerRestoresPublishSessionFromSRS(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	env.svc.SetSRSAPIBase(base)
	st := startTestLive(t, env.svc, "owner-session")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-session")
	srs.setPublisher(st.ID, "client-session")
	require.NoError(t, env.live.DeletePublishSession(ctx, publishSecret(st.StreamKey)))

	env.svc.Reconcile(ctx)

	client, err := env.live.PublishSession(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)
	require.Equal(t, "client-session", client)
}

func TestReconcilerFinishesUnpublishGraceAfterRestart(t *testing.T) {
	for name, withSRS := range map[string]bool{"srs unreachable": false, "srs reachable": true} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			env := newLifecycleTestEnv(t)
			srs, base := newFakeSRS(t)
			srs.setDown(!withSRS)
			t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
			env.svc.now = func() time.Time { return t0 }
			// The grace timer of this process never fires: it "restarts" first.
			env.svc.unpublishGrace = time.Hour
			st := startTestLive(t, env.svc, "owner-restart-grace")
			publishTestLiveClient(t, env.svc, st.StreamKey, "client-gone")
			require.NoError(t, env.svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "client-gone")))

			env.restarted(20*time.Second, t0.Add(10*time.Second), base).Reconcile(ctx)
			requireRoomStatus(t, env, st.ID, model.StatusLive)

			env.restarted(20*time.Second, t0.Add(30*time.Second), base).Reconcile(ctx)
			room := requireRoomStatus(t, env, st.ID, model.StatusEnded)
			require.NotNil(t, room.EndedAt)
			require.True(t, t0.Equal(*room.EndedAt))
			_, err := env.live.Resolve(ctx, publishSecret(st.StreamKey))
			require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
		})
	}
}

func TestReconcilerEndsLiveRoomMissingFromSRS(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	_, base := newFakeSRS(t)
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	env.svc.now = func() time.Time { return t0 }
	st := startTestLive(t, env.svc, "owner-lost-hook")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-lost")

	sub := env.rdb.Subscribe(ctx, "room:"+st.ID)
	defer func() { require.NoError(t, sub.Close()) }()
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	// SRS has no publisher for the room and on_unpublish never arrived.
	env.restarted(20*time.Second, t0.Add(time.Minute), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusLive)
	env.restarted(20*time.Second, t0.Add(time.Minute+10*time.Second), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusLive)

	env.restarted(20*time.Second, t0.Add(2*time.Minute), base).Reconcile(ctx)
	room := requireRoomStatus(t, env, st.ID, model.StatusEnded)
	require.True(t, t0.Add(time.Minute).Equal(*room.EndedAt))
	require.Equal(t, 1, countEnded(collectRoomEvents(t, sub)))
	_, err = env.live.Resolve(ctx, publishSecret(st.StreamKey))
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
}

func TestReconcilerKeepsPublisherReconnectingWithinGrace(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	env.svc.now = func() time.Time { return t0 }
	env.svc.unpublishGrace = time.Hour
	st := startTestLive(t, env.svc, "owner-flaky")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-1")
	require.NoError(t, env.svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "client-1")))

	// Within the grace period, with SRS showing no publisher yet.
	svc := env.restarted(20*time.Second, t0.Add(10*time.Second), base)
	svc.Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusLive)

	// The publisher is back before the grace period ends.
	require.NoError(t, svc.OnPublish(ctx, srsClientReq(st.StreamKey, "", "client-2")))
	srs.setPublisher(st.ID, "client-2")
	env.restarted(20*time.Second, t0.Add(5*time.Minute), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusLive)

	// A late on_unpublish of the replaced publisher does not end it either.
	require.NoError(t, env.svc.OnUnpublish(ctx, srsClientReq(st.StreamKey, "", "client-1")))
	for _, reachable := range []bool{false, true} {
		srs.setDown(!reachable)
		env.restarted(20*time.Second, t0.Add(10*time.Minute), base).Reconcile(ctx)
		requireRoomStatus(t, env, st.ID, model.StatusLive)
	}
}

func TestReconcilerDoesNotEndRoomsWhileSRSUnreachable(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	srs, base := newFakeSRS(t)
	srs.setDown(true)
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	env.svc.now = func() time.Time { return t0 }
	st := startTestLive(t, env.svc, "owner-srs-outage")
	publishTestLiveClient(t, env.svc, st.StreamKey, "client-outage")

	for i := range 5 {
		env.mr.FastForward(50 * time.Minute)
		env.restarted(20*time.Second, t0.Add(time.Duration(i+1)*50*time.Minute), base).Reconcile(ctx)
	}
	requireRoomStatus(t, env, st.ID, model.StatusLive)
	_, err := env.live.Resolve(ctx, publishSecret(st.StreamKey))
	require.NoError(t, err)

	// SRS answers again, without the stream: the room ends after the grace.
	srs.setDown(false)
	env.restarted(20*time.Second, t0.Add(5*time.Hour), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusLive)
	env.restarted(20*time.Second, t0.Add(5*time.Hour+time.Minute), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusEnded)
}

func TestReconcilerEndsPublishingRoomAfterKeyExpires(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	_, base := newFakeSRS(t)
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	env.svc.now = func() time.Time { return t0 }
	st := startTestLive(t, env.svc, "owner-no-show")

	env.restarted(20*time.Second, t0.Add(30*time.Minute), base).Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusPublishing)

	// keyTTL is 1h in tests; without SRS too, as nobody can publish any more.
	env.restarted(20*time.Second, t0.Add(61*time.Minute), "").Reconcile(ctx)
	requireRoomStatus(t, env, st.ID, model.StatusEnded)
}

func TestSRSActivePublishersReadsAllPages(t *testing.T) {
	ctx := context.Background()
	srs, base := newFakeSRS(t)
	for i := range 25 {
		srs.setPublisher("live-"+strconv.Itoa(i), "cid-"+strconv.Itoa(i))
	}
	srs.idle = []string{"live-idle"}
	api := newSRSAPI(base)
	api.pageSize = 10

	publishers, err := api.activePublishers(ctx)
	require.NoError(t, err)
	require.Len(t, publishers, 25)
	require.Equal(t, "cid-24", publishers["live-24"])
	require.NotContains(t, publishers, "live-idle")
	require.Equal(t, 3, srs.listCalls)

	// A server that ignores the offset must not look like a short list.
	srs.ignoreStart = true
	_, err = api.activePublishers(ctx)
	require.Error(t, err)
}

func TestRunReconcilerStopsOnCancel(t *testing.T) {
	env := newLifecycleTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		env.svc.RunReconciler(ctx, time.Millisecond)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop")
	}
}
