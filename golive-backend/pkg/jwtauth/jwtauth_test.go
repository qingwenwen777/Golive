package jwtauth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

func TestKeySet_SignsAndVerifiesRS256(t *testing.T) {
	privatePEM, publicPEM := rsaPEM(t)
	keys, err := jwtauth.NewKeySet("", "rsa-current", []jwtauth.KeyConfig{
		{
			KID:        "rsa-current",
			Alg:        "RS256",
			PrivateKey: string(privatePEM),
			PublicKey:  string(publicPEM),
		},
	})
	require.NoError(t, err)

	token, err := keys.SignAccess("user-rsa", time.Now(), time.Hour)
	require.NoError(t, err)

	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	require.NoError(t, err)
	require.Equal(t, "RS256", parsed.Header["alg"])
	require.Equal(t, "rsa-current", parsed.Header["kid"])

	uid, err := keys.VerifyAccess(token)
	require.NoError(t, err)
	require.Equal(t, "user-rsa", uid)
}

func TestKeySet_VerifiesRS256WithPublicKeyOnly(t *testing.T) {
	privatePEM, publicPEM := rsaPEM(t)
	signer, err := jwtauth.NewKeySet("", "rsa-current", []jwtauth.KeyConfig{
		{KID: "rsa-current", Alg: "RS256", PrivateKey: string(privatePEM)},
	})
	require.NoError(t, err)
	verifier, err := jwtauth.NewKeySet("", "rsa-current", []jwtauth.KeyConfig{
		{KID: "rsa-current", Alg: "RS256", PublicKey: string(publicPEM)},
	})
	require.NoError(t, err)

	token, err := signer.SignAccess("user-rsa", time.Now(), time.Hour)
	require.NoError(t, err)

	uid, err := verifier.VerifyAccess(token)
	require.NoError(t, err)
	require.Equal(t, "user-rsa", uid)

	_, err = verifier.SignAccess("user-rsa", time.Now(), time.Hour)
	require.Error(t, err)
}

func TestKeySet_RejectsAlgorithmConfusionForKID(t *testing.T) {
	_, publicPEM := rsaPEM(t)
	keys, err := jwtauth.NewKeySet("", "rsa-current", []jwtauth.KeyConfig{
		{KID: "rsa-current", Alg: "RS256", PublicKey: string(publicPEM)},
	})
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "rsa-current"
	token, err := tok.SignedString([]byte("not-the-rsa-key"))
	require.NoError(t, err)

	_, err = keys.VerifyAccess(token)
	require.ErrorIs(t, err, jwtauth.ErrInvalidToken)
}

func rsaPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicDER,
	})
	return privatePEM, publicPEM
}
