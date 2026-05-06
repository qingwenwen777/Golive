package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	maxDirectMessageRunes = 1000
)

type MessageService struct {
	messages *repo.MessageRepo
	rooms    *repo.RoomRepo
	social   *repo.SocialRepo
	now      func() time.Time
}

func NewMessageService(messages *repo.MessageRepo, rooms *repo.RoomRepo, social *repo.SocialRepo) *MessageService {
	return &MessageService{
		messages: messages,
		rooms:    rooms,
		social:   social,
		now:      time.Now,
	}
}

type MessageUserDTO struct {
	ID                   string `json:"id"`
	Username             string `json:"username,omitempty"`
	DisplayName          string `json:"displayName,omitempty"`
	Name                 string `json:"name"`
	Avatar               string `json:"avatar,omitempty"`
	Verified             bool   `json:"verified"`
	LivePermissionStatus string `json:"livePermissionStatus,omitempty"`
}

type DirectThreadDTO struct {
	ID                 string         `json:"id"`
	ViewerID           string         `json:"viewerId"`
	CreatorID          string         `json:"creatorId"`
	ChannelID          string         `json:"channelId"`
	Peer               MessageUserDTO `json:"peer"`
	LastMessagePreview string         `json:"lastMessagePreview,omitempty"`
	LastSenderID       string         `json:"lastSenderId,omitempty"`
	LastMessageAt      string         `json:"lastMessageAt,omitempty"`
	Unread             int64          `json:"unread"`
	Pinned             bool           `json:"pinned"`
	Muted              bool           `json:"muted"`
	PushDisabled       bool           `json:"pushDisabled"`
	CanSend            bool           `json:"canSend"`
	AwaitingReply      bool           `json:"awaitingReply"`
	Blocked            bool           `json:"blocked"`
}

