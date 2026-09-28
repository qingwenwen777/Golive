package server

import (
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

const loginCodeTestPassword = "secret123"

// loginEmail is how email codes are set up for a loginCodeEnv.
type loginEmail int

const (
	loginEmailWorking   loginEmail = iota // codes go out through a recordingMailer
	loginEmailNoService                   // Deps.EmailCodes is nil
	loginEmailNoMailer                    // what cmd/main.go builds when email is disabled
)

// loginCodeEnv is user-service with the login captcha on, as cmd/main.go
// always has it, so a login that gets past it without solving it shows that
// the email code stood in for it.
type loginCodeEnv struct {
	router *gin.Engine
	users  *repo.UserRepo
	auth   *service.AuthService
	rdb    *redis.Client
	mailer *recordingMailer
}

func newLoginCodeEnv(t *testing.T, email loginEmail) *loginCodeEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	users := repo.NewUserRepo(db)
	require.NoError(t, users.AutoMigrate())

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	env := &loginCodeEnv{
		users: users,
		auth: service.NewAuthService(users, repo.NewTokenRepo(rdb), service.Options{
			JWTSecret:  "test-secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		}),
		rdb:    rdb,
		mailer: &recordingMailer{codes: map[string]string{}},
	}
	var emailCodes *service.EmailCodeService
	switch email {
	case loginEmailWorking:
		emailCodes = service.NewEmailCodeService(rdb, env.mailer, 10*time.Minute, time.Minute)
	case loginEmailNoMailer:
		emailCodes = service.NewEmailCodeService(rdb, nil, 10*time.Minute, time.Minute)
	}
	env.router = NewRouter(Deps{
		Auth:       env.auth,
		Captcha:    service.NewCaptchaService(rdb, 5*time.Minute),
		EmailCodes: emailCodes,
		Users:      users,
	})
	return env
}

func (e *loginCodeEnv) post(path string, body map[string]string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	return serveJSON(e.router, http.MethodPost, path, "", string(raw))
}

// registerWithEmail creates an account whose email was verified at sign-up.
func (e *loginCodeEnv) registerWithEmail(t *testing.T, username, email string) {
	t.Helper()
	ctx := context.Background()
	invite, err := e.users.CreateInviteCode(ctx, "admin")
	require.NoError(t, err)
	registered, err := e.auth.RegisterWithInvite(ctx, username, loginCodeTestPassword, username, email, invite.Code)
	require.NoError(t, err)
	require.True(t, registered.User.EmailVerified)
}

// sendLoginCode asks for a login code the way the sign-in form does.
func (e *loginCodeEnv) sendLoginCode(username, email string) *httptest.ResponseRecorder {
	return e.post("/auth/email-code", map[string]string{"purpose": service.EmailPurposeLogin, "username": username, "email": email})
}

// mailedCode returns the last code emailed to email.
func (e *loginCodeEnv) mailedCode(t *testing.T, email string) string {
	t.Helper()
	code, ok := e.mailer.code(email)
	require.True(t, ok, "no code was emailed to %s", email)
	return code
}

// loginWithCode signs in with an email code instead of the captcha.
func (e *loginCodeEnv) loginWithCode(username, password, email, code string) *httptest.ResponseRecorder {
	return e.post("/auth/login", map[string]string{"username": username, "password": password, "email": email, "emailCode": code})
}

// captcha fetches a login captcha and reads its answer from Redis, as
// scripts/smoke/smoke.py does.
func (e *loginCodeEnv) captcha(t *testing.T) (id, answer string) {
	t.Helper()
	rec := serveJSON(e.router, http.MethodGet, "/auth/captcha", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var challenge service.CaptchaChallenge
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &challenge))
	answer, err := e.rdb.Get(context.Background(), "captcha:"+challenge.ID).Result()
	require.NoError(t, err)
	return challenge.ID, answer
}

func (e *loginCodeEnv) loginWithCaptcha(t *testing.T, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	id, answer := e.captcha(t)
	return e.post("/auth/login", map[string]string{"username": username, "password": password, "captchaId": id, "captchaCode": answer})
}

func requireSignedIn(t *testing.T, rec *httptest.ResponseRecorder, username string) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.Token)
	require.Equal(t, username, out.User.Username)
	require.Contains(t, rec.Header().Get("Set-Cookie"), "golive_refresh=")
}

