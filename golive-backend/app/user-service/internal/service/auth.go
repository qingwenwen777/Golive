// Package service implements the auth use-cases. It is HTTP-agnostic: handlers
// translate transport (Gin) to/from these methods. This keeps the unit tests
// independent of the web layer.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

// Errors surfaced by AuthService. Handlers should map these to errcode.AppError.
var (
	ErrInvalidCredentials      = errcode.New(401, "Invalid username or password")
	ErrInvalidRefresh          = errcode.New(401, "Invalid refresh token")
	ErrUnauthorized            = errcode.New(401, "Unauthorized")
	ErrInvalidRegister         = errcode.New(http.StatusBadRequest, "Invalid registration details")
	ErrUsernameTaken           = errcode.New(http.StatusConflict, "Username already exists")
	ErrEmailTaken              = errcode.New(http.StatusConflict, "Email already exists").WithReason("email_taken")
	ErrEmailNotFound           = errcode.New(http.StatusNotFound, "Email not found").WithReason("email_not_found")
	ErrEmailUserMismatch       = errcode.New(http.StatusNotFound, "Username and email do not match").WithReason("email_user_mismatch")
	ErrInvalidInvite           = errcode.New(http.StatusBadRequest, "Invalid invite code").WithReason("invalid_invite")
	ErrInviteUsed              = errcode.New(http.StatusConflict, "Invite code already used").WithReason("invite_used")
	ErrInvalidPassword         = errcode.New(http.StatusBadRequest, "Password must be at least 8 characters and include letters and numbers").WithReason("invalid_password")
	ErrGoogleNotConfigured     = errcode.New(http.StatusServiceUnavailable, "Google sign-in is not configured").WithReason("google_not_configured")
	ErrInvalidGoogleCredential = errcode.New(http.StatusUnauthorized, "Invalid Google credential").WithReason("invalid_google_credential")
	ErrGoogleAccountNotFound   = errcode.New(http.StatusNotFound, "Google account is not linked to a GoLive account").WithReason("google_account_not_found")
	ErrGoogleEmailTaken        = errcode.New(http.StatusConflict, "This Google email is already used by another account").WithReason("google_email_exists")
	ErrGoogleAlreadyLinked     = errcode.New(http.StatusConflict, "This Google account is already linked").WithReason("google_already_linked")
	ErrGoogleNotLinked         = errcode.New(http.StatusBadRequest, "This account is not linked to Google").WithReason("google_not_linked")
	ErrUserBanned              = errcode.New(http.StatusForbidden, "This account has been banned").WithReason("user_banned")
	ErrLoginCooldown           = errcode.New(http.StatusTooManyRequests, "Too many failed password attempts. Please wait 1 minute before trying again").WithReason("login_cooldown")
)

const (
	loginFailureLimit   = 5
	loginFailureWindow  = 5 * time.Minute
	loginCooldownPeriod = time.Minute
)

type UserStore interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
}

type userCreator interface {
	Create(ctx context.Context, u *model.User) error
}

type inviteRegistrar interface {
	RegisterWithInvite(ctx context.Context, u *model.User, inviteCode string) error
}

type emailFinder interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error)
}

type googleSubFinder interface {
	FindByGoogleSub(ctx context.Context, sub string) (*model.User, error)
}

type googleLinker interface {
	LinkGoogleAccount(ctx context.Context, id, googleSub, googleEmail string, linkedAt time.Time) (*model.User, error)
}

type googleUnlinker interface {
	UnlinkGoogleAccount(ctx context.Context, id string) (*model.User, error)
}

type passwordResetter interface {
	ResetPasswordByEmail(ctx context.Context, email, hash string) error
}

type usernameEmailPasswordResetter interface {
	ResetPasswordByUsernameEmail(ctx context.Context, username, email, hash string) error
}

type TokenStore interface {
	SaveRefresh(ctx context.Context, token, userID string, ttl time.Duration) error
	LookupRefresh(ctx context.Context, token string) (string, error)
	DeleteRefresh(ctx context.Context, token string) error
	Rotate(ctx context.Context, oldToken, newToken, userID string, ttl time.Duration) error
}

type userRefreshRevoker interface {
	RevokeUserRefresh(ctx context.Context, userID string) error
}

