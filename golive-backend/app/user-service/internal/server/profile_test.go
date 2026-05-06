package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

func TestPublicProfileReturnsPublicUserByID(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo Creator")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/users/profile/"+login.User.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, login.User.ID, got.ID)
	require.Equal(t, "Demo Creator", got.DisplayName)
}

func TestUpdateProfileChangesDisplayNameAndUsername(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo Creator")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/users/me/profile", bytes.NewBufferString(`{"username":"demo-live","displayName":"Demo Live"}`))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "demo-live", got.Username)
	require.Equal(t, "Demo Live", got.DisplayName)
	require.NotEmpty(t, got.UsernameChangeAvailableAt)

	persisted, err := users.FindByID(context.Background(), login.User.ID)
	require.NoError(t, err)
	require.Equal(t, "demo-live", persisted.Username)
	require.NotNil(t, persisted.UsernameUpdatedAt)
}

func TestUpdateProfileRejectsUsernameCooldown(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo Creator")
	require.NoError(t, err)

	first := httptest.NewRequest(http.MethodPatch, "/users/me/profile", bytes.NewBufferString(`{"username":"demo-live"}`))
	first.Header.Set("Authorization", "Bearer "+login.Token)
	first.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)
	require.Equal(t, http.StatusOK, firstRec.Code)

	second := httptest.NewRequest(http.MethodPatch, "/users/me/profile", bytes.NewBufferString(`{"username":"demo-next"}`))
	second.Header.Set("Authorization", "Bearer "+login.Token)
	second.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)

	require.Equal(t, http.StatusConflict, secondRec.Code)
	require.Contains(t, secondRec.Body.String(), "username_cooldown")
}

func TestChangePasswordRequiresCurrentPassword(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo Creator")
	require.NoError(t, err)

	bad := httptest.NewRequest(http.MethodPost, "/users/me/password", bytes.NewBufferString(`{"currentPassword":"wrong","newPassword":"new-secret1"}`))
	bad.Header.Set("Authorization", "Bearer "+login.Token)
	bad.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, bad)
	require.Equal(t, http.StatusUnauthorized, badRec.Code)

	good := httptest.NewRequest(http.MethodPost, "/users/me/password", bytes.NewBufferString(`{"currentPassword":"demo","newPassword":"new-secret1"}`))
	good.Header.Set("Authorization", "Bearer "+login.Token)
	good.Header.Set("Content-Type", "application/json")
	goodRec := httptest.NewRecorder()
	router.ServeHTTP(goodRec, good)
	require.Equal(t, http.StatusOK, goodRec.Code)

	_, err = auth.Login(context.Background(), "demo", "new-secret1")
	require.NoError(t, err)
}

func TestAdminCanUpdateUserEmailAndPassword(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
	target, err := auth.Register(ctx, "target", "oldpass", "Target User")
	require.NoError(t, err)

	emailReq := httptest.NewRequest(http.MethodPatch, "/admin/users/"+target.User.ID+"/email", bytes.NewBufferString(`{"email":"target-new@example.com"}`))
	emailReq.Header.Set("Authorization", "Bearer "+admin.Token)
	emailReq.Header.Set("Content-Type", "application/json")
	emailRec := httptest.NewRecorder()
	router.ServeHTTP(emailRec, emailReq)
	require.Equal(t, http.StatusOK, emailRec.Code)

	updated, err := users.FindByID(ctx, target.User.ID)
	require.NoError(t, err)
	require.Equal(t, "target-new@example.com", updated.Email)

	passwordReq := httptest.NewRequest(http.MethodPatch, "/admin/users/"+target.User.ID+"/password", bytes.NewBufferString(`{"password":"newpass123"}`))
	passwordReq.Header.Set("Authorization", "Bearer "+admin.Token)
	passwordReq.Header.Set("Content-Type", "application/json")
	passwordRec := httptest.NewRecorder()
	router.ServeHTTP(passwordRec, passwordReq)
	require.Equal(t, http.StatusOK, passwordRec.Code)

	_, err = auth.Login(ctx, "target", "newpass123")
	require.NoError(t, err)
}
