package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	moderationRoleViewer    = "viewer"
	moderationRoleOwner     = "owner"
	moderationRoleModerator = "moderator"
)

type ModerationService struct {
	moderation *repo.ModerationRepo
	rooms      *repo.RoomRepo
	social     *repo.SocialRepo
	live       *LiveService
	owners     OwnerServices
	runtime    SystemRuntimeConfig
	now        func() time.Time
}

func NewModerationService(moderation *repo.ModerationRepo, rooms *repo.RoomRepo, social *repo.SocialRepo) *ModerationService {
	return &ModerationService{
		moderation: moderation,
		rooms:      rooms,
		social:     social,
		now:        time.Now,
	}
}

func (s *ModerationService) SetLiveService(live *LiveService) {
	s.live = live
}

// SetOwnerServices wires the clients report actions use for data other
// services own (bans/mutes, super chats, chat messages).
func (s *ModerationService) SetOwnerServices(owners OwnerServices) {
	s.owners = owners
}

type ModerationUserDTO struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	Verified    bool   `json:"verified"`
	Moderator   bool   `json:"moderator"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type ModerationUserListResp struct {
	Items []ModerationUserDTO `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"size"`
}

func (s *ModerationService) moderationActorRole(ctx context.Context, room *model.Room, actorID string) (string, error) {
	if actorID == "" {
		return "", errcode.New(401, "Unauthorized")
	}
	if actorID == room.OwnerID {
		return moderationRoleOwner, nil
	}
	ok, err := s.moderation.IsModerator(ctx, room.OwnerID, actorID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errcode.New(403, "not allowed to moderate in this room")
	}
	return moderationRoleModerator, nil
}

func moderationUsers(rows []repo.ModerationUser) []ModerationUserDTO {
	out := make([]ModerationUserDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, moderationUser(row))
	}
	return out
}

func moderationUser(row repo.ModerationUser) ModerationUserDTO {
	dto := ModerationUserDTO{
		ID:          row.ID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		Name:        row.Name,
		Avatar:      row.Avatar,
		Verified:    row.Verified,
		Moderator:   row.Moderator,
	}
	if row.CreatedAt != nil && !row.CreatedAt.IsZero() {
		dto.CreatedAt = row.CreatedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

// requireAdmin guards platform administration (dashboard, audit logs, system
// settings). Platform moderators are not admins. A banned admin keeps the
// role (banned users can still sign in to appeal) but loses its powers.
func (s *ModerationService) requireAdmin(ctx context.Context, userID string) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	ok, err := s.moderation.IsAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.New(http.StatusForbidden, "admin access required")
	}
	return s.EnsureUserNotBanned(ctx, userID)
}

// requireContentModerator guards content review (reports, blocked words),
// which admins and platform moderators share; banned staff lose it like
// requireAdmin. Returns the caller's role.
func (s *ModerationService) requireContentModerator(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", errcode.ErrUnauthorized
	}
	role, err := s.moderation.UserRole(ctx, userID)
	if err != nil {
		return "", err
	}
	if role != repo.RoleAdmin && role != repo.RoleModerator {
		return "", errcode.New(http.StatusForbidden, "content moderator access required")
	}
	if err := s.EnsureUserNotBanned(ctx, userID); err != nil {
		return "", err
	}
	return role, nil
}

func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeSize(size int) int {
	if size < 1 {
		return 10
	}
	if size > 100 {
		return 100
	}
	return size
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *ModerationService) notifyModeration(ctx context.Context, userID, kind, title, body, link, adminID string, now time.Time) error {
	return s.moderation.CreateNotification(ctx, model.Notification{
		ID:        "mod-" + uuid.NewString(),
		UserID:    userID,
		Type:      kind,
		Title:     title,
		Body:      body,
		Link:      link,
		ActorID:   adminID,
		ActorName: "GoLive Admin",
		CreatedAt: now,
	})
}
