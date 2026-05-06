package service

import (
	"context"
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
	ID                   string       `json:"id"`
	Username             string       `json:"username,omitempty"`
	DisplayName          string       `json:"displayName,omitempty"`
	Name                 string       `json:"name"`
	Avatar               string       `json:"avatar,omitempty"`
	Verified             bool         `json:"verified"`
	LivePermissionStatus string       `json:"livePermissionStatus,omitempty"`
	FanBadge             *FanBadgeDTO `json:"fanBadge,omitempty"`
}

type FanBadgeDTO struct {
	CreatorID string `json:"creatorId"`
	Level     int    `json:"level"`
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
	ID         string         `json:"id"`
	ThreadID   string         `json:"threadId"`
	SenderID   string         `json:"senderId"`
	ReceiverID string         `json:"receiverId"`
	Sender     MessageUserDTO `json:"sender"`
	Body       string         `json:"body"`
	CreatedAt  string         `json:"createdAt"`
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
	Unread      int64               `json:"unread"`
	Members     []FanGroupMemberDTO `json:"members"`
	CreatedAt   string              `json:"createdAt"`
	UpdatedAt   string              `json:"updatedAt"`
}

type FanGroupMemberDTO struct {
	User              MessageUserDTO `json:"user"`
	FanBadge          *FanBadgeDTO   `json:"fanBadge,omitempty"`
	Role              string         `json:"role"`
	Muted             bool           `json:"muted"`
	MutedUntil        string         `json:"mutedUntil,omitempty"`
	Kicked            bool           `json:"kicked"`
	KickedAt          string         `json:"kickedAt,omitempty"`
	KickReason        string         `json:"kickReason,omitempty"`
	RejoinRequestedAt string         `json:"rejoinRequestedAt,omitempty"`
	RejoinRejectedAt  string         `json:"rejoinRejectedAt,omitempty"`
	PendingRejoin     bool           `json:"pendingRejoin"`
	CreatedAt         string         `json:"createdAt"`
}

type FanGroupListResp struct {
	Items []FanGroupDTO `json:"items"`
	Total int           `json:"total"`
}

type FanGroupMessageDTO struct {
	ID        string         `json:"id"`
	GroupID   string         `json:"groupId"`
	Sender    MessageUserDTO `json:"sender"`
	Role      string         `json:"role"`
	FanBadge  *FanBadgeDTO   `json:"fanBadge,omitempty"`
	Body      string         `json:"body"`
	CreatedAt string         `json:"createdAt"`
}

type FanGroupMessageListResp struct {
	Items []FanGroupMessageDTO `json:"items"`
	Total int64                `json:"total"`
	Page  int                  `json:"page"`
	Size  int                  `json:"size"`
}