func requireRejected(t *testing.T, rec *httptest.ResponseRecorder, status int, reason string) {
	t.Helper()
	requireReason(t, rec.Body.Bytes(), status, rec.Code, reason)
}

// otherCode returns a six-digit code that is not code.
func otherCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

// People who cannot read the captcha image get a code emailed to their
// account instead. It replaces the captcha, not the password.
func TestLoginWithEmailCodeInsteadOfCaptcha(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailWorking)
	env.registerWithEmail(t, "coder", "coder@example.com")

	require.Equal(t, http.StatusOK, env.sendLoginCode("coder", "coder@example.com").Code)
	code := env.mailedCode(t, "coder@example.com")
	requireSignedIn(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code), "coder")
	// A code signs in once.
	requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code),
		http.StatusBadRequest, "invalid_email_code")

	// The password is still checked, and a wrong one uses the code up.
	require.Equal(t, http.StatusOK, env.sendLoginCode("coder", "coder@example.com").Code)
	code = env.mailedCode(t, "coder@example.com")
	requireRejected(t, env.loginWithCode("coder", "wrong-pass1", "coder@example.com", code), http.StatusUnauthorized, "")
	requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code),
		http.StatusBadRequest, "invalid_email_code")
}

// The code is checked before the password, so without the mailbox the email
// path says nothing about the password.
func TestLoginEmailCodeIsCheckedBeforeThePassword(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailWorking)
	env.registerWithEmail(t, "coder", "coder@example.com")
	require.Equal(t, http.StatusOK, env.sendLoginCode("coder", "coder@example.com").Code)
	code := env.mailedCode(t, "coder@example.com")
	wrong := otherCode(code)

	rightPassword := env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", wrong)
	requireRejected(t, rightPassword, http.StatusBadRequest, "invalid_email_code")
	for range 4 {
		wrongPassword := env.loginWithCode("coder", "wrong-pass1", "coder@example.com", wrong)
		require.Equal(t, rightPassword.Code, wrongPassword.Code)
		require.Equal(t, rightPassword.Body.String(), wrongPassword.Body.String())
	}
	// The password was not tried: had those four wrong passwords counted,
	// this fifth one would lock the account (login_cooldown).
	requireRejected(t, env.loginWithCaptcha(t, "coder", "wrong-pass1"), http.StatusUnauthorized, "")
	// Five wrong guesses used the code up.
	requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code),
		http.StatusBadRequest, "invalid_email_code")
}

// Login codes go only to the account's own verified email, and sign in only
// that account.
func TestLoginEmailCodeNeedsTheAccountsVerifiedEmail(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailWorking)
	ctx := context.Background()
	env.registerWithEmail(t, "coder", "coder@example.com")
	env.registerWithEmail(t, "other", "other@example.com")
	typed, err := env.auth.Register(ctx, "typed", loginCodeTestPassword, "Typed")
	require.NoError(t, err)

	for _, tc := range []struct{ username, email string }{
		{"coder", "other@example.com"},  // another account's email
		{"coder", "nobody@example.com"}, // nobody's email
		{"nobody", "coder@example.com"}, // no such account
		{"typed", "typed@gmail.com"},    // no email on file
	} {
		requireRejected(t, env.sendLoginCode(tc.username, tc.email), http.StatusNotFound, "email_user_mismatch")
	}
	// An address typed in by hand (by an admin, say) was never proven.
	_, err = env.users.UpdateEmail(ctx, typed.User.ID, "typed@example.com")
	require.NoError(t, err)
	requireRejected(t, env.sendLoginCode("typed", "typed@example.com"), http.StatusForbidden, "email_not_verified")
	require.Empty(t, env.mailer.codes)

	require.Equal(t, http.StatusOK, env.sendLoginCode("coder", "coder@example.com").Code)
	code := env.mailedCode(t, "coder@example.com")
	requireRejected(t, env.loginWithCode("other", loginCodeTestPassword, "coder@example.com", code),
		http.StatusNotFound, "email_user_mismatch")
	requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "other@example.com", code),
		http.StatusNotFound, "email_user_mismatch")
	requireRejected(t, env.loginWithCode("typed", loginCodeTestPassword, "typed@example.com", code),
		http.StatusForbidden, "email_not_verified")
	// Those were turned away before the code was tried, so it still works.
	requireSignedIn(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code), "coder")
}

