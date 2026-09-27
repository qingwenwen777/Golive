package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A banned admin keeps the admin role (login stays open to banned users so
// they can appeal) but must lose the economy console, including settling and
// cancelling bets. user-service records a ban in users.banned and
// user_moderation_states; either one counts, as in room-service.
func TestBannedAdminLosesEconomyConsole(t *testing.T) {
	fx := newGiftHTTPFixture(t, 0)
	require.NoError(t, fx.db.Exec(`ALTER TABLE users ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT 'user'`).Error)
	require.NoError(t, fx.db.Exec(`CREATE TABLE user_moderation_states (
		user_id VARCHAR(36) PRIMARY KEY,
		banned BOOLEAN NOT NULL DEFAULT false
	)`).Error)
	require.NoError(t, fx.db.Exec(
		"INSERT INTO users (id, username, coin_balance, role) VALUES (?, ?, 0, 'admin'), (?, ?, 0, 'admin'), (?, ?, 0, 'admin')",
		"admin-1", "admin1", "admin-2", "admin2", "admin-3", "admin3",
	).Error)
	router := newInternalGiftRouter(t, fx.db, testInternalToken)

	for _, admin := range []string{"admin-1", "admin-2", "admin-3"} {
		rec := getJSON(router, "/admin/economy/summary", admin)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	require.NoError(t, fx.db.Exec(`UPDATE users SET banned = ? WHERE id = ?`, true, "admin-1").Error)
	require.NoError(t, fx.db.Exec(`INSERT INTO user_moderation_states (user_id, banned) VALUES (?, ?)`, "admin-2", true).Error)
	for _, admin := range []string{"admin-1", "admin-2"} {
		for _, req := range []struct{ method, path, body string }{
			{http.MethodGet, "/admin/economy/summary", ""},
			{http.MethodGet, "/admin/economy/orders", ""},
			{http.MethodPost, "/admin/economy/bets/round-1/settle", `{"option":"a"}`},
			{http.MethodPost, "/admin/economy/bets/round-1/cancel", ""},
		} {
			rec := serveAdmin(router, req.method, req.path, admin, req.body)
			require.Equal(t, http.StatusForbidden, rec.Code, "%s %s %s", admin, req.method, req.path)
			require.Contains(t, rec.Body.String(), "user_banned", "%s %s %s", admin, req.method, req.path)
		}
	}

	rec := getJSON(router, "/admin/economy/summary", "admin-3")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = getJSON(router, "/admin/economy/summary", "u-demo")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NotContains(t, rec.Body.String(), "user_banned")
}

func serveAdmin(router http.Handler, method, path, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+signGiftToken(userID))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
