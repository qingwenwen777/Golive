package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

type fakeGoogleVerifier struct {
	profiles map[string]*service.GoogleProfile
}

func (v fakeGoogleVerifier) VerifyGoogleCredential(_ context.Context, credential, _ string) (*service.GoogleProfile, error) {
	return v.profiles[credential], nil
}

func newGoogleTestRouter(t *testing.T, verifier service.GoogleCredentialVerifier) (*gin.Engine, *repo.UserRepo, *service.AuthService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	users := repo.NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	auth := service.NewAuthService(users, repo.NewTokenRepo(rdb), service.Options{
		JWTSecret:      "test-secret",
		AccessTTL:      time.Hour,
		RefreshTTL:     24 * time.Hour,
		GoogleClientID: "test-client",
		GoogleVerifier: verifier,
	})
	return NewRouter(Deps{Auth: auth, Users: users}), users, auth
}

func TestGoogleRegisterAndLoginWithInvite(t *testing.T) {
	router, users, _ := newGoogleTestRouter(t, fakeGoogleVerifier{profiles: map[string]*service.GoogleProfile{
		"google-credential": {
			Subject:       "google-sub-1",
			Email:         "new@example.com",
			EmailVerified: true,
			Name:          "New Creator",
			Picture:       "https://example.com/avatar.png",
		},
	}})
	ctx := context.Background()
	invite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/google/register", bytes.NewBufferString(`{
		"credential":"google-credential",
		"username":"newcreator",
		"displayName":"New Creator",
		"inviteCode":"`+invite.Code+`"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var login service.LoginResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &login))
	require.Equal(t, "newcreator", login.User.Username)
	require.True(t, login.User.GoogleLinked)

	loginReq := httptest.NewRequest(http.MethodPost, "/auth/google/login", bytes.NewBufferString(`{"credential":"google-credential"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	require.Equal(t, http.StatusOK, loginRec.Code)
}

func TestGoogleRegisterCanLinkExistingEmailByPassword(t *testing.T) {
	router, users, auth := newGoogleTestRouter(t, fakeGoogleVerifier{profiles: map[string]*service.GoogleProfile{
		"existing-google": {
			Subject:       "google-sub-existing",
			Email:         "manual@example.com",
			EmailVerified: true,
			Name:          "Manual User",
		},
	}})
	ctx := context.Background()

	manualInvite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	manual, err := auth.RegisterWithInvite(ctx, "manual", "secret123", "Manual User", "manual@example.com", manualInvite.Code)
	require.NoError(t, err)

	unusedInvite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/auth/google/register", bytes.NewBufferString(`{
		"credential":"existing-google",
		"username":"another",
		"displayName":"Another",
		"inviteCode":"`+unusedInvite.Code+`"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var conflict struct {
		Reason string `json:"reason"`
		Email  string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &conflict))
	require.Equal(t, "google_email_exists", conflict.Reason)
	require.Equal(t, "manual@example.com", conflict.Email)

	linkReq := httptest.NewRequest(http.MethodPost, "/auth/google/link-existing", bytes.NewBufferString(`{
		"credential":"existing-google",
		"password":"secret123"
	}`))
	linkReq.Header.Set("Content-Type", "application/json")
	linkRec := httptest.NewRecorder()
	router.ServeHTTP(linkRec, linkReq)

	require.Equal(t, http.StatusOK, linkRec.Code)
	var linked service.LoginResp
	require.NoError(t, json.Unmarshal(linkRec.Body.Bytes(), &linked))
	require.Equal(t, manual.User.ID, linked.User.ID)
	require.True(t, linked.User.GoogleLinked)

	items, err := users.ListInviteCodes(ctx)
	require.NoError(t, err)
	usedCount := 0
	for _, item := range items {
		if item.Used {
			usedCount++
		}
	}
	require.Equal(t, 1, usedCount)
}
