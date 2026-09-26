package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
)

var usernameCharsRe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type GoogleProfile struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

type GoogleCredentialVerifier interface {
	VerifyGoogleCredential(ctx context.Context, credential, audience string) (*GoogleProfile, error)
}

type GoogleEmailTakenError struct {
	Email string
}

func (e *GoogleEmailTakenError) Error() string { return ErrGoogleEmailTaken.Error() }

func (e *GoogleEmailTakenError) Unwrap() error { return ErrGoogleEmailTaken }

func (s *AuthService) GoogleLogin(ctx context.Context, credential string) (*LoginResp, error) {
	profile, err := s.verifyGoogleProfile(ctx, credential)
	if err != nil {
		return nil, err
	}

	u, err := s.findByGoogleSub(ctx, profile.Subject)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			if cleanEmail, ok := normalizeEmail(profile.Email); ok {
				if _, emailErr := s.findByEmail(ctx, cleanEmail); emailErr == nil {
					return nil, &GoogleEmailTakenError{Email: cleanEmail}
				} else if !errors.Is(emailErr, repo.ErrUserNotFound) {
					return nil, fmt.Errorf("find email: %w", emailErr)
				}
			}
			return nil, ErrGoogleAccountNotFound
		}
		return nil, fmt.Errorf("find google user: %w", err)
	}
	return s.loginResponseForUser(ctx, u)
}

func (s *AuthService) GoogleRegisterWithInvite(
	ctx context.Context,
	credential, username, displayName, inviteCode string,
) (*LoginResp, error) {
	profile, err := s.verifyGoogleProfile(ctx, credential)
	if err != nil {
		return nil, err
	}

	if u, err := s.findByGoogleSub(ctx, profile.Subject); err == nil {
		return s.loginResponseForUser(ctx, u)
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return nil, fmt.Errorf("find google user: %w", err)
	}

	cleanEmail, ok := normalizeEmail(profile.Email)
	if !ok {
		return nil, ErrInvalidGoogleCredential
	}
	if _, err := s.findByEmail(ctx, cleanEmail); err == nil {
		return nil, &GoogleEmailTakenError{Email: cleanEmail}
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return nil, fmt.Errorf("find email: %w", err)
	}

	username = cleanGoogleUsername(username, cleanEmail)
	displayName = trimDisplayName(displayName)
	if displayName == "" {
		displayName = trimDisplayName(profile.Name)
	}
	if displayName == "" {
		displayName = username
	}
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if len(username) < 3 || displayName == "" || inviteCode == "" {
		return nil, ErrInvalidRegister
	}

	hash, err := HashPassword(uuid.NewString() + uuid.NewString())
	if err != nil {
		return nil, fmt.Errorf("hash google placeholder password: %w", err)
	}

	u := newGoogleUser(username, cleanEmail, displayName, hash, profile)
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
			return nil, fmt.Errorf("register google with invite: %w", err)
		}
	}

	return s.loginResponseForUser(ctx, u)
}

func (s *AuthService) GoogleLinkByPassword(ctx context.Context, credential, password string) (*LoginResp, error) {
	profile, err := s.verifyGoogleProfile(ctx, credential)
	if err != nil {
		return nil, err
	}
	if u, err := s.findByGoogleSub(ctx, profile.Subject); err == nil {
		return s.loginResponseForUser(ctx, u)
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return nil, fmt.Errorf("find google user: %w", err)
	}

	cleanEmail, ok := normalizeEmail(profile.Email)
	if !ok {
		return nil, ErrInvalidGoogleCredential
	}
	u, err := s.findByEmail(ctx, cleanEmail)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return nil, ErrGoogleAccountNotFound
		}
		return nil, fmt.Errorf("find email: %w", err)
	}
	if err := ComparePassword(u.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials.WithReason("invalid_current_password")
	}
	linked, err := s.linkGoogle(ctx, u.ID, profile.Subject, cleanEmail)
	if err != nil {
		return nil, err
	}
	return s.loginResponseForUser(ctx, linked)
}

func (s *AuthService) GoogleLinkCurrentUser(ctx context.Context, userID, credential string) (*model.PublicUser, error) {
	profile, err := s.verifyGoogleProfile(ctx, credential)
	if err != nil {
		return nil, err
	}
	if existing, err := s.findByGoogleSub(ctx, profile.Subject); err == nil {
		if existing.ID == userID {
			pu := existing.Public()
			return &pu, nil
		}
		return nil, ErrGoogleAlreadyLinked
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return nil, fmt.Errorf("find google user: %w", err)
	}
	if cleanEmail, ok := normalizeEmail(profile.Email); ok {
		if existing, err := s.findByEmail(ctx, cleanEmail); err == nil && existing.ID != userID {
			return nil, ErrGoogleEmailTaken
		} else if err != nil && !errors.Is(err, repo.ErrUserNotFound) {
			return nil, fmt.Errorf("find email: %w", err)
		}
	}
	cleanEmail, _ := normalizeEmail(profile.Email)
	linked, err := s.linkGoogle(ctx, userID, profile.Subject, cleanEmail)
	if err != nil {
		return nil, err
	}
	pu := linked.Public()
	return &pu, nil
}

