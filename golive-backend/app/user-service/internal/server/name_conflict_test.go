package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func requireReason(t *testing.T, body []byte, status, code int, reason string) {
	t.Helper()
	require.Equal(t, status, code, string(body))
	var out struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	require.Equal(t, reason, out.Reason)
}

// Channel links resolve a name to the user with that username, so nobody may
// take another user's username as display name, in any case.
func TestDisplayNameCannotBeAnotherUsersUsername(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	creator, err := auth.Register(ctx, "kabun", "secret123", "Kabun Live")
	require.NoError(t, err)
	impostor, err := auth.Register(ctx, "impostor", "secret123", "Impostor")
	require.NoError(t, err)

	for _, name := range []string{"kabun", "KABUN", " Kabun "} {
		rec := serveJSON(router, http.MethodPatch, "/users/me/profile", impostor.Token, `{"displayName":"`+name+`"}`)
		requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "display_name_taken")
	}
	persisted, err := users.FindByID(ctx, impostor.User.ID)
	require.NoError(t, err)
	require.Equal(t, "Impostor", persisted.DisplayName)

	// A user's own username is fine, in any case.
	rec := serveJSON(router, http.MethodPatch, "/users/me/profile", creator.Token, `{"displayName":"KABUN"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Sign-up refuses it too, without using up the invite.
	_, err = auth.Register(ctx, "third", "secret123", "Kabun")
	require.ErrorIs(t, err, service.ErrDisplayNameTaken)
	invite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	_, err = auth.RegisterWithInvite(ctx, "fourth", "secret123", "kabun", "fourth@example.com", invite.Code)
	require.ErrorIs(t, err, service.ErrDisplayNameTaken)
	_, err = auth.RegisterWithInvite(ctx, "fourth", "secret123", "Fourth", "fourth@example.com", invite.Code)
	require.NoError(t, err)
}

// The reverse clash is refused as well: a new username may not be another
// user's display name.
func TestUsernameCannotBeAnotherUsersDisplayName(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	_, err := auth.Register(ctx, "solo_streams", "secret123", "Solo")
	require.NoError(t, err)
	other, err := auth.Register(ctx, "other", "secret123", "Other")
	require.NoError(t, err)

	rec := serveJSON(router, http.MethodPatch, "/users/me/profile", other.Token, `{"username":"SOLO","displayName":"Other"}`)
	requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "username_taken")
	_, err = auth.Register(ctx, "solo", "secret123", "Someone")
	require.ErrorIs(t, err, service.ErrUsernameIsDisplayName)
}

// Clashes stored before these checks stay as they are: their owners can
// still sign in and save their profile (the settings page always sends both
// names); only a newly chosen clashing name is refused.
func TestExistingNameClashesDoNotLockUsersOut(t *testing.T) {
	router, _, auth, db := newCoinsTestRouterWithDB(t)
	ctx := context.Background()
	creator, err := auth.Register(ctx, "kabun", "secret123", "Kabun Live")
	require.NoError(t, err)
	impostor, err := auth.Register(ctx, "impostor", "secret123", "Impostor")
	require.NoError(t, err)
	require.NoError(t, db.Exec(`UPDATE users SET display_name = ? WHERE id = ?`, "kabun", impostor.User.ID).Error)

	rec := serveJSON(router, http.MethodPatch, "/users/me/profile", impostor.Token, `{"username":"impostor2","displayName":"kabun"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = serveJSON(router, http.MethodPatch, "/users/me/profile", creator.Token, `{"username":"kabun","displayName":"Kabun Live"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, username := range []string{"kabun", "impostor2"} {
		_, err := auth.Login(ctx, username, "secret123")
		require.NoError(t, err, username)
	}

	rec = serveJSON(router, http.MethodPatch, "/users/me/profile", impostor.Token, `{"displayName":"KABUN"}`)
	requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "display_name_taken")
}

func TestAdminCannotGiveAnotherUsersUsernameAsDisplayName(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
	_, err = auth.Register(ctx, "kabun", "secret123", "Solo")
	require.NoError(t, err)
	target, err := auth.Register(ctx, "target", "secret123", "Target")
	require.NoError(t, err)

	rec := serveJSON(router, http.MethodPatch, "/admin/users/"+target.User.ID+"/profile", admin.Token, `{"username":"target","displayName":"Kabun"}`)
	requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "display_name_taken")
	rec = serveJSON(router, http.MethodPatch, "/admin/users/"+target.User.ID+"/profile", admin.Token, `{"username":"solo","displayName":"Target"}`)
	requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "username_taken")
	rec = serveJSON(router, http.MethodPost, "/admin/admins", admin.Token, `{"username":"second_admin","password":"secret123","displayName":"KABUN"}`)
	requireReason(t, rec.Body.Bytes(), http.StatusConflict, rec.Code, "display_name_taken")

	persisted, err := users.FindByID(ctx, target.User.ID)
	require.NoError(t, err)
	require.Equal(t, "target", persisted.Username)
	require.Equal(t, "Target", persisted.DisplayName)
}

// Google sign-up takes the display name from the Google profile, which the
// user cannot edit in that dialog: a clashing profile name falls back to the
// chosen username instead of failing. A clashing name the user sent is
// refused.
func TestGoogleRegisterAvoidsAnotherUsersUsername(t *testing.T) {
	router, users, auth := newGoogleTestRouter(t, fakeGoogleVerifier{profiles: map[string]*service.GoogleProfile{
		"profile-name-clash": {Subject: "google-sub-1", Email: "fan@example.com", EmailVerified: true, Name: "Kabun"},
		"chosen-name-clash":  {Subject: "google-sub-2", Email: "fan2@example.com", EmailVerified: true, Name: "Fan Two"},
	}})
	ctx := context.Background()
	_, err := auth.Register(ctx, "kabun", "secret123", "Kabun Live")
	require.NoError(t, err)
	register := func(credential, username, displayName string) (int, []byte) {
		invite, err := users.CreateInviteCode(ctx, "admin")
		require.NoError(t, err)
		rec := serveJSON(router, http.MethodPost, "/auth/google/register", "",
			`{"credential":"`+credential+`","username":"`+username+`","displayName":"`+displayName+`","inviteCode":"`+invite.Code+`"}`)
		return rec.Code, rec.Body.Bytes()
	}

	code, body := register("profile-name-clash", "kabunfan", "")
	require.Equal(t, http.StatusCreated, code, string(body))
	var login service.LoginResp
	require.NoError(t, json.Unmarshal(body, &login))
	require.Equal(t, "kabunfan", login.User.DisplayName)

	code, body = register("chosen-name-clash", "fantwo", "kabun")
	requireReason(t, body, http.StatusConflict, code, "display_name_taken")
}
