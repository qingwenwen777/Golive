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
	Type   string `json:"type"` // "chat"
	ID     string `json:"id"`
	User   string `json:"user"`
	Avatar string `json:"avatar,omitempty"`
	Text   string `json:"text"`
	Color  string `json:"color,omitempty"`
	Ts     int64  `json:"ts"`
}

type SuperChatMsg struct {
	Type   string `json:"type"` // "super_chat"
	ID     string `json:"id"`
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
	User      string `json:"user"`
	GiftName  string `json:"giftName"`
	GiftIcon  string `json:"giftIcon,omitempty"`
	Count     int    `json:"count,omitempty"`
	Tier      int    `json:"tier,omitempty"`
	Ts        int64  `json:"ts"`
}

// Inbound (client → server) -----------------------------------------------

type Inbound struct {
	Type          string `json:"type"`
	User          string `json:"user,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	Text          string `json:"text,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	LastMessageID string `json:"lastMessageId,omitempty"`
}

// EncodeSystem builds a welcome / notice message.
func EncodeSystem(text string) []byte {
	b, _ := json.Marshal(SystemMsg{Type: "system", Text: text, Ts: time.Now().UnixMilli()})
	return b
}

func EncodeChat(id, user, avatar, text string, ts int64) []byte {
	b, _ := json.Marshal(ChatMsg{
		Type:   "chat",
		ID:     id,
		User:   user,
		Avatar: avatar,
		Text:   text,
		Ts:     ts,
	})
	return b
}

// EncodeViewerCount is reused by tests.
func EncodeViewerCount(n int64) []byte { return encodeViewerCount(n) }
