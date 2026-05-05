package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func TestRegisterWithInviteConsumesCodeAndResetByEmail(t *testing.T) {
	_, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	invite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)

	login, err := auth.RegisterWithInvite(ctx, "kabun", "secret123", "Kabun Live", "kabun@example.com", invite.Code)
	require.NoError(t, err)
	require.Equal(t, "kabun", login.User.Username)

	items, err := users.ListInviteCodes(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.True(t, items[0].Used)
	require.Equal(t, login.User.ID, items[0].UsedBy)
	require.Equal(t, "kabun", items[0].UsedUsername)
	require.Equal(t, "kabun@example.com", items[0].UsedEmail)

	_, err = auth.RegisterWithInvite(ctx, "nextuser", "secret123", "Next User", "next@example.com", invite.Code)
	require.ErrorIs(t, err, service.ErrInviteUsed)

	invite2, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	_, err = auth.RegisterWithInvite(ctx, "otheruser", "secret123", "Other User", "kabun@example.com", invite2.Code)
	require.ErrorIs(t, err, service.ErrEmailTaken)

	require.NoError(t, auth.ResetPasswordByEmail(ctx, "kabun@example.com", "newsecret1"))
	_, err = auth.Login(ctx, "kabun", "newsecret1")
	require.NoError(t, err)
}

func TestDeleteUnusedInviteCode(t *testing.T) {
	_, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	unused, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)

	deleted, err := users.DeleteUnusedInviteCode(ctx, unused.ID)
	require.NoError(t, err)
	require.Equal(t, unused.Code, deleted.Code)

	items, err := users.ListInviteCodes(ctx)
	require.NoError(t, err)
	require.Empty(t, items)

	used, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	_, err = auth.RegisterWithInvite(ctx, "inviteuser", "secret123", "Invite User", "invite@example.com", used.Code)
	require.NoError(t, err)

	_, err = users.DeleteUnusedInviteCode(ctx, used.ID)
	require.ErrorIs(t, err, repo.ErrInviteUsed)
}

func TestDeleteInviteCodeAdminRoute(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))

	unused, err := users.CreateInviteCode(ctx, admin.User.ID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/admin/invite-codes/"+unused.ID, nil)
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	items, err := users.ListInviteCodes(ctx)
	require.NoError(t, err)
	require.Empty(t, items)

	used, err := users.CreateInviteCode(ctx, admin.User.ID)
	require.NoError(t, err)
	_, err = auth.RegisterWithInvite(ctx, "inviteuser", "secret123", "Invite User", "invite@example.com", used.Code)
	require.NoError(t, err)

	usedReq := httptest.NewRequest(http.MethodDelete, "/admin/invite-codes/"+used.ID, nil)
	usedReq.Header.Set("Authorization", "Bearer "+admin.Token)
	usedRec := httptest.NewRecorder()
	router.ServeHTTP(usedRec, usedReq)

	require.Equal(t, http.StatusConflict, usedRec.Code)
}
