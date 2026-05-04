package jwtauth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

func TestKeySet_SignsWithActiveKIDAndVerifiesByKID(t *testing.T) {
	keys, err := jwtauth.NewKeySet("old-secret", "current", []jwtauth.KeyConfig{
		{KID: "current", Secret: "new-secret"},
		{KID: "previous", Secret: "old-secret"},
	})
	require.NoError(t, err)

	token, err := keys.SignAccess("user-1", time.Now(), time.Hour)
	require.NoError(t, err)

	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	require.NoError(t, err)
	require.Equal(t, "current", parsed.Header["kid"])

	uid, err := keys.VerifyAccess(token)
	require.NoError(t, err)
	require.Equal(t, "user-1", uid)
}

func TestKeySet_VerifiesLegacyTokenWithoutKID(t *testing.T) {
	keys, err := jwtauth.NewKeySet("legacy-secret", "current", []jwtauth.KeyConfig{
		{KID: "current", Secret: "new-secret"},
	})
	require.NoError(t, err)

	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "legacy-user",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	token, err := legacy.SignedString([]byte("legacy-secret"))
	require.NoError(t, err)

	uid, err := keys.VerifyAccess(token)
	require.NoError(t, err)
	require.Equal(t, "legacy-user", uid)
}

func TestKeySet_RejectsUnknownKID(t *testing.T) {
	keys, err := jwtauth.NewKeySet("", "current", []jwtauth.KeyConfig{
		{KID: "current", Secret: "new-secret"},
	})
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "missing"
	token, err := tok.SignedString([]byte("new-secret"))
	require.NoError(t, err)

	_, err = keys.VerifyAccess(token)
	require.ErrorIs(t, err, jwtauth.ErrInvalidToken)
}