type DirectThreadListResp struct {
	Items []DirectThreadDTO `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

type DirectMessageDTO struct {
	ID         string `json:"id"`
	ThreadID   string `json:"threadId"`
	SenderID   string `json:"senderId"`
	ReceiverID string `json:"receiverId"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

type DirectMessageListResp struct {
	Items []DirectMessageDTO `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
}

type SendDirectReq struct {
	CreatorID string `json:"creatorId"`
	ChannelID string `json:"channelId"`
	Content   string `json:"content"`
}

type UpdateThreadOptionsReq struct {
	Pinned       *bool `json:"pinned"`
	Muted        *bool `json:"muted"`
	PushDisabled *bool `json:"pushDisabled"`
}

type BlockUserReq struct {
	Reason string `json:"reason"`
}

type BlockedUserDTO struct {
	User      MessageUserDTO `json:"user"`
	Role      string         `json:"role"`
	Reason    string         `json:"reason,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

type BlockedUserListResp struct {
	Items []BlockedUserDTO `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

type MessagePreferenceDTO struct {
	MessageReminderEnabled bool   `json:"messageReminderEnabled"`
	ReplyReminderScope     string `json:"replyReminderScope"`
	MentionReminderScope   string `json:"mentionReminderScope"`
	LikeReminderEnabled    bool   `json:"likeReminderEnabled"`
	FoldUnfollowedMessages bool   `json:"foldUnfollowedMessages"`
}

type FanGroupDTO struct {
	ID          string              `json:"id"`
	CreatorID   string              `json:"creatorId"`
	GroupNo     int                 `json:"groupNo"`
	Name        string              `json:"name"`
	MemberCount int                 `json:"memberCount"`
	Members     []FanGroupMemberDTO `json:"members"`
	CreatedAt   string              `json:"createdAt"`
	UpdatedAt   string              `json:"updatedAt"`
}

type FanGroupMemberDTO struct {
	User       MessageUserDTO `json:"user"`
	Role       string         `json:"role"`
	Muted      bool           `json:"muted"`
	MutedUntil string         `json:"mutedUntil,omitempty"`
	CreatedAt  string         `json:"createdAt"`
}

type FanGroupListResp struct {
	Items []FanGroupDTO `json:"items"`
	Total int           `json:"total"`
}

type UpdateFanGroupMemberReq struct {
	Role        string `json:"role"`
	MuteMinutes *int   `json:"muteMinutes"`
	Kick        bool   `json:"kick"`
}

func (s *MessageService) ListDirectThreads(ctx context.Context, userID string, page, size int) (*DirectThreadListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	page, size = normalizeListPage(page, size)
	rows, total, err := s.messages.ListDirectThreads(ctx, userID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]DirectThreadDTO, 0, len(rows))
	for _, row := range rows {
		blocked, err := s.messages.BlocksEitherWay(ctx, userID, peerID(row, userID))
		if err != nil {
			return nil, err
		}
		if blocked {
			continue
		}
		dto, err := s.directThreadDTO(ctx, row, userID)
		if err != nil {
			return nil, err
		}
		items = append(items, dto)
	}
	return &DirectThreadListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *MessageService) DirectThread(ctx context.Context, userID, threadID string) (*DirectThreadDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	thread, err := s.messages.DirectThreadForUser(ctx, threadID, userID)
	if err != nil {
		return nil, directThreadError(err)
	}
	dto, err := s.directThreadDTO(ctx, *thread, userID)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *MessageService) DirectDraft(ctx context.Context, userID, creatorID string) (*DirectThreadDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" {
		return nil, errcode.New(http.StatusBadRequest, "creator is required").WithReason("creator_required")
	}
	if existing, err := s.messages.ThreadByViewerCreator(ctx, userID, creatorID); err == nil {
		dto, err := s.directThreadDTO(ctx, *existing, userID)
		if err != nil {
			return nil, err
		}
		return &dto, nil
	} else if !errors.Is(err, repo.ErrDirectThreadNotFound) {
		return nil, err
	}
	if err := s.canViewerStartDirect(ctx, userID, creatorID); err != nil {
		return nil, err
	}
	peer, err := s.messageUser(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	return &DirectThreadDTO{
		ID:        "",
		ViewerID:  userID,
		CreatorID: creatorID,
		ChannelID: channelIDForOwner(creatorID),
		Peer:      peer,
		CanSend:   true,
	}, nil
}

func (s *MessageService) SendDirect(ctx context.Context, senderID string, req SendDirectReq) (*DirectThreadDTO, error) {
	if senderID == "" {
		return nil, errcode.ErrUnauthorized
	}
	creatorID, err := s.resolveCreatorID(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.canViewerStartDirect(ctx, senderID, creatorID); err != nil {
		return nil, err
	}
	body, err := cleanDirectMessage(req.Content)
	if err != nil {
		return nil, err
	}
	thread, _, err := s.messages.SendDirectMessage(ctx, repo.DirectSendInput{
		ViewerID:   senderID,
		CreatorID:  creatorID,
		ChannelID:  channelIDForOwner(creatorID),
		SenderID:   senderID,
		ReceiverID: creatorID,
		Body:       body,
		Now:        s.now(),
	})
	if err != nil {
		return nil, directSendError(err)
	}
	_ = s.notifyDirectMessage(ctx, *thread, senderID, creatorID, body)
	dto, err := s.directThreadDTO(ctx, *thread, senderID)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *MessageService) SendThreadMessage(ctx context.Context, senderID, threadID, content string) (*DirectThreadDTO, error) {
	if senderID == "" {
		return nil, errcode.ErrUnauthorized
	}
	thread, err := s.messages.DirectThreadForUser(ctx, threadID, senderID)
	if err != nil {
		return nil, directThreadError(err)
	}
	if senderID == thread.ViewerID {
		if err := s.canViewerStartDirect(ctx, senderID, thread.CreatorID); err != nil {
			return nil, err
		}
	} else if senderID != thread.CreatorID {
		return nil, errcode.New(http.StatusForbidden, "not a thread member").WithReason("not_thread_member")
	}
	blocked, err := s.messages.BlocksEitherWay(ctx, thread.ViewerID, thread.CreatorID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, errcode.New(http.StatusForbidden, "message target is blocked").WithReason("user_blocked")
	}
	body, err := cleanDirectMessage(content)
	if err != nil {
		return nil, err
	}
	receiverID := thread.CreatorID
	if senderID == thread.CreatorID {
		receiverID = thread.ViewerID
	}
	updated, _, err := s.messages.SendDirectMessage(ctx, repo.DirectSendInput{
		ViewerID:   thread.ViewerID,
		CreatorID:  thread.CreatorID,
		ChannelID:  thread.ChannelID,
		SenderID:   senderID,
		ReceiverID: receiverID,
		Body:       body,
		Now:        s.now(),
	})
	if err != nil {
		return nil, directSendError(err)
	}
	_ = s.notifyDirectMessage(ctx, *updated, senderID, receiverID, body)
	dto, err := s.directThreadDTO(ctx, *updated, senderID)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *MessageService) DirectMessages(ctx context.Context, userID, threadID string, page, size int) (*DirectMessageListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	page, size = normalizeListPage(page, size)
	thread, err := s.messages.DirectThreadForUser(ctx, threadID, userID)
	if err != nil {
		return nil, directThreadError(err)
	}
	blocked, err := s.messages.BlocksEitherWay(ctx, thread.ViewerID, thread.CreatorID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, errcode.New(http.StatusForbidden, "message target is blocked").WithReason("user_blocked")
	}
	items, total, err := s.messages.DirectMessages(ctx, threadID, userID, page, size)
	if err != nil {
		return nil, directThreadError(err)
	}
	_ = s.messages.MarkDirectThreadRead(ctx, threadID, userID)
	out := make([]DirectMessageDTO, 0, len(items))
	for _, item := range items {
		out = append(out, DirectMessageDTO{
			ID:         item.ID,
			ThreadID:   item.ThreadID,
			SenderID:   item.SenderID,
			ReceiverID: item.ReceiverID,
			Body:       item.Body,
			CreatedAt:  item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &DirectMessageListResp{Items: out, Total: total, Page: page, Size: size}, nil
}

func (s *MessageService) UpdateThreadOptions(ctx context.Context, userID, threadID string, req UpdateThreadOptionsReq) (*DirectThreadDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	thread, err := s.messages.UpdateThreadOptions(ctx, threadID, userID, repo.ThreadOptionUpdate{
		Pinned:       req.Pinned,
		Muted:        req.Muted,
		PushDisabled: req.PushDisabled,
	}, s.now())
	if err != nil {
		return nil, directThreadError(err)
	}
	dto, err := s.directThreadDTO(ctx, *thread, userID)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *MessageService) BlockUser(ctx context.Context, userID, targetID string, req BlockUserReq) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	targetID = strings.TrimSpace(targetID)
	if targetID == "" || targetID == userID {
		return errcode.New(http.StatusBadRequest, "invalid blocked user").WithReason("invalid_block_target")
	}
	role := "user"
	if approved, err := s.messages.IsApprovedCreator(ctx, targetID); err == nil && approved {
		role = "creator"
	}
	return s.messages.UpsertBlock(ctx, userID, targetID, role, req.Reason, s.now())
}

func (s *MessageService) UnblockUser(ctx context.Context, userID, targetID string) error {
	if userID == "" {
		return errcode.ErrUnauthorized
	}
	return s.messages.DeleteBlock(ctx, userID, targetID)
}

func (s *MessageService) ListBlocks(ctx context.Context, userID string, page, size int) (*BlockedUserListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	page, size = normalizeListPage(page, size)
	rows, total, err := s.messages.ListBlocks(ctx, userID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]BlockedUserDTO, 0, len(rows))
	for _, row := range rows {
		user := fallbackMessageUser(row.TargetUserID)
		if profile, err := s.messageUser(ctx, row.TargetUserID); err == nil {
			user = profile
		}
		items = append(items, BlockedUserDTO{
			User:      user,
			Role:      row.TargetRole,
			Reason:    row.Reason,
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &BlockedUserListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *MessageService) Preference(ctx context.Context, userID string) (*MessagePreferenceDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	pref, err := s.messages.Preference(ctx, userID)
	if err != nil {
		return nil, err
	}
	dto := preferenceDTO(*pref)
	return &dto, nil
}

func (s *MessageService) UpdatePreference(ctx context.Context, userID string, req MessagePreferenceDTO) (*MessagePreferenceDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	now := s.now()
	pref := &model.MessagePreference{
		UserID:                 userID,
		MessageReminderEnabled: req.MessageReminderEnabled,
		ReplyReminderScope:     normalizeMessageScope(req.ReplyReminderScope),
		MentionReminderScope:   normalizeMessageScope(req.MentionReminderScope),
		LikeReminderEnabled:    req.LikeReminderEnabled,
		FoldUnfollowedMessages: req.FoldUnfollowedMessages,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := s.messages.UpsertPreference(ctx, pref); err != nil {
		return nil, err
	}
	dto := preferenceDTO(*pref)
	return &dto, nil
}

func (s *MessageService) SyncFanGroups(ctx context.Context, creatorID string) (*FanGroupListResp, error) {
	if creatorID == "" {
		return nil, errcode.ErrUnauthorized
	}
	profile, _ := s.messages.UserProfile(ctx, creatorID)
	if err := s.requireCreator(ctx, creatorID); err != nil {
		return nil, err
	}
	if _, err := s.messages.SyncFanGroups(ctx, creatorID, profile.Name, s.now()); err != nil {
		return nil, err
	}
	return s.ListFanGroups(ctx, creatorID)
}

func (s *MessageService) ListFanGroups(ctx context.Context, creatorID string) (*FanGroupListResp, error) {
	if creatorID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if err := s.requireCreator(ctx, creatorID); err != nil {
		return nil, err
	}
	rows, err := s.messages.ListFanGroups(ctx, creatorID)
	if err != nil {
		return nil, err
	}
	items := make([]FanGroupDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, fanGroupDTO(row))
	}
	return &FanGroupListResp{Items: items, Total: len(items)}, nil
}

func (s *MessageService) UpdateFanGroupMember(ctx context.Context, creatorID, groupID, userID string, req UpdateFanGroupMemberReq) (*FanGroupListResp, error) {
	if creatorID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if err := s.requireCreator(ctx, creatorID); err != nil {
		return nil, err
	}
	role := ""
	if req.Role != "" {
		role = normalizeFanGroupRole(req.Role)
		if role == model.FanGroupRoleOwner {
			return nil, errcode.New(http.StatusBadRequest, "owner role cannot be assigned").WithReason("invalid_group_role")
		}
	}
	var mutedUntil *time.Time
	clearMute := false
	if req.MuteMinutes != nil {
		if *req.MuteMinutes > 0 {
			value := s.now().Add(time.Duration(*req.MuteMinutes) * time.Minute)
			mutedUntil = &value
		} else {
			clearMute = true
		}
	}
	if err := s.messages.UpdateFanGroupMember(ctx, creatorID, groupID, userID, role, mutedUntil, clearMute, req.Kick, s.now()); err != nil {
		if errors.Is(err, repo.ErrFanGroupNotFound) || errors.Is(err, repo.ErrFanGroupMemberNotFound) {
			return nil, errcode.New(http.StatusNotFound, "fan group member not found").WithReason("fan_group_member_not_found")
		}
		return nil, err
	}
	return s.ListFanGroups(ctx, creatorID)
}

func (s *MessageService) CreatorBlocks(ctx context.Context, creatorID, viewerID string) (bool, error) {
	return s.messages.BlockExists(ctx, creatorID, viewerID)
}

func (s *MessageService) BlocksInteraction(ctx context.Context, viewerID, creatorID string) (bool, error) {
	return s.messages.BlocksEitherWay(ctx, viewerID, creatorID)
}

func (s *MessageService) directThreadDTO(ctx context.Context, thread model.DirectThread, userID string) (DirectThreadDTO, error) {
	peerID := peerID(thread, userID)
	peer, err := s.messageUser(ctx, peerID)
	if err != nil {
		peer = fallbackMessageUser(peerID)
	}
	blocked, err := s.messages.BlocksEitherWay(ctx, userID, peerID)
	if err != nil {
		return DirectThreadDTO{}, err
	}
	dto := DirectThreadDTO{
		ID:                 thread.ID,
		ViewerID:           thread.ViewerID,
		CreatorID:          thread.CreatorID,
		ChannelID:          thread.ChannelID,
		Peer:               peer,
		LastMessagePreview: thread.LastMessagePreview,
		LastSenderID:       thread.LastSenderID,
		CanSend:            !blocked,
		Blocked:            blocked,
	}
	if thread.LastMessageAt != nil {
		dto.LastMessageAt = thread.LastMessageAt.UTC().Format(time.RFC3339)
	}
	if userID == thread.ViewerID {
		dto.Unread = thread.ViewerUnread
		dto.Pinned = thread.ViewerPinnedAt != nil
		dto.Muted = thread.ViewerMuted
		dto.PushDisabled = thread.ViewerPushDisabled
		dto.AwaitingReply = thread.LastSenderID == thread.ViewerID
		dto.CanSend = dto.CanSend && !dto.AwaitingReply
	} else {
		dto.Unread = thread.CreatorUnread
		dto.Pinned = thread.CreatorPinnedAt != nil
		dto.Muted = thread.CreatorMuted
		dto.PushDisabled = thread.CreatorPushDisabled
	}
	return dto, nil
}

func (s *MessageService) resolveCreatorID(ctx context.Context, req SendDirectReq) (string, error) {
	if creatorID := strings.TrimSpace(req.CreatorID); creatorID != "" {
		return creatorID, nil
	}
	if channelID := strings.TrimSpace(req.ChannelID); channelID != "" && s.rooms != nil {
		ownerID, err := s.rooms.ResolveOwnerID(ctx, channelID)
		if err != nil {
			if errors.Is(err, repo.ErrRoomNotFound) {
				return "", errcode.New(http.StatusNotFound, "creator not found").WithReason("creator_not_found")
			}
			return "", err
		}
		return ownerID, nil
	}
	return "", errcode.New(http.StatusBadRequest, "creator is required").WithReason("creator_required")
}

func (s *MessageService) canViewerStartDirect(ctx context.Context, viewerID, creatorID string) error {
	if viewerID == "" {
		return errcode.ErrUnauthorized
	}
	if creatorID == "" || creatorID == viewerID {
		return errcode.New(http.StatusBadRequest, "invalid creator").WithReason("invalid_creator")
	}
	approved, err := s.messages.IsApprovedCreator(ctx, creatorID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return errcode.New(http.StatusNotFound, "creator not found").WithReason("creator_not_found")
		}
		return err
	}
	if !approved {
		return errcode.New(http.StatusForbidden, "only approved creators can receive direct messages").WithReason("creator_required")
	}
	blocked, err := s.messages.BlocksEitherWay(ctx, viewerID, creatorID)
	if err != nil {
		return err
	}
	if blocked {
		return errcode.New(http.StatusForbidden, "message target is blocked").WithReason("user_blocked")
	}
	if s.social == nil {
		return nil
	}
	following, err := s.social.IsFollowing(ctx, viewerID, channelIDForOwner(creatorID))
	if err != nil {
		return err
	}
	if !following {
		return errcode.New(http.StatusForbidden, "follow the creator before sending a message").WithReason("follow_required")
	}
	return nil
}

func (s *MessageService) requireCreator(ctx context.Context, userID string) error {
	approved, err := s.messages.IsApprovedCreator(ctx, userID)
	if err != nil {
		return err
	}
	if !approved {
		return errcode.New(http.StatusForbidden, "creator permission is not approved").WithReason("creator_permission_required")
	}
	return nil
}

func (s *MessageService) messageUser(ctx context.Context, userID string) (MessageUserDTO, error) {
	profile, err := s.messages.UserProfile(ctx, userID)
	if err != nil {
		return MessageUserDTO{}, err
	}
	return MessageUserDTO{
		ID:                   profile.ID,
		Username:             profile.Username,
		DisplayName:          profile.DisplayName,
		Name:                 nonEmpty(profile.Name, profile.Username, profile.ID),
		Avatar:               profile.Avatar,
		Verified:             profile.Verified,
		LivePermissionStatus: profile.LivePermissionStatus,
	}, nil
}

func (s *MessageService) notifyDirectMessage(ctx context.Context, thread model.DirectThread, senderID, receiverID, body string) error {
	if s.messages == nil || receiverID == "" {
		return nil
	}
	sender, _ := s.messages.UserProfile(ctx, senderID)
	return s.messages.CreateNotification(ctx, model.Notification{
		ID:            "dm-" + notificationHash(thread.ID, senderID, receiverID, time.Now().Format(time.RFC3339Nano)),
		UserID:        receiverID,
		Type:          "direct_message",
		Title:         "你收到一条新的私信",
		Body:          trimRunes(body, 120),
		Link:          "/messages/private",
		ActorID:       senderID,
		ActorUsername: sender.Username,
		ActorName:     nonEmpty(sender.Name, sender.Username, senderID),
		ActorAvatar:   sender.Avatar,
		ActorVerified: sender.Verified,
		CreatedAt:     s.now(),
	})
}

func fanGroupDTO(row repo.FanGroupWithMembers) FanGroupDTO {
	members := make([]FanGroupMemberDTO, 0, len(row.Members))
	now := time.Now()
	for _, member := range row.Members {
		muted := member.MutedUntil != nil && member.MutedUntil.After(now)
		dto := FanGroupMemberDTO{
			User: MessageUserDTO{
				ID:          member.UserID,
				Username:    member.Username,
				DisplayName: member.DisplayName,
				Name:        nonEmpty(member.Name, member.Username, member.UserID),
				Avatar:      member.Avatar,
				Verified:    member.Verified,
			},
			Role:      member.Role,
			Muted:     muted,
			CreatedAt: member.CreatedAt.UTC().Format(time.RFC3339),
		}
		if muted {
			dto.MutedUntil = member.MutedUntil.UTC().Format(time.RFC3339)
		}
		members = append(members, dto)
	}
	return FanGroupDTO{
		ID:          row.Group.ID,
		CreatorID:   row.Group.CreatorID,
		GroupNo:     row.Group.GroupNo,
		Name:        row.Group.Name,
		MemberCount: row.Group.MemberCount,
		Members:     members,
		CreatedAt:   row.Group.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   row.Group.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func cleanDirectMessage(value string) (string, error) {
	text := strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if text == "" {
		return "", errcode.New(http.StatusBadRequest, "message content is required").WithReason("content_required")
	}
	runes := []rune(text)
	if len(runes) > maxDirectMessageRunes {
		text = string(runes[:maxDirectMessageRunes])
	}
	return text, nil
}

func preferenceDTO(pref model.MessagePreference) MessagePreferenceDTO {
	return MessagePreferenceDTO{
		MessageReminderEnabled: pref.MessageReminderEnabled,
		ReplyReminderScope:     normalizeMessageScope(pref.ReplyReminderScope),
		MentionReminderScope:   normalizeMessageScope(pref.MentionReminderScope),
		LikeReminderEnabled:    pref.LikeReminderEnabled,
		FoldUnfollowedMessages: pref.FoldUnfollowedMessages,
	}
}

func normalizeMessageScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.MessagePreferenceScopeFollowing:
		return model.MessagePreferenceScopeFollowing
	case model.MessagePreferenceScopeNone:
		return model.MessagePreferenceScopeNone
	default:
		return model.MessagePreferenceScopeAll
	}
}

func normalizeFanGroupRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.FanGroupRoleAdmin:
		return model.FanGroupRoleAdmin
	default:
		return model.FanGroupRoleMember
	}
}

func peerID(thread model.DirectThread, userID string) string {
	if userID == thread.ViewerID {
		return thread.CreatorID
	}
	return thread.ViewerID
}

func fallbackMessageUser(userID string) MessageUserDTO {
	name := userID
	if len(name) > 8 {
		name = "User " + name[:8]
	}
	if name == "" {
		name = "User"
	}
	return MessageUserDTO{ID: userID, Name: name}
}

func directThreadError(err error) error {
	if errors.Is(err, repo.ErrDirectThreadNotFound) {
		return errcode.New(http.StatusNotFound, "direct thread not found").WithReason("direct_thread_not_found")
	}
	return err
}

func directSendError(err error) error {
	if errors.Is(err, repo.ErrAwaitingCreatorReply) {
		return errcode.New(http.StatusConflict, "wait for the creator to reply before sending another message").WithReason("awaiting_creator_reply")
	}
	return directThreadError(err)
}

func notificationHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])[:32]
}
