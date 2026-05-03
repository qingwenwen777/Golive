package model

import "time"

const (
	ModeratorActionAdd    = "add_moderator"
	ModeratorActionRemove = "remove_moderator"
	ModeratorActionMute   = "mute"
)

type RoomModerator struct {
	OwnerID   string `gorm:"primaryKey;type:varchar(36)"`
	UserID    string `gorm:"primaryKey;type:varchar(36)"`
	CreatedBy string `gorm:"type:varchar(36);not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	RevokedAt *time.Time `gorm:"index"`
}

func (RoomModerator) TableName() string { return "room_moderators" }

type RoomMute struct {
	ID              string `gorm:"primaryKey;type:varchar(64)"`
	RoomID          string `gorm:"type:varchar(64);not null;index"`
	OwnerID         string `gorm:"type:varchar(36);not null;index"`
	TargetUserID    string `gorm:"type:varchar(36);not null;index"`
	TargetName      string `gorm:"type:varchar(128)"`
	TargetAvatar    string `gorm:"type:varchar(500)"`
	OperatorID      string `gorm:"type:varchar(36);not null;index"`
	OperatorRole    string `gorm:"type:varchar(16);not null"`
	DurationMinutes int    `gorm:"not null"`
	StartedAt       time.Time
	ExpiresAt       time.Time `gorm:"index"`
	CreatedAt       time.Time
}

func (RoomMute) TableName() string { return "room_mutes" }

type ModeratorActionLog struct {
	ID              string `gorm:"primaryKey;type:varchar(64)"`
	OwnerID         string `gorm:"type:varchar(36);not null;index"`
	RoomID          string `gorm:"type:varchar(64);index"`
	ActorID         string `gorm:"type:varchar(36);not null;index"`
	ActorName       string `gorm:"type:varchar(128)"`
	ActorAvatar     string `gorm:"type:varchar(500)"`
	TargetUserID    string `gorm:"type:varchar(36);not null;index"`
	TargetName      string `gorm:"type:varchar(128)"`
	TargetAvatar    string `gorm:"type:varchar(500)"`
	Action          string `gorm:"type:varchar(32);not null;index"`
	DurationMinutes int
	CreatedAt       time.Time `gorm:"index"`
}

func (ModeratorActionLog) TableName() string { return "moderator_action_logs" }