type UpdateFanGroupMemberReq struct {
	Role          string `json:"role"`
	MuteMinutes   *int   `json:"muteMinutes"`
	Kick          bool   `json:"kick"`
	ApproveRejoin bool   `json:"approveRejoin"`
	RejectRejoin  bool   `json:"rejectRejoin"`
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
	senders := map[string]MessageUserDTO{}
	for _, item := range items {
		sender, ok := senders[item.SenderID]
		if !ok {
			sender, err = s.messageUser(ctx, item.SenderID)
			if err != nil {
				sender = fallbackMessageUser(item.SenderID)
			}
			if badge := s.fanBadgeDTO(ctx, item.SenderID, thread.CreatorID); badge != nil {
				sender.FanBadge = badge
			}
			senders[item.SenderID] = sender
		}
		out = append(out, DirectMessageDTO{
			ID:         item.ID,
			ThreadID:   item.ThreadID,
			SenderID:   item.SenderID,
			ReceiverID: item.ReceiverID,
			Sender:     sender,
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

func (s *MessageService) ListJoinedFanGroups(ctx context.Context, userID string) (*FanGroupListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	rows, err := s.messages.ListJoinedFanGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		groupIDs = append(groupIDs, row.Group.ID)
	}
	unreadCounts, err := s.messages.FanGroupUnreadCounts(ctx, userID, groupIDs)
	if err != nil {
		return nil, err
	}
	items := make([]FanGroupDTO, 0, len(rows))
	for _, row := range rows {
		item := fanGroupDTO(row)
		item.Unread = unreadCounts[row.Group.ID]
		items = append(items, item)
	}
	return &FanGroupListResp{Items: items, Total: len(items)}, nil
}

func (s *MessageService) FanGroupMessages(ctx context.Context, userID, groupID string, page, size int) (*FanGroupMessageListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	page, size = normalizeListPage(page, size)
	rows, total, err := s.messages.FanGroupMessages(ctx, groupID, userID, page, size)
	if err != nil {
		return nil, fanGroupError(err)
	}
	_ = s.messages.MarkFanGroupRead(ctx, groupID, userID, s.now())
	items := make([]FanGroupMessageDTO, 0, len(rows))
	for _, row := range rows {
		fanBadge := fanBadgeFromLevel(row.CreatorID, row.FanBadgeLevel)
		items = append(items, FanGroupMessageDTO{
			ID:      row.ID,
			GroupID: row.GroupID,
			Sender: MessageUserDTO{
				ID:          row.SenderID,
				Username:    row.Username,
				DisplayName: row.DisplayName,
				Name:        nonEmpty(row.Name, row.Username, row.SenderID),
				Avatar:      row.Avatar,
				Verified:    row.Verified,
				FanBadge:    fanBadge,
			},
			Role:      nonEmpty(row.Role, model.FanGroupRoleMember),
			FanBadge:  fanBadge,
			Body:      row.Body,
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &FanGroupMessageListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *MessageService) SendFanGroupMessage(ctx context.Context, userID, groupID, content string) (*FanGroupMessageDTO, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	body, err := cleanDirectMessage(content)
	if err != nil {
		return nil, err
	}
	msg, err := s.messages.SendFanGroupMessage(ctx, groupID, userID, body, s.now())
	if err != nil {
		return nil, fanGroupError(err)
	}
	sender, err := s.messageUser(ctx, userID)
	if err != nil {
		sender = fallbackMessageUser(userID)
	}
	member, _ := s.messages.FanGroupMemberForUser(ctx, groupID, userID)
	if group, groupErr := s.messages.FanGroupByID(ctx, groupID); groupErr == nil {
		if badge := s.fanBadgeDTO(ctx, userID, group.CreatorID); badge != nil {
			sender.FanBadge = badge
		}
	}
	role := model.FanGroupRoleMember
	if member != nil && member.Role != "" {
		role = member.Role
	}
	return &FanGroupMessageDTO{
		ID:        msg.ID,
		GroupID:   msg.GroupID,
		Sender:    sender,
		Role:      role,
		FanBadge:  sender.FanBadge,
		Body:      msg.Body,
		CreatedAt: msg.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *MessageService) UpdateFanGroupMember(ctx context.Context, actorID, groupID, userID string, req UpdateFanGroupMemberReq) (*FanGroupListResp, error) {
	if actorID == "" {
		return nil, errcode.ErrUnauthorized
	}
	group, err := s.messages.FanGroupByID(ctx, groupID)
	if err != nil {
		return nil, fanGroupError(err)
	}
	isOwner := actorID == group.CreatorID
	isAdmin := false
	if !isOwner {
		actor, err := s.messages.FanGroupMemberForUser(ctx, group.ID, actorID)
		if err == nil && actor.Role == model.FanGroupRoleAdmin {
			isAdmin = true
		}
	}
	if !isOwner && !isAdmin {
		return nil, errcode.New(http.StatusForbidden, "fan group moderator required").WithReason("fan_group_moderator_required")
	}
	if !isOwner && (req.Role != "" || req.Kick || req.ApproveRejoin || req.RejectRejoin) {
		return nil, errcode.New(http.StatusForbidden, "only the creator can manage fan group membership").WithReason("fan_group_owner_required")
	}
	if userID == actorID || userID == group.CreatorID {
		return nil, errcode.New(http.StatusBadRequest, "cannot manage this member").WithReason("invalid_group_member")
	}
	if isAdmin && req.MuteMinutes != nil {
		target, err := s.messages.FanGroupMemberForUser(ctx, group.ID, userID)
		if err != nil {
			return nil, fanGroupError(err)
		}
		if target.Role != model.FanGroupRoleMember {
			return nil, errcode.New(http.StatusForbidden, "admins can only mute regular members").WithReason("fan_group_owner_required")
		}
	}
	if isOwner {
		if err := s.requireCreator(ctx, actorID); err != nil {
			return nil, err
		}
	}
	if req.ApproveRejoin && req.RejectRejoin {
		return nil, errcode.New(http.StatusBadRequest, "choose approve or reject").WithReason("invalid_rejoin_action")
	}
	if req.ApproveRejoin || req.RejectRejoin {
		req.Kick = false
		req.MuteMinutes = nil
		req.Role = ""
	}
	if !isOwner && req.MuteMinutes == nil {
		return nil, errcode.New(http.StatusBadRequest, "no permitted member update").WithReason("invalid_group_member_update")
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
	if err := s.messages.UpdateFanGroupMember(ctx, group.CreatorID, group.ID, userID, role, mutedUntil, clearMute, req.Kick, req.ApproveRejoin, req.RejectRejoin, s.now()); err != nil {
		if errors.Is(err, repo.ErrFanGroupNotFound) || errors.Is(err, repo.ErrFanGroupMemberNotFound) || errors.Is(err, repo.ErrFanGroupRejoinNotFound) {
			return nil, errcode.New(http.StatusNotFound, "fan group member not found").WithReason("fan_group_member_not_found")
		}
		return nil, err
	}
	if actorID == group.CreatorID {
		return s.ListFanGroups(ctx, actorID)
	}
	return s.ListJoinedFanGroups(ctx, actorID)
}

func (s *MessageService) RequestFanGroupRejoin(ctx context.Context, userID, groupID string) (*FanGroupListResp, error) {
	if userID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if err := s.messages.RequestFanGroupRejoin(ctx, groupID, userID, s.now()); err != nil {
		return nil, fanGroupError(err)
	}
	return s.ListJoinedFanGroups(ctx, userID)
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
		if thread.LastSenderID == thread.ViewerID {
			creatorHasReplied, err := s.messages.DirectThreadHasMessageFrom(ctx, thread.ID, thread.CreatorID)
			if err != nil {
				return DirectThreadDTO{}, err
			}
			dto.AwaitingReply = !creatorHasReplied
		}
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

func (s *MessageService) fanBadgeDTO(ctx context.Context, userID, creatorID string) *FanBadgeDTO {
	if s.messages == nil || userID == "" || creatorID == "" {
		return nil
	}
	badge, err := s.messages.FanBadge(ctx, userID, creatorID)
	if err != nil || badge == nil {
		return nil
	}
	return fanBadgeFromLevel(creatorID, badge.Level)
}

func fanGroupDTO(row repo.FanGroupWithMembers) FanGroupDTO {
	members := make([]FanGroupMemberDTO, 0, len(row.Members))
	now := time.Now()
	for _, member := range row.Members {
		muted := member.MutedUntil != nil && member.MutedUntil.After(now)
		fanBadge := fanBadgeFromLevel(row.Group.CreatorID, member.FanBadgeLevel)
		dto := FanGroupMemberDTO{
			User: MessageUserDTO{
				ID:          member.UserID,
				Username:    member.Username,
				DisplayName: member.DisplayName,
				Name:        nonEmpty(member.Name, member.Username, member.UserID),
				Avatar:      member.Avatar,
				Verified:    member.Verified,
				FanBadge:    fanBadge,
			},
			FanBadge:      fanBadge,
			Role:          member.Role,
			Muted:         muted,
			Kicked:        member.KickedAt != nil,
			KickReason:    member.KickReason,
			PendingRejoin: member.KickedAt != nil && member.RejoinRequestedAt != nil,
			CreatedAt:     member.CreatedAt.UTC().Format(time.RFC3339),
		}
		if muted {
			dto.MutedUntil = member.MutedUntil.UTC().Format(time.RFC3339)
		}
		if member.KickedAt != nil {
			dto.KickedAt = member.KickedAt.UTC().Format(time.RFC3339)
		}
		if member.RejoinRequestedAt != nil {
			dto.RejoinRequestedAt = member.RejoinRequestedAt.UTC().Format(time.RFC3339)
		}
		if member.RejoinRejectedAt != nil {
			dto.RejoinRejectedAt = member.RejoinRejectedAt.UTC().Format(time.RFC3339)
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

func fanBadgeFromLevel(creatorID string, level int) *FanBadgeDTO {
	if creatorID == "" || level <= 0 {
		return nil
	}
	if level > 99 {
		level = 99
	}
	return &FanBadgeDTO{CreatorID: creatorID, Level: level}
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

func fanGroupError(err error) error {
	if errors.Is(err, repo.ErrFanGroupNotFound) {
		return errcode.New(http.StatusNotFound, "fan group not found").WithReason("fan_group_not_found")
	}
	if errors.Is(err, repo.ErrFanGroupMemberNotFound) {
		return errcode.New(http.StatusForbidden, "join the fan group before chatting").WithReason("fan_group_member_required")
	}
	if errors.Is(err, repo.ErrFanGroupMuted) {
		return errcode.New(http.StatusForbidden, "you are muted in this fan group").WithReason("fan_group_muted")
	}
	if errors.Is(err, repo.ErrFanGroupRejoinDenied) {
		return errcode.New(http.StatusForbidden, "you need an active fan badge before requesting to rejoin").WithReason("fan_group_rejoin_denied")
	}
	if errors.Is(err, repo.ErrFanGroupRejoinNotFound) {
		return errcode.New(http.StatusNotFound, "fan group rejoin request not found").WithReason("fan_group_rejoin_not_found")
	}
	return err
}