// Codes are bound to their purpose: sign-up and password reset codes don't
// sign in, and a login code doesn't reset the password.
func TestLoginEmailCodesAreOnlyForSignIn(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailWorking)
	env.registerWithEmail(t, "coder", "coder@example.com")

	require.Equal(t, http.StatusOK, env.sendLoginCode("coder", "coder@example.com").Code)
	loginCode := env.mailedCode(t, "coder@example.com")
	reset := env.post("/auth/password/reset", map[string]string{
		"username": "coder", "email": "coder@example.com", "emailCode": loginCode, "newPassword": "hijacked1",
	})
	requireRejected(t, reset, http.StatusBadRequest, "invalid_email_code")
	requireSignedIn(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", loginCode), "coder")

	for _, purpose := range []string{service.EmailPurposeRegister, service.EmailPurposePasswordReset} {
		sent := env.post("/auth/email-code", map[string]string{"purpose": purpose, "username": "coder", "email": "coder@example.com"})
		require.Equal(t, http.StatusOK, sent.Code, purpose)
		code := env.mailedCode(t, "coder@example.com")
		requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", code),
			http.StatusBadRequest, "invalid_email_code")
	}
}

// Without an email code, sign-in takes the captcha exactly as before.
func TestLoginWithoutEmailCodeStillNeedsTheCaptcha(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailWorking)
	env.registerWithEmail(t, "coder", "coder@example.com")

	id, _ := env.captcha(t)
	for _, body := range []map[string]string{
		{"username": "coder", "password": loginCodeTestPassword},
		{"username": "coder", "password": loginCodeTestPassword, "captchaId": id, "captchaCode": "WRONG1"},
		// An email without a code is not the email path.
		{"username": "coder", "password": loginCodeTestPassword, "email": "coder@example.com"},
		{"username": "coder", "password": loginCodeTestPassword, "email": "coder@example.com", "emailCode": " "},
	} {
		requireRejected(t, env.post("/auth/login", body), http.StatusBadRequest, "invalid_captcha")
	}
	requireRejected(t, env.loginWithCaptcha(t, "coder", "wrong-pass1"), http.StatusUnauthorized, "")
	requireSignedIn(t, env.loginWithCaptcha(t, "coder", loginCodeTestPassword), "coder")
}

// Without email the email path is closed, whatever the password; the captcha
// still works.
func TestLoginEmailCodeFailsClosedWithoutEmail(t *testing.T) {
	env := newLoginCodeEnv(t, loginEmailNoService)
	env.registerWithEmail(t, "coder", "coder@example.com")
	requireRejected(t, env.sendLoginCode("coder", "coder@example.com"), http.StatusServiceUnavailable, "email_not_configured")
	for _, password := range []string{loginCodeTestPassword, "wrong-pass1"} {
		requireRejected(t, env.loginWithCode("coder", password, "coder@example.com", "123456"),
			http.StatusServiceUnavailable, "email_not_configured")
	}
	requireSignedIn(t, env.loginWithCaptcha(t, "coder", loginCodeTestPassword), "coder")

	// cmd/main.go builds the service without a mailer when email is
	// disabled: no code is ever sent, so none is accepted.
	env = newLoginCodeEnv(t, loginEmailNoMailer)
	env.registerWithEmail(t, "coder", "coder@example.com")
	requireRejected(t, env.sendLoginCode("coder", "coder@example.com"), http.StatusServiceUnavailable, "email_not_configured")
	requireRejected(t, env.loginWithCode("coder", loginCodeTestPassword, "coder@example.com", "123456"),
		http.StatusBadRequest, "invalid_email_code")
	requireSignedIn(t, env.loginWithCaptcha(t, "coder", loginCodeTestPassword), "coder")
}