type LoginAttemptStore interface {
	LoginCooldown(ctx context.Context, username string) (time.Duration, bool, error)
	RecordLoginFailure(ctx context.Context, username string, window, cooldown time.Duration, maxAttempts int) (time.Duration, bool, error)
	ClearLoginFailures(ctx context.Context, username string) error
}

type AuthService struct {
	users          UserStore
	tokens         TokenStore
	loginAttempts  LoginAttemptStore
	jwtKeys        *jwtauth.KeySet
	accessTTL      time.Duration
	refreshTTL     time.Duration
	googleClientID string
	googleVerifier GoogleCredentialVerifier
	now            func() time.Time // injectable for tests
}

type Options struct {
	JWTSecret      string
	JWTKeys        *jwtauth.KeySet
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	GoogleClientID string
	GoogleVerifier GoogleCredentialVerifier
}

func NewAuthService(users UserStore, tokens TokenStore, opts Options) *AuthService {
	keys := opts.JWTKeys
	if keys == nil {
		var err error
		keys, err = jwtauth.NewKeySet(opts.JWTSecret, "", nil)
		if err != nil {
			panic("auth service jwt key set: " + err.Error())
		}
	}
	loginAttempts, _ := tokens.(LoginAttemptStore)
	return &AuthService{
		users:          users,
		tokens:         tokens,
		loginAttempts:  loginAttempts,
		jwtKeys:        keys,
		accessTTL:      opts.AccessTTL,
		refreshTTL:     opts.RefreshTTL,
		googleClientID: strings.TrimSpace(opts.GoogleClientID),
		googleVerifier: opts.GoogleVerifier,
		now:            time.Now,
	}
}

// LoginResp returns the public login payload. RefreshToken is transported via
// an HttpOnly cookie by the handler rather than exposed to browser JavaScript.
type LoginResp struct {
	Token        string           `json:"token"`
	RefreshToken string           `json:"-"`
	User         model.PublicUser `json:"user"`
}

// Login validates credentials and issues an access+refresh pair.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResp, error) {
	username = strings.TrimSpace(username)
	if err := s.ensureLoginAllowed(ctx, username); err != nil {
		return nil, err
	}
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return nil, s.recordLoginFailure(ctx, username)
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, s.recordLoginFailure(ctx, username)
	}
	if err := s.clearLoginFailures(ctx, username); err != nil {
		return nil, err
	}

	access, err := s.signAccess(u.ID)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	refresh := uuid.NewString()
	if err := s.tokens.SaveRefresh(ctx, refresh, u.ID, s.refreshTTL); err != nil {
		return nil, fmt.Errorf("save refresh: %w", err)
	}
	return &LoginResp{
		Token:        access,
		RefreshToken: refresh,
		User:         u.Public(),
	}, nil
}

func (s *AuthService) ensureLoginAllowed(ctx context.Context, username string) error {
	if s.loginAttempts == nil {
		return nil
	}
	if _, locked, err := s.loginAttempts.LoginCooldown(ctx, username); err != nil {
		return fmt.Errorf("check login cooldown: %w", err)
	} else if locked {
		return ErrLoginCooldown
	}
	return nil
}

func (s *AuthService) recordLoginFailure(ctx context.Context, username string) error {
	if s.loginAttempts == nil {
		return ErrInvalidCredentials
	}
	if _, locked, err := s.loginAttempts.RecordLoginFailure(ctx, username, loginFailureWindow, loginCooldownPeriod, loginFailureLimit); err != nil {
		return fmt.Errorf("record login failure: %w", err)
	} else if locked {
		return ErrLoginCooldown
	}
	return ErrInvalidCredentials
}

func (s *AuthService) clearLoginFailures(ctx context.Context, username string) error {
	if s.loginAttempts == nil {
		return nil
	}
	if err := s.loginAttempts.ClearLoginFailures(ctx, username); err != nil {
		return fmt.Errorf("clear login failures: %w", err)
	}
	return nil
}

