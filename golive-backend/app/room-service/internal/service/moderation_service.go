package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	moderationRoleViewer    = "viewer"
	moderationRoleOwner     = "owner"
	moderationRoleModerator = "moderator"
)

var allowedMuteDurations = map[int]struct{}{
	5:  {},
	10: {},
	30: {},
	60: {},
}

type ModerationService struct {
	moderation *repo.ModerationRepo
	rooms      *repo.RoomRepo
	social     *repo.SocialRepo
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

type ModerationLogDTO struct {
	ID              string `json:"id"`
	OwnerID         string `json:"ownerId"`
	RoomID          string `json:"roomId,omitempty"`
	ActorID         string `json:"actorId"`
	ActorName       string `json:"actorName"`
	ActorAvatar     string `json:"actorAvatar,omitempty"`
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	TargetAvatar    string `json:"targetAvatar,omitempty"`
	Action          string `json:"action"`
	DurationMinutes int    `json:"durationMinutes,omitempty"`
	CreatedAt       string `json:"createdAt"`
}

type ModerationLogListResp struct {
	Items []ModerationLogDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
}

type RoomModerationState struct {
	RoomID               string `json:"roomId"`
	OwnerID              string `json:"ownerId"`
	Role                 string `json:"role"`
	CanModerate          bool   `json:"canModerate"`
	Muted                bool   `json:"muted"`
	MuteExpiresAt        string `json:"muteExpiresAt,omitempty"`
	MuteRemainingSeconds int64  `json:"muteRemainingSeconds,omitempty"`
}

type MuteReq struct {
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	TargetAvatar    string `json:"targetAvatar"`
	DurationMinutes int    `json:"durationMinutes"`
}

type MuteResp struct {
	RoomID          string `json:"roomId"`
	TargetUserID    string `json:"targetUserId"`
	TargetName      string `json:"targetName"`
	DurationMinutes int    `json:"durationMinutes"`
	ExpiresAt       string `json:"expiresAt"`
}

type MuteStateResp struct {
	RoomID               string `json:"roomId"`
	TargetUserID         string `json:"targetUserId"`
	TargetName           string `json:"targetName,omitempty"`
	TargetAvatar         string `json:"targetAvatar,omitempty"`
	Muted                bool   `json:"muted"`
	MuteExpiresAt        string `json:"muteExpiresAt,omitempty"`
	MuteRemainingSeconds int64  `json:"muteRemainingSeconds,omitempty"`
}

func (s *ModerationService) ListFollowers(ctx context.Context, ownerID, query string, page, size int) (*ModerationUserListResp, error) {
	followerIDs, err := s.social.Followers(ctx, "ch-"+ownerID)
	if err != nil {
		return nil, err
	}
	followerIDs = excludeUserID(followerIDs, ownerID)
	items, total, err := s.moderation.ListFollowers(ctx, ownerID, query, page, size, followerIDs)
	if err != nil {
		return nil, err
	}
	return &ModerationUserListResp{
		Items: moderationUsers(items),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) ListModerators(ctx context.Context, ownerID string, page, size int) (*ModerationUserListResp, error) {
	items, total, err := s.moderation.ListModerators(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	return &ModerationUserListResp{
		Items: moderationUsers(items),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) AddModerator(ctx context.Context, ownerID, targetUserID string) (*ModerationUserDTO, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" || targetUserID == ownerID {
		return nil, errcode.New(400, "invalid moderator")
	}
	following, err := s.social.IsFollowing(ctx, targetUserID, "ch-"+ownerID)
	if err != nil {
		return nil, err
	}
	if !following {
		return nil, errcode.New(409, "moderator must be selected from your followers").WithReason("not_follower")
	}
	now := s.now()
	if err := s.moderation.AddModerator(ctx, ownerID, ownerID, targetUserID, now); err != nil {
		return nil, err
	}
	if err := s.syncOwnerActiveRooms(ctx, ownerID); err != nil {
		return nil, err
	}
	profile, err := s.moderation.UserProfile(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	profile.Moderator = true
	profile.CreatedAt = &now
	dto := moderationUser(profile)
	return &dto, nil
}

func (s *ModerationService) RemoveModerator(ctx context.Context, ownerID, targetUserID string) error {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" || targetUserID == ownerID {
		return errcode.New(400, "invalid moderator")
	}
	if err := s.moderation.RemoveModerator(ctx, ownerID, ownerID, targetUserID, s.now()); err != nil {
		if errors.Is(err, repo.ErrModeratorNotFound) {
			return errcode.New(404, "moderator not found")
		}
		return err
	}
	return s.syncOwnerActiveRooms(ctx, ownerID)
}

func (s *ModerationService) Logs(ctx context.Context, ownerID string, page, size int) (*ModerationLogListResp, error) {
	rows, total, err := s.moderation.Logs(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]ModerationLogDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, ModerationLogDTO{
			ID:              row.ID,
			OwnerID:         row.OwnerID,
			RoomID:          row.RoomID,
			ActorID:         row.ActorID,
			ActorName:       row.ActorName,
			ActorAvatar:     row.ActorAvatar,
			TargetUserID:    row.TargetUserID,
			TargetName:      row.TargetName,
			TargetAvatar:    row.TargetAvatar,
			Action:          row.Action,
			DurationMinutes: row.DurationMinutes,
			CreatedAt:       row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &ModerationLogListResp{Items: items, Total: total, Page: normalizePage(page), Size: normalizeSize(size)}, nil
}

func (s *ModerationService) RoomState(ctx context.Context, roomID, userID string) (*RoomModerationState, error) {
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	now := s.now()
	role := moderationRoleViewer
	if userID != "" && userID == room.OwnerID {
		role = moderationRoleOwner
	} else if userID != "" {
		ok, err := s.moderation.IsModerator(ctx, room.OwnerID, userID)
		if err != nil {
			return nil, err
		}
		if ok {
			role = moderationRoleModerator
		}
	}
	resp := &RoomModerationState{
		RoomID:      room.ID,
		OwnerID:     room.OwnerID,
		Role:        role,
		CanModerate: role == moderationRoleOwner || role == moderationRoleModerator,
	}
	if userID != "" {
		mute, err := s.moderation.CurrentMute(ctx, room.ID, userID, now)
		if err != nil {
			return nil, err
		}
		resp.Muted = mute != nil
		if mute != nil {
			resp.MuteExpiresAt = mute.ExpiresAt.UTC().Format(time.RFC3339)
			resp.MuteRemainingSeconds = int64(mute.ExpiresAt.Sub(now).Seconds())
			if resp.MuteRemainingSeconds < 0 {
				resp.MuteRemainingSeconds = 0
			}
		}
	}
	return resp, nil
}

func (s *ModerationService) Mute(ctx context.Context, roomID, actorID string, req MuteReq) (*MuteResp, error) {
	if actorID == "" {
		return nil, errcode.New(401, "Unauthorized")
	}
	if _, ok := allowedMuteDurations[req.DurationMinutes]; !ok {
		return nil, errcode.New(400, "duration must be one of 5, 10, 30, 60 minutes")
	}
	targetUserID := strings.TrimSpace(req.TargetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if targetUserID == room.OwnerID {
		return nil, errcode.New(409, "cannot mute the creator").WithReason("target_is_owner")
	}
	targetIsMod, err := s.moderation.IsModerator(ctx, room.OwnerID, targetUserID)
	if err != nil {
		return nil, err
	}
	if targetIsMod {
		return nil, errcode.New(409, "cannot mute a room moderator").WithReason("target_is_moderator")
	}

	role, err := s.moderationActorRole(ctx, room, actorID)
	if err != nil {
		return nil, err
	}
	if actorID == targetUserID {
		return nil, errcode.New(409, "cannot mute yourself")
	}

	mute, err := s.moderation.MuteUser(
		ctx,
		room.OwnerID,
		room.ID,
		actorID,
		role,
		targetUserID,
		strings.TrimSpace(req.TargetName),
		strings.TrimSpace(req.TargetAvatar),
		req.DurationMinutes,
		s.now(),
	)
	if err != nil {
		return nil, err
	}
	return &MuteResp{
		RoomID:          mute.RoomID,
		TargetUserID:    mute.TargetUserID,
		TargetName:      mute.TargetName,
		DurationMinutes: mute.DurationMinutes,
		ExpiresAt:       mute.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *ModerationService) MuteState(ctx context.Context, roomID, actorID, targetUserID string) (*MuteStateResp, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if _, err := s.moderationActorRole(ctx, room, actorID); err != nil {
		return nil, err
	}
	mute, err := s.moderation.CurrentMute(ctx, room.ID, targetUserID, s.now())
	if err != nil {
		return nil, err
	}
	return muteStateResp(room.ID, targetUserID, mute, s.now()), nil
}

func (s *ModerationService) Unmute(ctx context.Context, roomID, actorID, targetUserID string) (*MuteStateResp, error) {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil, errcode.New(400, "target user is required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, errcode.New(404, "room not found")
		}
		return nil, err
	}
	if _, err := s.moderationActorRole(ctx, room, actorID); err != nil {
		return nil, err
	}
	if actorID == targetUserID {
		return nil, errcode.New(409, "cannot unmute yourself")
	}
	if _, err := s.moderation.UnmuteUser(ctx, room.OwnerID, room.ID, actorID, targetUserID, "", "", s.now()); err != nil {
		return nil, err
	}
	return &MuteStateResp{
		RoomID:       room.ID,
		TargetUserID: targetUserID,
		Muted:        false,
	}, nil
}

func (s *ModerationService) syncOwnerActiveRooms(ctx context.Context, ownerID string) error {
	rooms, err := s.rooms.ActiveRoomsByOwner(ctx, ownerID)
	if err != nil {
		return err
	}
	return s.moderation.SyncActiveRooms(ctx, rooms)
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

func muteStateResp(roomID, targetUserID string, mute *model.RoomMute, now time.Time) *MuteStateResp {
	resp := &MuteStateResp{
		RoomID:       roomID,
		TargetUserID: targetUserID,
		Muted:        mute != nil,
	}
	if mute != nil {
		resp.TargetName = mute.TargetName
		resp.TargetAvatar = mute.TargetAvatar
		resp.MuteExpiresAt = mute.ExpiresAt.UTC().Format(time.RFC3339)
		resp.MuteRemainingSeconds = int64(mute.ExpiresAt.Sub(now).Seconds())
		if resp.MuteRemainingSeconds < 0 {
			resp.MuteRemainingSeconds = 0
		}
	}
	return resp
}

func excludeUserID(ids []string, userID string) []string {
	if userID == "" || len(ids) == 0 {
		return ids
	}
	out := ids[:0]
	for _, id := range ids {
		if id != userID {
			out = append(out, id)
		}
	}
	return out
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
