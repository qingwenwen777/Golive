package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
)

func sign(t *testing.T, secret, sub string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func TestVerifier_EmptyToken_Error(t *testing.T) {
	v := auth.NewHMACVerifier("k")
	_, err := v.Verify("")
	require.ErrorIs(t, err, auth.ErrMissingToken)
}

func TestVerifier_GoodToken_Authenticated(t *testing.T) {
	v := auth.NewHMACVerifier("k")
	id, err := v.Verify(sign(t, "k", "u-1"))
	require.NoError(t, err)
	require.False(t, id.Anonymous)
	require.Equal(t, "u-1", id.UserID)
	require.True(t, id.CanChat())
}

func TestVerifier_BadToken_Error(t *testing.T) {
	v := auth.NewHMACVerifier("k")
	_, err := v.Verify("not-a-jwt")
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestVerifier_WrongSecret_Error(t *testing.T) {
	v := auth.NewHMACVerifier("right-key")
	_, err := v.Verify(sign(t, "wrong-key", "u-1"))
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}