// Register creates a local preview account and returns the same login payload
// shape as Login so the frontend can enter the app immediately.
func (s *AuthService) Register(ctx context.Context, username, password, displayName string) (*LoginResp, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	if len(username) < 3 || len(password) < 3 || displayName == "" {
		return nil, ErrInvalidRegister
	}

	if _, err := s.users.FindByUsername(ctx, username); err == nil {
		return nil, ErrUsernameTaken
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return nil, fmt.Errorf("find user: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := newLocalUser(username, strings.ToLower(username)+"@gmail.com", displayName, hash)
	creator, ok := s.users.(userCreator)
	if !ok {
		return nil, errors.New("user store cannot create users")
	}
	if err := creator.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	access, err := s.signAccess(u.ID)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	refresh := uuid.NewString()
	if err := s.tokens.SaveRefresh(ctx, refresh, u.ID, s.refreshTTL); err != nil {
		return nil, fmt.Errorf("save refresh: %w", err)
	}
	return &LoginResp{
		Token:        access,
		RefreshToken: refresh,
		User:         u.Public(),
	}, nil
}

func (s *AuthService) RegisterWithInvite(ctx context.Context, username, password, displayName, email, inviteCode string) (*LoginResp, error) {
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	cleanEmail, ok := normalizeEmail(email)
	if !ok {
		return nil, ErrInvalidRegister.WithReason("invalid_email")
	}
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if len(username) < 3 || len(password) < 3 || displayName == "" || inviteCode == "" {
		return nil, ErrInvalidRegister
	}

	if err := ValidatePasswordPolicy(password); err != nil {
		return nil, err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := newLocalUser(username, cleanEmail, displayName, hash)
	registrar, ok := s.users.(inviteRegistrar)
	if !ok {
		return nil, errors.New("user store cannot register with invites")
	}
	if err := registrar.RegisterWithInvite(ctx, u, inviteCode); err != nil {
		switch {
		case errors.Is(err, repo.ErrUsernameTaken):
			return nil, ErrUsernameTaken
		case errors.Is(err, repo.ErrEmailTaken):
			return nil, ErrEmailTaken
		case errors.Is(err, repo.ErrInviteNotFound):
			return nil, ErrInvalidInvite
		case errors.Is(err, repo.ErrInviteUsed):
			return nil, ErrInviteUsed
		default:
			return nil, fmt.Errorf("register with invite: %w", err)
		}
	}

	access, err := s.signAccess(u.ID)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	refresh := uuid.NewString()
	if err := s.tokens.SaveRefresh(ctx, refresh, u.ID, s.refreshTTL); err != nil {
		return nil, fmt.Errorf("save refresh: %w", err)
	}
	return &LoginResp{
		Token:        access,
		RefreshToken: refresh,
		User:         u.Public(),
	}, nil
}

func (s *AuthService) ResetPasswordByEmail(ctx context.Context, email, newPassword string) error {
	cleanEmail, ok := normalizeEmail(email)
	if !ok {
		return ErrInvalidRegister.WithReason("invalid_email")
	}
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	resetter, ok := s.users.(passwordResetter)
	if !ok {
		return errors.New("user store cannot reset passwords")
	}
	if err := resetter.ResetPasswordByEmail(ctx, cleanEmail, hash); err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return ErrEmailNotFound
		}
		return fmt.Errorf("reset password: %w", err)
	}
	return nil
}

func (s *AuthService) EnsureUsernameEmailMatch(ctx context.Context, username, email string) error {
	username = strings.TrimSpace(username)
	cleanEmail, ok := normalizeEmail(email)
	if !ok || username == "" {
		return ErrEmailUserMismatch
	}
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return ErrEmailUserMismatch
		}
		return fmt.Errorf("find user: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(u.Email), cleanEmail) {
		return ErrEmailUserMismatch
	}
	return nil
}

func (s *AuthService) ResetPasswordByUsernameEmail(ctx context.Context, username, email, newPassword string) error {
	username = strings.TrimSpace(username)
	cleanEmail, ok := normalizeEmail(email)
	if !ok || username == "" {
		return ErrEmailUserMismatch
	}
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	resetter, ok := s.users.(usernameEmailPasswordResetter)
	if !ok {
		return errors.New("user store cannot reset passwords by username and email")
	}
	if err := resetter.ResetPasswordByUsernameEmail(ctx, username, cleanEmail, hash); err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return ErrEmailUserMismatch
		}
		return fmt.Errorf("reset password: %w", err)
	}
	return nil
}

