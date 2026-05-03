package hub

import (
	"encoding/json"
	"time"
)

// Outbound (server → client) ----------------------------------------------

type SystemMsg struct {
	Type string `json:"type"` // "system"
	Text string `json:"text"`
	Ts   int64  `json:"ts"`
}

type ChatMsg struct {
	Type      string           `json:"type"` // "chat"
	ID        string           `json:"id"`
	UserID    string           `json:"userId,omitempty"`
	User      string           `json:"user"`
	Avatar    string           `json:"avatar,omitempty"`
	Text      string           `json:"text"`
	Color     string           `json:"color,omitempty"`
	Role      string           `json:"role,omitempty"`
	FanBadge  *FanBadgePayload `json:"fanBadge,omitempty"`
	UserLevel int              `json:"userLevel,omitempty"`
	Ts        int64            `json:"ts"`
}

type FanBadgePayload struct {
	CreatorID string `json:"creatorId"`
	Level     int    `json:"level"`
}

type ViewerProfile struct {
	UserID    string `json:"userId,omitempty"`
	User      string `json:"user"`
	Avatar    string `json:"avatar,omitempty"`
	UserLevel int    `json:"userLevel,omitempty"`
	IsOwner   bool   `json:"-"`
}

type ViewerListItem struct {
	UserID       string `json:"userId,omitempty"`
	User         string `json:"user"`
	Avatar       string `json:"avatar,omitempty"`
	Contribution int64  `json:"contribution"`
	UserLevel    int    `json:"userLevel,omitempty"`
}

type ViewerListMsg struct {
	Type    string           `json:"type"` // "viewer_list"
	Total   int              `json:"total"`
	Viewers []ViewerListItem `json:"viewers"`
	Ts      int64            `json:"ts"`
}

type SuperChatMsg struct {
	Type   string `json:"type"` // "super_chat"
	ID     string `json:"id"`
	UserID string `json:"userId,omitempty"`
	User   string `json:"user"`
	Avatar string `json:"avatar,omitempty"`
	Amount string `json:"amount"`
	Tier   int    `json:"tier"` // 0–5
	Text   string `json:"text"`
	Ts     int64  `json:"ts"`
}

type GiftMsg struct {
	Type      string `json:"type"` // "gift"
	ID        string `json:"id,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	User      string `json:"user"`
	Avatar    string `json:"avatar,omitempty"`
	GiftName  string `json:"giftName"`
	GiftIcon  string `json:"giftIcon,omitempty"`
	Count     int    `json:"count,omitempty"`
	Tier      int    `json:"tier,omitempty"`
	TotalCoin int64  `json:"totalCoin,omitempty"`
	Ts        int64  `json:"ts"`
}

// Inbound (client → server) -----------------------------------------------

type Inbound struct {
	Type          string           `json:"type"`
	UserID        string           `json:"userId,omitempty"`
	User          string           `json:"user,omitempty"`
	Avatar        string           `json:"avatar,omitempty"`
	Text          string           `json:"text,omitempty"`
	ClientID      string           `json:"clientId,omitempty"`
	LastMessageID string           `json:"lastMessageId,omitempty"`
	FanBadge      *FanBadgePayload `json:"fanBadge,omitempty"`
	UserLevel     int              `json:"userLevel,omitempty"`
}

// EncodeSystem builds a welcome / notice message.
func EncodeSystem(text string) []byte {
	b, _ := json.Marshal(SystemMsg{Type: "system", Text: text, Ts: time.Now().UnixMilli()})
	return b
}

func EncodeChat(id, userID, user, avatar, text string, ts int64, fanBadge *FanBadgePayload, role string, userLevel int) []byte {
	b, _ := json.Marshal(ChatMsg{
		Type:      "chat",
		ID:        id,
		UserID:    userID,
		User:      user,
		Avatar:    avatar,
		Text:      text,
		Role:      role,
		FanBadge:  fanBadge,
		UserLevel: userLevel,
		Ts:        ts,
	})
	return b
}

func EncodeViewerList(total int, viewers []ViewerListItem) []byte {
	b, _ := json.Marshal(ViewerListMsg{
		Type:    "viewer_list",
		Total:   total,
		Viewers: viewers,
		Ts:      time.Now().UnixMilli(),
	})
	return b
}

// EncodeViewerCount is reused by tests.
func EncodeViewerCount(n int64) []byte { return encodeViewerCount(n) }
