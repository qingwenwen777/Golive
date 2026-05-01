package model

import (
	"fmt"
	"time"
)

// Status values for Room.Status.
const (
	StatusPublishing = "publishing"
	StatusLive       = "live"
	StatusEnding     = "ending"
	StatusEnded      = "ended"
)

// Room is the GORM model. Internal-only fields are kept off the JSON wire by
// converting to Stream (the DTO) before responding.
type Room struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	Title       string    `gorm:"type:varchar(255);not null"`
	TitleJa     string    `gorm:"type:varchar(255)"`
	Description string    `gorm:"type:text"`
	Channel     string    `gorm:"type:varchar(128);index"`
	ChannelID   string    `gorm:"type:varchar(64);index"`
	Verified    bool      `gorm:"not null;default:false"`
	Avatar      string    `gorm:"type:varchar(500)"`
	Cover       string    `gorm:"type:varchar(500)"`
	Viewers     int64     `gorm:"not null;default:0"`
	Category    string    `gorm:"type:varchar(64);index"`
	CategoryJa  string    `gorm:"type:varchar(64)"`
	StartedAt   time.Time `gorm:"index"`
	Status      string    `gorm:"type:varchar(16);not null;default:'live';index"`
	OwnerID     string    `gorm:"type:varchar(36);index"`
	StreamKey   string    `gorm:"type:varchar(128);index"`
	EndedAt     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Room) TableName() string { return "rooms" }

// Stream is the DTO returned to the frontend. Field names + omitempty match
// src/types/stream.ts. StreamKey is set ONLY for the publishing owner.
// PlaybackURL is the public HTTP-FLV URL exposed to all viewers when live.
type Stream struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	TitleJa         string `json:"titleJa,omitempty"`
	Description     string `json:"description,omitempty"`
	Channel         string `json:"channel"`
	ChannelID       string `json:"channelId"`
	Verified        bool   `json:"verified"`
	Avatar          string `json:"avatar"`
	Cover           string `json:"cover"`
	Viewers         int64  `json:"viewers"`
	Duration        string `json:"duration"`
	Category        string `json:"category"`
	CategoryJa      string `json:"categoryJa,omitempty"`
	StartedAt       string `json:"startedAt"`
	IsLive          bool   `json:"isLive"`
	OwnerID         string `json:"ownerId,omitempty"`
	PlaybackURL     string `json:"playbackUrl,omitempty"`
	StreamKey       string `json:"streamKey,omitempty"`
	Status          string `json:"status,omitempty"`
	SubscriberCount int64  `json:"subscriberCount"`
}

// ToStream converts a Room into the public DTO. Duration is computed live.
// streamKey is intentionally NOT populated here — callers add it only for
// the owner (POST /rooms/live response).
func (r *Room) ToStream(now time.Time) Stream {
	st := Stream{
		ID:          r.ID,
		Title:       r.Title,
		TitleJa:     r.TitleJa,
		Description: r.Description,
		Channel:     r.Channel,
		ChannelID:   r.ChannelID,
		Verified:    r.Verified,
		Avatar:      r.Avatar,
		Cover:       r.Cover,
		Viewers:     r.Viewers,
		Duration:    FormatDuration(now.Sub(r.StartedAt)),
		Category:    r.Category,
		CategoryJa:  r.CategoryJa,
		StartedAt:   r.StartedAt.UTC().Format(time.RFC3339),
		IsLive:      r.Status == StatusLive,
		OwnerID:     r.OwnerID,
		Status:      r.Status,
	}
	return st
}

// FormatDuration renders h:mm:ss for the frontend.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}