// RefreshResp is what /api/auth/refresh returns. No `user` per contract.
type RefreshResp struct {
	Token        string `json:"token"`
	RefreshToken string `json:"-"`
}

// Refresh rotates a refresh token: old one is revoked, a new pair is issued.
func (s *AuthService) Refresh(ctx context.Context, oldRefresh string) (*RefreshResp, error) {
	if oldRefresh == "" {
		return nil, ErrInvalidRefresh
	}
	userID, err := s.tokens.LookupRefresh(ctx, oldRefresh)
	if err != nil {
		if errors.Is(err, repo.ErrRefreshNotFound) {
			return nil, ErrInvalidRefresh
		}
		return nil, fmt.Errorf("lookup refresh: %w", err)
	}
	access, err := s.signAccess(userID)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	newRefresh := uuid.NewString()
	if err := s.tokens.Rotate(ctx, oldRefresh, newRefresh, userID, s.refreshTTL); err != nil {
		if errors.Is(err, repo.ErrRefreshNotFound) {
			return nil, ErrInvalidRefresh
		}
		return nil, fmt.Errorf("rotate refresh: %w", err)
	}
	return &RefreshResp{Token: access, RefreshToken: newRefresh}, nil
}

// Logout best-effort revokes the refresh token. The frontend may not always
// send one, so we ignore "" without erroring.
func (s *AuthService) Logout(ctx context.Context, refresh string) error {
	if refresh == "" {
		return nil
	}
	return s.tokens.DeleteRefresh(ctx, refresh)
}

// RevokeUserSessions revokes every refresh token issued to userID, forcing
// each of their sessions to log in again once its access token expires.
// Access tokens are verified statelessly and stay valid until then.
func (s *AuthService) RevokeUserSessions(ctx context.Context, userID string) error {
	revoker, ok := s.tokens.(userRefreshRevoker)
	if !ok {
		return errors.New("token store cannot revoke user sessions")
	}
	return revoker.RevokeUserRefresh(ctx, userID)
}

// Me returns the user identified by an access token. Returns ErrUnauthorized
// for any signature/expiry/lookup failure.
func (s *AuthService) Me(ctx context.Context, accessToken string) (*model.PublicUser, error) {
	uid, err := s.parseAccess(accessToken)
	if err != nil {
		return nil, ErrUnauthorized
	}
	u, err := s.users.FindByID(ctx, uid)
	if err != nil {
		return nil, ErrUnauthorized
	}
	pu := u.Public()
	return &pu, nil
}

// ParseAccess is exposed so the JWT middleware can validate without going
// through Me (avoids a DB hit on every request).
func (s *AuthService) ParseAccess(token string) (string, error) {
	return s.parseAccess(token)
}

func (s *AuthService) RefreshTTL() time.Duration {
	return s.refreshTTL
}

// signAccess produces an access JWT with sub=userID, exp=now+accessTTL.
func (s *AuthService) signAccess(userID string) (string, error) {
	return s.jwtKeys.SignAccess(userID, s.now(), s.accessTTL)
}

func (s *AuthService) parseAccess(token string) (string, error) {
	return s.jwtKeys.VerifyAccess(token)
}

// HashPassword is a convenience for bootstrap and signup.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func ComparePassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

func ValidatePasswordPolicy(password string) error {
	if len([]rune(password)) < 8 {
		return ErrInvalidPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrInvalidPassword
	}
	return nil
}

func newLocalUser(username, email, displayName, hash string) *model.User {
	return &model.User{
		ID:                   uuid.NewString(),
		Username:             username,
		Email:                email,
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               DefaultAvatarURL(displayName),
		CoinBalance:          1200,
		Verified:             false,
		Role:                 model.RoleUser,
		LivePermissionStatus: model.LivePermissionNone,
	}
}

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 || strings.ContainsAny(email, " <>") {
		return "", false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", false
	}
	return email, true
}

func NormalizeEmail(raw string) (string, bool) {
	return normalizeEmail(raw)
}

func urlSafeSeed(seed string) string {
	if seed == "" {
		return "user"
	}
	return url.QueryEscape(seed)
}
