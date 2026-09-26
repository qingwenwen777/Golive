package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type CreateUnbanAppealReq struct {
	Reason string `json:"reason"`
}

func (s *ModerationService) EnsureUserCanInteract(ctx context.Context, userID string) error {
	if s == nil || s.moderation == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	restriction, err := s.moderation.UserRestriction(ctx, userID, s.now())
	if err != nil {
		return err
	}
	if restriction.Banned {
		return errcode.New(http.StatusForbidden, "user is banned").WithReason("user_banned")
	}
	if restriction.Muted {
		return errcode.New(http.StatusForbidden, "user is muted").WithReason("site_muted")
	}
	return nil
}

// UserBanChecker rejects banned users. *ModerationService implements it.
type UserBanChecker interface {
	EnsureUserNotBanned(ctx context.Context, userID string) error
}

// EnsureUserNotBanned is EnsureUserCanInteract without the site-mute check:
// a mute only silences chat/comments, a ban also takes away going live.
func (s *ModerationService) EnsureUserNotBanned(ctx context.Context, userID string) error {
	if s == nil || s.moderation == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	restriction, err := s.moderation.UserRestriction(ctx, userID, s.now())
	if err != nil {
		return err
	}
	if restriction.Banned {
		return errcode.New(http.StatusForbidden, "user is banned").WithReason("user_banned")
	}
	return nil
}

func (s *ModerationService) CreateUnbanAppeal(ctx context.Context, userID string, req CreateUnbanAppealReq) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	reason := trimRunes(strings.TrimSpace(req.Reason), 1000)
	if len([]rune(reason)) < 10 {
		return errcode.New(http.StatusBadRequest, "appeal reason is required").WithReason("appeal_reason_required")
	}
	restriction, err := s.moderation.UserRestriction(ctx, userID, s.now())
	if err != nil {
		return err
	}
	if !restriction.Banned {
		return errcode.New(http.StatusConflict, "user is not banned").WithReason("not_banned")
	}
	now := s.now()
	return s.moderation.CreateUnbanAppeal(ctx, &model.UnbanAppeal{
		ID:        uuid.NewString(),
		UserID:    userID,
		Reason:    reason,
		Status:    model.UnbanAppealPending,
		CreatedAt: now,
		UpdatedAt: now,
	})
}
