package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
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

func stringPtr(s string) *string { return &s }

type recordingMailer struct {
	mu    sync.Mutex
	codes map[string]string
}

func (m *recordingMailer) SendVerificationCode(_ context.Context, toEmail, code string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[toEmail] = code
	return nil
}

func (m *recordingMailer) code(email string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	code, ok := m.codes[email]
	return code, ok
}

func newEmailResetTestRouter(t *testing.T, verifier service.GoogleCredentialVerifier) (*gin.Engine, *repo.UserRepo, *service.AuthService, *recordingMailer) {
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
	mailer := &recordingMailer{codes: map[string]string{}}
	emailCodes := service.NewEmailCodeService(rdb, mailer, 10*time.Minute, time.Minute)
	return NewRouter(Deps{Auth: auth, Users: users, EmailCodes: emailCodes}), users, auth, mailer
}

func requestResetCode(router *gin.Engine, username, email string) *responseWithReason {
	rec := serveJSON(router, http.MethodPost, "/auth/email-code", "",
		`{"purpose":"`+service.EmailPurposePasswordReset+`","username":"`+username+`","email":"`+email+`"}`)
	out := &responseWithReason{Code: rec.Code}
	_ = json.Unmarshal(rec.Body.Bytes(), out)
	return out
}

type responseWithReason struct {
	Code   int    `json:"-"`
	Reason string `json:"reason"`
}

// Accounts used to get an invented lower(username)+"@gmail.com" address, so
// the owner of e.g. admin@gmail.com could reset their password.
func TestPasswordResetRefusesPlaceholderAndUnverifiedEmails(t *testing.T) {
	router, users, auth, mailer := newEmailResetTestRouter(t, nil)
	ctx := context.Background()
	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))

	createRec := serveJSON(router, http.MethodPost, "/admin/admins", admin.Token, `{"username":"Boss","password":"secret123"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)
	boss, err := users.FindByUsername(ctx, "Boss")
	require.NoError(t, err)
	require.Nil(t, boss.Email)
	require.False(t, boss.EmailVerified)
	registered, err := users.FindByUsername(ctx, "admin")
	require.NoError(t, err)
	require.Nil(t, registered.Email)

	for _, username := range []string{"Boss", "admin"} {
		got := requestResetCode(router, username, username+"@gmail.com")
		require.Equal(t, http.StatusNotFound, got.Code, username)
		require.Equal(t, "email_user_mismatch", got.Reason, username)
	}

	// An address typed in by an admin is not proof of ownership either.
	emailRec := serveJSON(router, http.MethodPatch, "/admin/users/"+boss.ID+"/email", admin.Token, `{"email":"boss@company.example"}`)
	require.Equal(t, http.StatusOK, emailRec.Code)
	got := requestResetCode(router, "Boss", "boss@company.example")
	require.Equal(t, http.StatusForbidden, got.Code)
	require.Equal(t, "email_not_verified", got.Reason)

	resetRec := serveJSON(router, http.MethodPost, "/auth/password/reset", "",
		`{"username":"Boss","email":"boss@company.example","emailCode":"000000","newPassword":"hijacked1"}`)
	require.Equal(t, http.StatusForbidden, resetRec.Code)
	require.ErrorIs(t, auth.ResetPasswordByUsernameEmail(ctx, "Boss", "boss@company.example", "hijacked1"), service.ErrEmailUserMismatch)
	require.ErrorIs(t, auth.ResetPasswordByEmail(ctx, "boss@company.example", "hijacked1"), service.ErrEmailNotFound)

	require.Empty(t, mailer.codes)
	_, err = auth.Login(ctx, "Boss", "secret123")
	require.NoError(t, err)
}

func TestPasswordResetWorksForCodeAndGoogleVerifiedEmails(t *testing.T) {
	router, users, auth, mailer := newEmailResetTestRouter(t, fakeGoogleVerifier{profiles: map[string]*service.GoogleProfile{
		"google-credential": {
			Subject:       "google-sub-1",
			Email:         "linked@example.com",
			EmailVerified: true,
		},
	}})
	ctx := context.Background()

	invite, err := users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	registered, err := auth.RegisterWithInvite(ctx, "coder", "secret123", "Coder", "coder@example.com", invite.Code)
	require.NoError(t, err)
	require.True(t, registered.User.EmailVerified)

	local, err := auth.Register(ctx, "local", "secret123", "Local")
	require.NoError(t, err)
	bindRec := serveJSON(router, http.MethodPost, "/auth/google/bind", local.Token, `{"credential":"google-credential"}`)
	require.Equal(t, http.StatusOK, bindRec.Code)
	linked, err := users.FindByUsername(ctx, "local")
	require.NoError(t, err)
	require.Equal(t, "linked@example.com", linked.EmailAddress())
	require.True(t, linked.EmailVerified)

	for _, tc := range []struct{ username, email string }{
		{"coder", "coder@example.com"},
		{"local", "linked@example.com"},
	} {
		got := requestResetCode(router, tc.username, tc.email)
		require.Equal(t, http.StatusOK, got.Code, tc.username)
		code, ok := mailer.code(tc.email)
		require.True(t, ok, tc.username)

		resetRec := serveJSON(router, http.MethodPost, "/auth/password/reset", "",
			`{"username":"`+tc.username+`","email":"`+tc.email+`","emailCode":"`+code+`","newPassword":"newsecret1"}`)
		require.Equal(t, http.StatusOK, resetRec.Code, tc.username)
		_, err = auth.Login(ctx, tc.username, "newsecret1")
		require.NoError(t, err, tc.username)
	}

	// Changing the address by hand drops the verification again.
	changed, err := users.UpdateEmail(ctx, registered.User.ID, "typo@example.com")
	require.NoError(t, err)
	require.False(t, changed.EmailVerified)
	got := requestResetCode(router, "coder", "typo@example.com")
	require.Equal(t, http.StatusForbidden, got.Code)
}
