package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func serveJSON(router *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAdminBanRevokesRefreshTokensAndBlocksWrites(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
	target, err := auth.Register(ctx, "target", "secret123", "Target")
	require.NoError(t, err)
	secondSession, err := auth.Login(ctx, "target", "secret123")
	require.NoError(t, err)

	// A checkout started before the ban must still be creditable after it.
	topupRec := serveJSON(router, http.MethodPost, "/users/me/coins/topup", target.Token, `{"amount":10}`)
	require.Equal(t, http.StatusOK, topupRec.Code)
	var checkout struct {
		SessionID string `json:"sessionId"`
	}
	require.NoError(t, json.Unmarshal(topupRec.Body.Bytes(), &checkout))

	banRec := serveJSON(router, http.MethodPatch, "/admin/users/"+target.User.ID+"/ban", admin.Token, `{"banned":true,"reason":"spam"}`)
	require.Equal(t, http.StatusOK, banRec.Code)

	// Every refresh token issued before the ban is revoked.
	for _, refresh := range []string{target.RefreshToken, secondSession.RefreshToken} {
		_, err := auth.Refresh(ctx, refresh)
		require.Error(t, err)
	}

	// Signing in again still works and reports the ban so the client can
	// route to the appeal page; the new session can read, but a banned user
	// cannot refresh and must sign in again once the access token expires.
	relogin, err := auth.Login(ctx, "target", "secret123")
	require.NoError(t, err)
	require.True(t, relogin.User.Banned)
	_, err = auth.Refresh(ctx, relogin.RefreshToken)
	require.ErrorIs(t, err, service.ErrUserBanned)
	refreshRec := serveJSON(router, http.MethodPost, "/auth/refresh", "", fmt.Sprintf(`{"refreshToken":%q}`, relogin.RefreshToken))
	require.Equal(t, http.StatusForbidden, refreshRec.Code)
	require.Contains(t, refreshRec.Body.String(), "user_banned")
	meRec := serveJSON(router, http.MethodGet, "/users/me", relogin.Token, "")
	require.Equal(t, http.StatusOK, meRec.Code)
	txRec := serveJSON(router, http.MethodGet, "/users/me/coins/transactions", relogin.Token, "")
	require.Equal(t, http.StatusOK, txRec.Code)

	blocked := []struct {
		method, path, body string
	}{
		{http.MethodPatch, "/users/me/profile", `{"displayName":"Renamed"}`},
		{http.MethodPost, "/users/me/coins/topup", `{"amount":10}`},
		{http.MethodPost, "/users/me/coins/withdrawals", `{"amount":10}`},
		{http.MethodPost, "/users/me/coins/daily-tasks/daily-login-lottery/claim", ""},
		{http.MethodPost, "/users/me/avatar", ""},
		{http.MethodPost, "/users/me/cover", ""},
		{http.MethodPost, "/creator/applications", `{"reason":"let me stream please"}`},
		{http.MethodPost, "/creator/platform-applications", `{"reason":"let me stream please"}`},
	}
	for _, tc := range blocked {
		rec := serveJSON(router, tc.method, tc.path, relogin.Token, tc.body)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s", tc.method, tc.path)
		require.Contains(t, rec.Body.String(), "user_banned", "%s %s", tc.method, tc.path)
	}

	confirmRec := serveJSON(router, http.MethodPost, "/users/me/coins/topup/confirm", relogin.Token, fmt.Sprintf(`{"sessionId":%q}`, checkout.SessionID))
	require.Equal(t, http.StatusOK, confirmRec.Code)

	persisted, err := users.FindByID(ctx, target.User.ID)
	require.NoError(t, err)
	require.True(t, persisted.Banned)
	require.Equal(t, "Target", persisted.DisplayName)
	require.Equal(t, int64(1210), persisted.CoinBalance)

	unbanRec := serveJSON(router, http.MethodPatch, "/admin/users/"+target.User.ID+"/ban", admin.Token, `{"banned":false}`)
	require.Equal(t, http.StatusOK, unbanRec.Code)
	profileRec := serveJSON(router, http.MethodPatch, "/users/me/profile", relogin.Token, `{"displayName":"Renamed"}`)
	require.Equal(t, http.StatusOK, profileRec.Code)
	_, err = auth.Refresh(ctx, relogin.RefreshToken)
	require.NoError(t, err)
}

func TestBannedAdminLosesAdminAccess(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
	_, err = users.AdminSetUserBan(ctx, admin.User.ID, true, "compromised", "")
	require.NoError(t, err)

	rec := serveJSON(router, http.MethodGet, "/admin/users", admin.Token, "")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "user_banned")
}