func (s *AuthService) GoogleUnlinkCurrentUser(ctx context.Context, userID, password string) (*model.PublicUser, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if u.GoogleSub == nil || strings.TrimSpace(*u.GoogleSub) == "" {
		return nil, ErrGoogleNotLinked
	}
	if err := ComparePassword(u.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials.WithReason("invalid_current_password")
	}
	unlinker, ok := s.users.(googleUnlinker)
	if !ok {
		return nil, errors.New("user store cannot unlink google accounts")
	}
	updated, err := unlinker.UnlinkGoogleAccount(ctx, userID)
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return nil, ErrUnauthorized
		}
		if errors.Is(err, repo.ErrGoogleNotLinked) {
			return nil, ErrGoogleNotLinked
		}
		return nil, fmt.Errorf("unlink google account: %w", err)
	}
	pu := updated.Public()
	return &pu, nil
}

func (s *AuthService) verifyGoogleProfile(ctx context.Context, credential string) (*GoogleProfile, error) {
	if strings.TrimSpace(s.googleClientID) == "" {
		return nil, ErrGoogleNotConfigured
	}
	if strings.TrimSpace(credential) == "" {
		return nil, ErrInvalidGoogleCredential
	}
	verifier := s.googleVerifier
	if verifier == nil {
		verifier = defaultGoogleVerifier{}
	}
	profile, err := verifier.VerifyGoogleCredential(ctx, credential, s.googleClientID)
	if err != nil {
		return nil, ErrInvalidGoogleCredential
	}
	if profile == nil {
		return nil, ErrInvalidGoogleCredential
	}
	profile.Subject = strings.TrimSpace(profile.Subject)
	profile.Email = strings.ToLower(strings.TrimSpace(profile.Email))
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Picture = strings.TrimSpace(profile.Picture)
	if profile.Subject == "" || profile.Email == "" || !profile.EmailVerified {
		return nil, ErrInvalidGoogleCredential
	}
	return profile, nil
}

func (s *AuthService) findByGoogleSub(ctx context.Context, sub string) (*model.User, error) {
	finder, ok := s.users.(googleSubFinder)
	if !ok {
		return nil, errors.New("user store cannot find google accounts")
	}
	return finder.FindByGoogleSub(ctx, sub)
}

func (s *AuthService) findByEmail(ctx context.Context, email string) (*model.User, error) {
	finder, ok := s.users.(emailFinder)
	if !ok {
		return nil, errors.New("user store cannot find emails")
	}
	return finder.FindByEmail(ctx, email)
}

func (s *AuthService) linkGoogle(ctx context.Context, userID, googleSub, googleEmail string) (*model.User, error) {
	linker, ok := s.users.(googleLinker)
	if !ok {
		return nil, errors.New("user store cannot link google accounts")
	}
	u, err := linker.LinkGoogleAccount(ctx, userID, googleSub, googleEmail, s.now().UTC())
	if err != nil {
		if errors.Is(err, repo.ErrUserNotFound) {
			return nil, ErrUnauthorized
		}
		if errors.Is(err, repo.ErrGoogleAlreadyLinked) {
			return nil, ErrGoogleAlreadyLinked
		}
		if errors.Is(err, repo.ErrEmailTaken) {
			return nil, ErrGoogleEmailTaken
		}
		return nil, fmt.Errorf("link google account: %w", err)
	}
	return u, nil
}

func (s *AuthService) loginResponseForUser(ctx context.Context, u *model.User) (*LoginResp, error) {
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

func newGoogleUser(username, email, displayName, hash string, profile *GoogleProfile) *model.User {
	avatar := strings.TrimSpace(profile.Picture)
	if avatar == "" {
		avatar = DefaultAvatarURL(displayName)
	}
	sub := profile.Subject
	now := time.Now().UTC()
	return &model.User{
		ID:                   uuid.NewString(),
		Username:             username,
		Email:                &email,
		EmailVerified:        true, // verifyGoogleProfile requires email_verified
		GoogleSub:            &sub,
		GoogleLinkedAt:       &now,
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               avatar,
		CoinBalance:          1200,
		Verified:             false,
		Role:                 model.RoleUser,
		LivePermissionStatus: model.LivePermissionNone,
	}
}

func cleanGoogleUsername(username, email string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		local := strings.Split(email, "@")[0]
		username = usernameCharsRe.ReplaceAllString(local, "_")
		username = strings.Trim(username, "_.-")
	}
	if len([]rune(username)) > 32 {
		username = string([]rune(username)[:32])
	}
	return username
}

func trimDisplayName(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 64 {
		return string(runes[:64])
	}
	return s
}
