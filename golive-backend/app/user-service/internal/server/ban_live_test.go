package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

// fakeRoomService stands in for room-service's
// POST /internal/users/:id/end-live and records the calls it gets.
type fakeRoomService struct {
	mu     sync.Mutex
	calls  []string
	status int
	hang   bool
}

func newFakeRoomService(t *testing.T) (*fakeRoomService, string) {
	t.Helper()
	f := &fakeRoomService{status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalauth.Header) != testInternalToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		status, hang := f.status, f.hang
		f.mu.Unlock()
		if hang {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"endedRooms":1}`))
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeRoomService) set(status int, hang bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.hang = status, hang
}

func (f *fakeRoomService) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type banLiveFixture struct {
	router *gin.Engine
	users  *repo.UserRepo
	auth   *service.AuthService
	rooms  *fakeRoomService
	admin  string
	target *service.LoginResp
}

func newBanLiveFixture(t *testing.T) banLiveFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	users := repo.NewUserRepo(db).WithRedis(rdb)
	require.NoError(t, users.AutoMigrate())
	auth := service.NewAuthService(users, repo.NewTokenRepo(rdb), service.Options{
		JWTSecret:  "test-secret",
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})
	rooms, roomsURL := newFakeRoomService(t)
	router := NewRouter(Deps{
		Auth:          auth,
		Users:         users,
		LiveRooms:     service.NewRoomServiceClient(roomsURL, testInternalToken),
		InternalToken: testInternalToken,
	})

	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
	target, err := auth.Register(ctx, "streamer", "secret123", "Streamer")
	require.NoError(t, err)
	return banLiveFixture{router: router, users: users, auth: auth, rooms: rooms, admin: admin.Token, target: target}
}

func (fx banLiveFixture) adminBan(t *testing.T, banned bool) (int, map[string]any) {
	t.Helper()
	body := `{"banned":false}`
	if banned {
		body = `{"banned":true,"reason":"streaming abuse"}`
	}
	rec := serveJSON(fx.router, http.MethodPatch, "/admin/users/"+fx.target.User.ID+"/ban", fx.admin, body)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec.Code, out
}

func (fx banLiveFixture) internalBan(t *testing.T) (int, map[string]any) {
	t.Helper()
	rec := serveInternal(fx.router, http.MethodPost, "/internal/users/"+fx.target.User.ID+"/restriction", testInternalToken, `{"action":"ban","reason":"report","operatorId":"admin-1"}`)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec.Code, out
}

// A ban ends a stream that is already running: user-service asks room-service,
// which owns rooms, to end the banned user's live rooms.
func TestBanEndsUsersLiveThroughRoomService(t *testing.T) {
	fx := newBanLiveFixture(t)
	endLive := "POST /internal/users/" + fx.target.User.ID + "/end-live"

	code, body := fx.adminBan(t, true)
	require.Equal(t, http.StatusOK, code, body)
	require.NotContains(t, body, "warning")
	require.Equal(t, []string{endLive}, fx.rooms.called())

	code, _ = fx.adminBan(t, false)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, fx.rooms.called(), 1, "an unban leaves rooms alone")

	// Report bans reach user-service through the internal API: same path.
	code, body = fx.internalBan(t)
	require.Equal(t, http.StatusOK, code, body)
	require.NotContains(t, body, "warning")
	require.Equal(t, []string{endLive, endLive}, fx.rooms.called())
}

// If room-service cannot end the live, the ban still stands and the response
// says so; room-service's reconciler ends the rooms of banned owners later.
func TestBanStandsWhenRoomServiceFails(t *testing.T) {
	fx := newBanLiveFixture(t)
	fx.rooms.set(http.StatusServiceUnavailable, false)

	code, body := fx.adminBan(t, true)
	require.Equal(t, http.StatusOK, code, body)
	requireLiveEndWarning(t, body)
	requireBanned(t, fx)

	code, body = fx.internalBan(t)
	require.Equal(t, http.StatusOK, code, body)
	requireLiveEndWarning(t, body)
	require.Len(t, fx.rooms.called(), 2)
}

// A room-service that does not answer delays the ban only briefly.
func TestBanDoesNotWaitLongForRoomService(t *testing.T) {
	fx := newBanLiveFixture(t)
	fx.rooms.set(http.StatusOK, true)

	started := time.Now()
	code, body := fx.adminBan(t, true)
	require.Less(t, time.Since(started), 5*time.Second)
	require.Equal(t, http.StatusOK, code, body)
	requireLiveEndWarning(t, body)
	requireBanned(t, fx)
}

func requireLiveEndWarning(t *testing.T, body map[string]any) {
	t.Helper()
	warning, ok := body["warning"].(map[string]any)
	require.True(t, ok, "missing warning in %v", body)
	require.Equal(t, "live_end_failed", warning["reason"])
	require.NotEmpty(t, warning["message"])
}

func requireBanned(t *testing.T, fx banLiveFixture) {
	t.Helper()
	ctx := context.Background()
	persisted, err := fx.users.FindByID(ctx, fx.target.User.ID)
	require.NoError(t, err)
	require.True(t, persisted.Banned)
	_, err = fx.auth.Refresh(ctx, fx.target.RefreshToken)
	require.Error(t, err, "sessions are revoked even when the live could not be ended")
}
