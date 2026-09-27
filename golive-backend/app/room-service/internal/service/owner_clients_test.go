package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

const fakeOwnerToken = "owner-test-token"

// fakeOwners stands in for user-, gift- and chat-service's /internal APIs.
// It writes the shared test DB the way the owning service would, so
// moderation tests can observe the effect through room-service's reads.
type fakeOwners struct {
	db  *gorm.DB
	srv *httptest.Server

	mu     sync.Mutex
	calls  []string
	status map[string]int // path prefix -> forced status
}

func newFakeOwners(t *testing.T, db *gorm.DB) *fakeOwners {
	t.Helper()
	f := &fakeOwners{db: db, status: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/users/{id}/restriction", f.restriction)
	mux.HandleFunc("POST /internal/super-chats/{id}/moderation", f.superChat)
	mux.HandleFunc("DELETE /internal/rooms/{room}/danmus/{id}", f.danmu)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalauth.Header) != fakeOwnerToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		forced := f.status[r.URL.Path]
		f.mu.Unlock()
		if forced != 0 {
			w.WriteHeader(forced)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOwners) services() OwnerServices {
	return NewOwnerServices(f.srv.URL, f.srv.URL, f.srv.URL, fakeOwnerToken)
}

func (f *fakeOwners) fail(path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[path] = status
}

func (f *fakeOwners) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeOwners) restriction(w http.ResponseWriter, r *http.Request) {
	var req UserRestrictionUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	now := time.Now()
	state := model.UserModerationState{UserID: r.PathValue("id"), UpdatedBy: req.OperatorID, UpdatedAt: now, CreatedAt: now}
	updates := map[string]any{"updated_by": req.OperatorID, "updated_at": now}
	switch req.Action {
	case RestrictionBan:
		state.Banned, state.BanReason = true, req.Reason
		updates["banned"], updates["ban_reason"] = true, req.Reason
	case RestrictionMute:
		state.MutedUntil, state.MuteReason = req.MutedUntil, req.Reason
		updates["muted_until"], updates["mute_reason"] = req.MutedUntil, req.Reason
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err := f.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&state).Error
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeOwners) superChat(w http.ResponseWriter, r *http.Request) {
	if err := f.db.Exec(`UPDATE super_chat_orders SET moderated_at = ? WHERE order_id = ? AND moderated_at IS NULL`, time.Now(), r.PathValue("id")).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Like gift-service: an already hidden order is fine, an unknown one is
	// its own 404.
	var orders int64
	if err := f.db.Raw(`SELECT COUNT(*) FROM super_chat_orders WHERE order_id = ?`, r.PathValue("id")).Scan(&orders).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if orders == 0 {
		writeOwnerNotFound(w, superChatNotFoundReason)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fakeOwners) danmu(w http.ResponseWriter, r *http.Request) {
	var hidden int64
	for i := 0; i < 2; i++ {
		res := f.db.Exec(fmt.Sprintf(`UPDATE danmus_%d SET deleted_at = ? WHERE room_id = ? AND id = ?`, i), time.Now(), r.PathValue("room"), r.PathValue("id"))
		if res.Error == nil {
			hidden += res.RowsAffected
		}
	}
	if hidden == 0 {
		writeOwnerNotFound(w, chatMessageNotFoundReason)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeOwnerNotFound answers like the owning service's handler does for an
// item that does not exist: 404 with an errcode body carrying reason.
func writeOwnerNotFound(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found", "reason": reason})
}

// capture records the last request an owner service received.
type capture struct {
	method, path, token string
	body                map[string]any
}

func captureServer(t *testing.T, status int, got *capture) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.token = r.Method, r.URL.Path, r.Header.Get(internalauth.Header)
		got.body = nil
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestUserServiceClientSetsRestriction(t *testing.T) {
	var got capture
	owners := NewOwnerServices(captureServer(t, http.StatusOK, &got), "", "", "tok")
	until := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	err := owners.Users.SetUserRestriction(context.Background(), "u/1", UserRestrictionUpdate{
		Action: RestrictionMute, Reason: "flood", MutedUntil: &until, OperatorID: "admin-1",
	})
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/internal/users/u/1/restriction", got.path)
	require.Equal(t, "tok", got.token)
	require.Equal(t, map[string]any{"action": "mute", "reason": "flood", "mutedUntil": "2030-01-01T00:00:00Z", "operatorId": "admin-1"}, got.body)

	// Errors, including 404 and auth failures, surface to the caller.
	for _, status := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusInternalServerError} {
		owners = NewOwnerServices(captureServer(t, status, &got), "", "", "tok")
		err = owners.Users.SetUserRestriction(context.Background(), "u1", UserRestrictionUpdate{Action: RestrictionBan})
		require.Error(t, err, status)
		require.True(t, internalauth.IsStatus(err, status))
	}
}

func TestGiftServiceClientModeratesSuperChat(t *testing.T) {
	var got capture
	owners := NewOwnerServices("", captureServer(t, http.StatusOK, &got), "", "tok")
	require.NoError(t, owners.SuperChats.ModerateSuperChat(context.Background(), "sc-1", "admin-1"))
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/internal/super-chats/sc-1/moderation", got.path)
	require.Equal(t, "tok", got.token)
	require.Equal(t, map[string]any{"operatorId": "admin-1"}, got.body)

	owners = NewOwnerServices("", notFoundServer(t, superChatNotFoundReason), "", "tok")
	require.NoError(t, owners.SuperChats.ModerateSuperChat(context.Background(), "sc-gone", "admin-1"), "nothing left to hide")
	// A 404 that is not gift-service saying so (no such route, another
	// service's not-found) leaves the super chat up: it is an error.
	for _, url := range []string{
		captureServer(t, http.StatusNotFound, &got),
		notFoundServer(t, chatMessageNotFoundReason),
		captureServer(t, http.StatusBadGateway, &got),
	} {
		owners = NewOwnerServices("", url, "", "tok")
		require.Error(t, owners.SuperChats.ModerateSuperChat(context.Background(), "sc-1", "admin-1"))
	}
}

func TestChatServiceClientHidesMessage(t *testing.T) {
	var got capture
	owners := NewOwnerServices("", "", captureServer(t, http.StatusOK, &got), "tok")
	require.NoError(t, owners.Chat.HideChatMessage(context.Background(), "room-1", "m-1"))
	require.Equal(t, http.MethodDelete, got.method)
	require.Equal(t, "/internal/rooms/room-1/danmus/m-1", got.path)
	require.Equal(t, "tok", got.token)

	owners = NewOwnerServices("", "", notFoundServer(t, chatMessageNotFoundReason), "tok")
	require.NoError(t, owners.Chat.HideChatMessage(context.Background(), "room-1", "gone"), "nothing left to hide")
	for _, url := range []string{
		captureServer(t, http.StatusNotFound, &got),
		notFoundServer(t, superChatNotFoundReason),
	} {
		owners = NewOwnerServices("", "", url, "tok")
		require.Error(t, owners.Chat.HideChatMessage(context.Background(), "room-1", "m-1"))
	}
	owners = NewOwnerServices("", "", captureServer(t, http.StatusUnauthorized, &got), "")
	require.Error(t, owners.Chat.HideChatMessage(context.Background(), "room-1", "m-1"))
}

// notFoundServer answers every request like an owning service whose item
// does not exist.
func notFoundServer(t *testing.T, reason string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOwnerNotFound(w, reason)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestOwnerClientsTimeOut(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	owners := NewOwnerServices(srv.URL, "", "", "tok")
	require.Error(t, owners.Users.SetUserRestriction(ctx, "u1", UserRestrictionUpdate{Action: RestrictionBan}))
}
