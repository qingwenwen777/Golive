// Package service implements the auth use-cases. It is HTTP-agnostic: handlers
// translate transport (Gin) to/from these methods. This keeps the unit tests
// independent of the web layer.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// Errors surfaced by AuthService. Handlers should map these to errcode.AppError.
var (
	ErrInvalidCredentials = errcode.New(401, "Invalid username or password")
	ErrInvalidRefresh     = errcode.New(401, "Invalid refresh token")
	ErrUnauthorized       = errcode.New(401, "Unauthorized")
	ErrInvalidRegister    = errcode.New(http.StatusBadRequest, "Invalid registration details")
	ErrUsernameTaken      = errcode.New(http.StatusConflict, "Username already exists")
)

type UserStore interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
}

type userCreator interface {
	Create(ctx context.Context, u *model.User) error
}

type TokenStore interface {
	SaveRefresh(ctx context.Context, token, userID string, ttl time.Duration) error
	LookupRefresh(ctx context.Context, token string) (string, error)
	DeleteRefresh(ctx context.Context, token string) error
	Rotate(ctx context.Context, oldToken, newToken, userID string, ttl time.Duration) error
}

type AuthService struct {
	users      UserStore
	tokens     TokenStore
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time // injectable for tests
}

type Options struct {
	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

func NewAuthService(users UserStore, tokens TokenStore, opts Options) *AuthService {
	return &AuthService{
		users:      users,
		tokens:     tokens,
		jwtSecret:  []byte(opts.JWTSecret),
		accessTTL:  opts.AccessTTL,
		refreshTTL: opts.RefreshTTL,
		now:        time.Now,
	}
}

// LoginResp matches src/types/user.ts LoginResp exactly.
type LoginResp struct {
	Token        string           `json:"token"`
	RefreshToken string           `json:"refreshToken"`
	User         model.PublicUser `json:"user"`
}

// Login validates credentials and issues an access+refresh pair.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResp, error) {
	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
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

	u := &model.User{
		ID:                   uuid.NewString(),
		Username:             username,
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=" + urlSafeSeed(displayName),
		CoinBalance:          1200,
		Verified:             false,
		Role:                 model.RoleUser,
		LivePermissionStatus: model.LivePermissionNone,
	}
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

// RefreshResp is what /api/auth/refresh returns. No `user` per contract.
type RefreshResp struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
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

// signAccess produces an HS256 JWT with sub=userID, exp=now+accessTTL.
func (s *AuthService) signAccess(userID string) (string, error) {
	now := s.now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(s.accessTTL).Unix(),
		"typ": "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

func (s *AuthService) parseAccess(token string) (string, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing sub")
	}
	return sub, nil
}

// HashPassword is a convenience for bootstrap and signup.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func urlSafeSeed(seed string) string {
	if seed == "" {
		return "user"
	}
	return url.QueryEscape(seed)
}
