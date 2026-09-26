package model

import (
	"fmt"
	"time"
)

// Status values for Room.Status.
const (
	StatusScheduled  = "scheduled"
	StatusPublishing = "publishing"
	StatusLive       = "live"
	StatusEnding     = "ending"
	StatusEnded      = "ended"
	StatusExpired    = "expired"
	StatusCanceled   = "canceled"
)

// Replay status values for Room.ReplayStatus.
const (
	ReplayStatusNone       = "none"
	ReplayStatusPending    = "pending"
	ReplayStatusUploading  = "uploading"
	ReplayStatusProcessing = "processing"
	ReplayStatusReady      = "ready"
	ReplayStatusFailed     = "failed"
	ReplayStatusDeleted    = "deleted"
)

// Room is the GORM model. Internal-only fields are kept off the JSON wire by
// converting to Stream (the DTO) before responding.
type Room struct {
	ID                   string    `gorm:"primaryKey;type:varchar(64)"`
	Title                string    `gorm:"type:varchar(255);not null"`
	TitleJa              string    `gorm:"type:varchar(255)"`
	Description          string    `gorm:"type:text"`
	Channel              string    `gorm:"type:varchar(128);index"`
	ChannelID            string    `gorm:"type:varchar(64);index"`
	Verified             bool      `gorm:"not null;default:false"`
	Avatar               string    `gorm:"type:varchar(500)"`
	Cover                string    `gorm:"type:varchar(500)"`
	Viewers              int64     `gorm:"not null;default:0;index:idx_rooms_live_list,priority:3,sort:desc"`
	PeakViewers          int64     `gorm:"not null;default:0"`
	Category             string    `gorm:"type:varchar(64);index"`
	CategoryJa           string    `gorm:"type:varchar(64)"`
	StartedAt            time.Time `gorm:"index;index:idx_rooms_live_list,priority:2,sort:desc"`
	Status               string    `gorm:"type:varchar(16);not null;default:'live';index;index:idx_rooms_live_list,priority:1;index:idx_rooms_owner_status,priority:2"`
	OwnerID              string    `gorm:"type:varchar(36);index;index:idx_rooms_owner_status,priority:1"`
	StreamKey            string    `gorm:"type:varchar(128);index"`
	ReplayUploadEnabled  bool      `gorm:"not null;default:false"`
	ReplayStatus         string    `gorm:"type:varchar(20);not null;default:'none';index"`
	ReplayVisibility     string    `gorm:"type:varchar(16);not null;default:'public';index"`
	FanClubOnly          bool      `gorm:"not null;default:false;index"`
	ReplayBunnyVideoID   string    `gorm:"type:varchar(80);index"`
	ReplayBunnyLibraryID string    `gorm:"type:varchar(32)"`
	ReplayError          string    `gorm:"type:text"`
	ReplayUploadedAt     *time.Time
	ReplayDeletedAt      *time.Time
	EndedAt              *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (Room) TableName() string { return "rooms" }

type RoomWatchEvent struct {
	ID              string    `gorm:"primaryKey;type:varchar(64)"`
	UserID          string    `gorm:"type:varchar(36);not null;index:idx_watch_user_time,priority:1;index:idx_watch_user_date,priority:1;uniqueIndex:idx_watch_user_room,priority:1"`
	RoomID          string    `gorm:"type:varchar(64);not null;index;uniqueIndex:idx_watch_user_room,priority:2"`
	ChannelID       string    `gorm:"type:varchar(64);index"`
	OwnerID         string    `gorm:"type:varchar(36);index"`
	Category        string    `gorm:"type:varchar(64);index"`
	WatchCount      int64     `gorm:"not null;default:1"`
	WatchDate       string    `gorm:"type:varchar(10);not null;default:'';index:idx_watch_user_date,priority:2"`
	DailyWatchCount int64     `gorm:"not null;default:1"`
	LastWatchedAt   time.Time `gorm:"not null;index:idx_watch_user_time,priority:2"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (RoomWatchEvent) TableName() string { return "room_watch_events" }

const (
	LibraryTypeHistory    = "history"
	LibraryTypeWatchLater = "watch_later"
	LibraryTypeLiked      = "liked"
)

type UserLibraryItem struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	UserID    string    `gorm:"type:varchar(36);not null;index:idx_user_library_user_type_time,priority:1;uniqueIndex:idx_user_library_user_type_room,priority:1"`
	Type      string    `gorm:"type:varchar(20);not null;index:idx_user_library_user_type_time,priority:2;uniqueIndex:idx_user_library_user_type_room,priority:2"`
	RoomID    string    `gorm:"type:varchar(64);not null;index;uniqueIndex:idx_user_library_user_type_room,priority:3"`
	SavedAt   time.Time `gorm:"not null;index:idx_user_library_user_type_time,priority:3,sort:desc"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (UserLibraryItem) TableName() string { return "user_library_items" }

// Stream is the DTO returned to the frontend. Field names + omitempty match
// src/types/stream.ts. StreamKey is set ONLY for the publishing owner.
// PlaybackURL is the public HTTP-FLV URL exposed to all viewers when live.
type Stream struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	TitleJa         string  `json:"titleJa,omitempty"`
	Description     string  `json:"description,omitempty"`
	Channel         string  `json:"channel"`
	ChannelID       string  `json:"channelId"`
	Verified        bool    `json:"verified"`
	Avatar          string  `json:"avatar"`
	Cover           string  `json:"cover"`
	Viewers         int64   `json:"viewers"`
	PeakViewers     int64   `json:"peakViewers,omitempty"`
	Duration        string  `json:"duration"`
	Category        string  `json:"category"`
	CategoryJa      string  `json:"categoryJa,omitempty"`
	StartedAt       string  `json:"startedAt"`
	EndedAt         string  `json:"endedAt,omitempty"`
	IsLive          bool    `json:"isLive"`
	OwnerID         string  `json:"ownerId,omitempty"`
	PlaybackURL     string  `json:"playbackUrl,omitempty"`
	StreamKey       string  `json:"streamKey,omitempty"`
	Status          string  `json:"status,omitempty"`
	SubscriberCount int64   `json:"subscriberCount"`
	FanClubOnly     bool    `json:"fanClubOnly"`
	FanClubMember   bool    `json:"fanClubMember,omitempty"`
	Replay          *Replay `json:"replay,omitempty"`
}

// Replay is the public wire shape for a room's replay asset. The room id stays
// the business id; bunnyVideoId is just the external video asset id.
type Replay struct {
	RoomID         string `json:"roomId"`
	Status         string `json:"status"`
	Visibility     string `json:"visibility"`
	UploadAfterEnd bool   `json:"uploadAfterEnd"`
	EmbedURL       string `json:"embedUrl,omitempty"`
	BunnyVideoID   string `json:"bunnyVideoId,omitempty"`
	UploadedAt     string `json:"uploadedAt,omitempty"`
	DeletedAt      string `json:"deletedAt,omitempty"`
	Error          string `json:"error,omitempty"`
	CanWatch       bool   `json:"canWatch"`
	CanManage      bool   `json:"canManage"`
}

// ToStream converts a Room into the public DTO. Duration is computed live.
// streamKey is intentionally NOT populated here — callers add it only for
// the owner (POST /rooms/live response).
func (r *Room) ToStream(now time.Time) Stream {
	durationEnd := now
	endedAt := ""
	if r.EndedAt != nil {
		durationEnd = *r.EndedAt
		endedAt = r.EndedAt.UTC().Format(time.RFC3339)
	}
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
		PeakViewers: r.PeakViewers,
		Duration:    FormatDuration(durationEnd.Sub(r.StartedAt)),
		Category:    r.Category,
		CategoryJa:  r.CategoryJa,
		StartedAt:   r.StartedAt.UTC().Format(time.RFC3339),
		EndedAt:     endedAt,
		IsLive:      r.Status == StatusLive,
		OwnerID:     r.OwnerID,
		Status:      r.Status,
		FanClubOnly: r.FanClubOnly,
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
