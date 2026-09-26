package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// lifecycleTestEnv is newLiveServiceTestDeps plus the database and Redis
// server, so tests can inject races and move Redis time.
type lifecycleTestEnv struct {
	svc   *LiveService
	rooms *repo.RoomRepo
	live  *repo.LiveRepo
	rdb   *redis.Client
	db    *gorm.DB
	mr    *miniredis.Miniredis
}

func newLifecycleTestEnv(t *testing.T) *lifecycleTestEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps the in-memory database shared with goroutines.
	sqlDB.SetMaxOpenConns(1)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	live := repo.NewLiveRepo(rdb)
	svc := NewLiveService(rooms, live, "test-secret", time.Hour, "http://srs/live")
	svc.unpublishGrace = 0
	return &lifecycleTestEnv{svc: svc, rooms: rooms, live: live, rdb: rdb, db: db, mr: mr}
}

// beforeRoomUpdate runs fn once, just before the first rooms UPDATE that sets
// status to wantStatus, emulating a concurrent request winning that race.
func (e *lifecycleTestEnv) beforeRoomUpdate(t *testing.T, wantStatus string, fn func()) {
	t.Helper()
	var fired atomic.Bool
	require.NoError(t, e.db.Callback().Update().Before("gorm:begin_transaction").Register("test:race", func(tx *gorm.DB) {
		dest, ok := tx.Statement.Dest.(map[string]any)
		if !ok || dest["status"] != wantStatus || !fired.CompareAndSwap(false, true) {
			return
		}
		fn()
	}))
}

// collectRoomEvents returns the payloads published on roomID's channel until
// the channel stays quiet for a moment.
func collectRoomEvents(t *testing.T, sub *redis.PubSub) []string {
	t.Helper()
	var out []string
	for {
		select {
		case msg := <-sub.Channel():
			out = append(out, msg.Payload)
		case <-time.After(200 * time.Millisecond):
			return out
		}
	}
}

func countEnded(events []string) int {
	n := 0
	for _, e := range events {
		if strings.Contains(e, `"type":"live_status"`) && strings.Contains(e, `"status":"ended"`) {
			n++
		}
	}
	return n
}

func TestOnPublishDoesNotReviveRoomStoppedMidPublish(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	st := startTestLive(t, env.svc, "owner-race-publish")
	env.beforeRoomUpdate(t, model.StatusLive, func() {
		require.NoError(t, env.svc.StopLive(ctx, "owner-race-publish"))
	})

	require.Error(t, env.svc.OnPublish(ctx, srsClientReq(st.StreamKey, "", "client-race")))

	room, err := env.rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
	require.NotNil(t, room.EndedAt)
	_, err = env.live.PublishSession(ctx, publishSecret(st.StreamKey))
	require.ErrorIs(t, err, repo.ErrStreamKeyNotFound)
}

func TestConcurrentStopsRunFollowUpOnce(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	st := startTestLive(t, env.svc, "owner-race-stop")
	publishTestLive(t, env.svc, st.StreamKey)

	sub := env.rdb.Subscribe(ctx, "room:"+st.ID)
	defer func() { require.NoError(t, sub.Close()) }()
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	firstEnd := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)
	env.svc.now = func() time.Time { return firstEnd }
	env.beforeRoomUpdate(t, model.StatusEnded, func() {
		require.NoError(t, env.svc.ForceStopRoom(ctx, st.ID))
	})
	env.svc.now = func() time.Time { return firstEnd.Add(time.Minute) }
	require.NoError(t, env.svc.StopLive(ctx, "owner-race-stop"))

	require.Equal(t, 1, countEnded(collectRoomEvents(t, sub)))
	room, err := env.rooms.GetByID(ctx, st.ID)
	require.NoError(t, err)
	require.Equal(t, model.StatusEnded, room.Status)
}

func TestConcurrentGoLiveKeepsOneActiveRoom(t *testing.T) {
	ctx := context.Background()
	env := newLifecycleTestEnv(t)
	var nestedErr error
	var fired atomic.Bool
	require.NoError(t, env.db.Callback().Create().Before("gorm:begin_transaction").Register("test:race", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*model.Room); !ok || !fired.CompareAndSwap(false, true) {
			return
		}
		_, nestedErr = env.svc.GoLive(ctx, "owner-race-golive", GoLiveReq{Title: "Second", Category: "Chat"})
	}))

	_, err := env.svc.GoLive(ctx, "owner-race-golive", GoLiveReq{Title: "First", Category: "Chat"})
	require.NoError(t, err)

	var appErr *errcode.AppError
	require.ErrorAs(t, nestedErr, &appErr)
	require.Equal(t, 409, appErr.HTTPStatus)
	active, err := env.rooms.ActiveRoomsByOwner(ctx, "owner-race-golive")
	require.NoError(t, err)
	require.Len(t, active, 1)
}
