package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/server"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

const internalToken = "internal-test-token"

type endLiveFixture struct {
	router http.Handler
	rooms  *repo.RoomRepo
	live   *repo.LiveRepo
	kicked func() []string
}

// newEndLiveFixture serves room-service with a fake SRS API that records the
// publishers it is asked to disconnect.
func newEndLiveFixture(t *testing.T, token string) endLiveFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	live := repo.NewLiveRepo(rdb)

	var mu sync.Mutex
	var kicked []string
	srs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/api/v1/clients/")
		if r.Method != http.MethodDelete || !ok {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		kicked = append(kicked, id)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	t.Cleanup(srs.Close)

	liveSvc := service.NewLiveService(rooms, live, "test-secret", time.Hour, "http://srs/live")
	liveSvc.SetSRSAPIBase(srs.URL)
	router := server.NewRouter(server.Deps{JWTSecret: jwtSecret, Live: liveSvc, InternalToken: token})
	return endLiveFixture{router: router, rooms: rooms, live: live, kicked: func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), kicked...)
	}}
}

// seedRoom stores a room with a stream key; clientID, when set, is the SRS
// client publishing to it.
func (fx endLiveFixture) seedRoom(t *testing.T, id, ownerID, status, clientID string) {
	t.Helper()
	ctx := context.Background()
	room := &model.Room{ID: id, Title: id, OwnerID: ownerID, ChannelID: "ch-" + ownerID, Status: status, StreamKey: "lk_" + id, StartedAt: time.Now().Add(-time.Hour)}
	if status == model.StatusEnded {
		endedAt := time.Now().Add(-30 * time.Minute)
		room.EndedAt = &endedAt
	}
	require.NoError(t, fx.rooms.Upsert(ctx, room))
	require.NoError(t, fx.live.Save(ctx, room.StreamKey, id, time.Hour))
	if clientID != "" {
		require.NoError(t, fx.live.SavePublishSession(ctx, room.StreamKey, clientID, time.Hour))
	}
}

func (fx endLiveFixture) requireStatus(t *testing.T, roomID, status string) {
	t.Helper()
	room, err := fx.rooms.GetByID(context.Background(), roomID)
	require.NoError(t, err)
	require.Equal(t, status, room.Status, roomID)
}

func postInternal(router http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if token != "" {
		req.Header.Set(internalauth.Header, token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// user-service calls POST /internal/users/:id/end-live when it bans a user:
// every active room of the user ends and its publisher is kicked from SRS.
func TestInternalEndUserLiveEndsTheUsersRooms(t *testing.T) {
	fx := newEndLiveFixture(t, internalToken)
	fx.seedRoom(t, "room-live", "banned-owner", model.StatusLive, "client-live")
	fx.seedRoom(t, "room-publishing", "banned-owner", model.StatusPublishing, "")
	fx.seedRoom(t, "room-old", "banned-owner", model.StatusEnded, "")
	fx.seedRoom(t, "room-other", "other-owner", model.StatusLive, "client-other")
	path := "/internal/users/banned-owner/end-live"

	rec := postInternal(fx.router, path, internalToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"userId":"banned-owner","endedRooms":2}`, rec.Body.String())
	fx.requireStatus(t, "room-live", model.StatusEnded)
	fx.requireStatus(t, "room-publishing", model.StatusEnded)
	fx.requireStatus(t, "room-old", model.StatusEnded)
	fx.requireStatus(t, "room-other", model.StatusLive)
	require.Equal(t, []string{"client-live"}, fx.kicked())
	for _, key := range []string{"lk_room-live", "lk_room-publishing"} {
		_, err := fx.live.Resolve(context.Background(), key)
		require.ErrorIs(t, err, repo.ErrStreamKeyNotFound, key)
	}

	// Repeating it (a retried ban, the report and admin paths both calling)
	// changes nothing.
	rec = postInternal(fx.router, path, internalToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"userId":"banned-owner","endedRooms":0}`, rec.Body.String())
	require.Equal(t, []string{"client-live"}, fx.kicked())
	fx.requireStatus(t, "room-other", model.StatusLive)
}

func TestInternalEndUserLiveRequiresInternalToken(t *testing.T) {
	fx := newEndLiveFixture(t, internalToken)
	fx.seedRoom(t, "room-live", "some-owner", model.StatusLive, "client-live")
	path := "/internal/users/some-owner/end-live"

	for _, token := range []string{"", "wrong"} {
		rec := postInternal(fx.router, path, token)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "token %q", token)
	}
	// The owner's own access token is not an internal credential.
	rec := requestJSON(fx.router, http.MethodPost, path, signedToken(t, "some-owner"), "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	fx.requireStatus(t, "room-live", model.StatusLive)
	require.Empty(t, fx.kicked())

	// Without a configured token the internal API is closed.
	closed := newEndLiveFixture(t, "")
	closed.seedRoom(t, "room-live", "some-owner", model.StatusLive, "client-live")
	for _, token := range []string{"", internalToken} {
		rec := postInternal(closed.router, path, token)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "token %q", token)
	}
	closed.requireStatus(t, "room-live", model.StatusLive)
}
