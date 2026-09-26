package model

import (
	"time"

	"gorm.io/gorm"
)

// Danmu is the persisted form. The table name is computed at runtime from
// shard-of(roomId), so we DO NOT set TableName() — callers always pass the
// resolved name to gorm.Table().
//
// History reads one room's newest messages (room_id = ? ORDER BY ts DESC),
// served by the (room_id, ts) index. It is declared with composite:room_ts
// rather than a fixed name so each shard gets its own idx_danmus_<n>_room_ts
// (index names are database-wide in SQLite).
type Danmu struct {
	ID                string `gorm:"primaryKey;type:varchar(64)"`
	RoomID            string `gorm:"type:varchar(64);index;index:,composite:room_ts,priority:1;not null"`
	UserID            string `gorm:"type:varchar(36);index"`
	Username          string `gorm:"type:varchar(64)"`
	Avatar            string `gorm:"type:varchar(500)"`
	Text              string `gorm:"type:varchar(500);not null"`
	Color             string `gorm:"type:varchar(16)"`
	Role              string `gorm:"type:varchar(16)"`
	FanBadgeCreatorID string `gorm:"type:varchar(80)"`
	FanBadgeLevel     int
	UserLevel         int
	Ts                int64 `gorm:"not null;index;index:,composite:room_ts,priority:2"` // ms epoch
	CreatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

// Public is the wire-shape served via GET /rooms/:id/danmus and Redis pub/sub.
// History can also include SuperChat fields.
type Public struct {
	Type      string           `json:"type"` // "chat" or "super_chat"
	ID        string           `json:"id"`
	UserID    string           `json:"userId,omitempty"`
	User      string           `json:"user"`
	Avatar    string           `json:"avatar,omitempty"`
	Text      string           `json:"text"`
	Color     string           `json:"color,omitempty"`
	Role      string           `json:"role,omitempty"`
	FanBadge  *FanBadgePayload `json:"fanBadge,omitempty"`
	UserLevel int              `json:"userLevel,omitempty"`
	Amount    string           `json:"amount,omitempty"`
	Tier      *int             `json:"tier,omitempty"`
	Ts        int64            `json:"ts"`
}

type FanBadgePayload struct {
	CreatorID string `json:"creatorId"`
	Level     int    `json:"level"`
}

func (d *Danmu) ToPublic() Public {
	out := Public{
		Type:      "chat",
		ID:        d.ID,
		UserID:    d.UserID,
		User:      d.Username,
		Avatar:    d.Avatar,
		Text:      d.Text,
		Color:     d.Color,
		Role:      d.Role,
		UserLevel: d.UserLevel,
		Ts:        d.Ts,
	}
	if d.FanBadgeCreatorID != "" && d.FanBadgeLevel > 0 {
		out.FanBadge = &FanBadgePayload{
			CreatorID: d.FanBadgeCreatorID,
			Level:     d.FanBadgeLevel,
		}
	}
	return out
}
