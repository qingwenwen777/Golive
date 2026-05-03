package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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
