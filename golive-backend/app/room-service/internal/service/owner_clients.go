package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/qingwenwen777/golive/pkg/internalauth"
)

// Report moderation acts on data other services own: user restrictions
// (user-service), super chats (gift-service) and chat messages
// (chat-service). room-service calls their /internal APIs instead of
// writing those tables, and returns their errors so a moderation action
// fails visibly rather than being skipped.

const ownerServiceTimeout = 3 * time.Second

// Restriction actions understood by user-service.
const (
	RestrictionBan  = "ban"
	RestrictionMute = "mute"
)

// Reasons the owning services answer a 404 with when the item itself does
// not exist. Only these mean "nothing left to hide": a bare 404 also comes
// from a route the service lacks (an older build during a rolling deploy, a
// wrong service_url), and taking it as done would resolve the report while
// the content stays up.
const (
	superChatNotFoundReason   = "super_chat_not_found" // gift-service
	chatMessageNotFoundReason = "message_not_found"    // chat-service
)

// UserRestrictionUpdate is the body of user-service's
// POST /internal/users/:id/restriction.
type UserRestrictionUpdate struct {
	Action     string     `json:"action"`
	Reason     string     `json:"reason,omitempty"`
	MutedUntil *time.Time `json:"mutedUntil,omitempty"`
	OperatorID string     `json:"operatorId,omitempty"`
}

// UserRestrictionSetter sets site-wide bans and mutes (user-service).
type UserRestrictionSetter interface {
	SetUserRestriction(ctx context.Context, userID string, update UserRestrictionUpdate) error
}

// SuperChatModerator hides a super chat without changing its payment
// (gift-service).
type SuperChatModerator interface {
	ModerateSuperChat(ctx context.Context, orderID, operatorID string) error
}

// ChatMessageModerator hides a live chat message (chat-service).
type ChatMessageModerator interface {
	HideChatMessage(ctx context.Context, roomID, messageID string) error
}

// OwnerServices are the clients report moderation uses for data owned by
// other services.
type OwnerServices struct {
	Users      UserRestrictionSetter
	SuperChats SuperChatModerator
	Chat       ChatMessageModerator
}

// NewOwnerServices builds HTTP clients for the owning services' /internal
// APIs, authenticated with the shared internal token.
func NewOwnerServices(userURL, giftURL, chatURL, token string) OwnerServices {
	return OwnerServices{
		Users:      &UserServiceClient{c: internalauth.NewClient(userURL, token, ownerServiceTimeout)},
		SuperChats: &GiftServiceClient{c: internalauth.NewClient(giftURL, token, ownerServiceTimeout)},
		Chat:       &ChatServiceClient{c: internalauth.NewClient(chatURL, token, ownerServiceTimeout)},
	}
}

type UserServiceClient struct{ c *internalauth.Client }

func (u *UserServiceClient) SetUserRestriction(ctx context.Context, userID string, update UserRestrictionUpdate) error {
	path := "/internal/users/" + url.PathEscape(userID) + "/restriction"
	if err := u.c.Do(ctx, http.MethodPost, path, update, nil); err != nil {
		return fmt.Errorf("user-service %s user %s: %w", update.Action, userID, err)
	}
	return nil
}

type GiftServiceClient struct{ c *internalauth.Client }

// ModerateSuperChat treats gift-service's own "super chat not found" as
// done: there is nothing left to hide.
func (g *GiftServiceClient) ModerateSuperChat(ctx context.Context, orderID, operatorID string) error {
	path := "/internal/super-chats/" + url.PathEscape(orderID) + "/moderation"
	err := g.c.Do(ctx, http.MethodPost, path, map[string]string{"operatorId": operatorID}, nil)
	if err != nil && !internalauth.IsReason(err, http.StatusNotFound, superChatNotFoundReason) {
		return fmt.Errorf("gift-service moderate super chat %s: %w", orderID, err)
	}
	return nil
}

type ChatServiceClient struct{ c *internalauth.Client }

// HideChatMessage treats chat-service's own "message not found" as done:
// there is nothing left to hide.
func (ch *ChatServiceClient) HideChatMessage(ctx context.Context, roomID, messageID string) error {
	path := "/internal/rooms/" + url.PathEscape(roomID) + "/danmus/" + url.PathEscape(messageID)
	err := ch.c.Do(ctx, http.MethodDelete, path, nil, nil)
	if err != nil && !internalauth.IsReason(err, http.StatusNotFound, chatMessageNotFoundReason) {
		return fmt.Errorf("chat-service hide message %s in room %s: %w", messageID, roomID, err)
	}
	return nil
}
