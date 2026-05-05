package model

import "time"

const (
	UnbanAppealPending   = "pending"
	UnbanAppealReviewing = "reviewing"
	UnbanAppealApproved  = "approved"
	UnbanAppealRejected  = "rejected"
)

type UserModerationState struct {
	UserID     string     `gorm:"primaryKey;type:varchar(36)"`
	Banned     bool       `gorm:"not null;default:false;index"`
	BanReason  string     `gorm:"type:varchar(300)"`
	MutedUntil *time.Time `gorm:"index"`
	MuteReason string     `gorm:"type:varchar(300)"`
	UpdatedBy  string     `gorm:"type:varchar(36);index"`
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

func (UserModerationState) TableName() string { return "user_moderation_states" }

type UnbanAppeal struct {
	ID         string     `gorm:"primaryKey;type:varchar(64)"`
	UserID     string     `gorm:"type:varchar(36);not null;index"`
	Reason     string     `gorm:"type:varchar(1000)"`
	Status     string     `gorm:"type:varchar(24);not null;default:pending;index"`
	ReviewerID string     `gorm:"type:varchar(36);index"`
	ReviewNote string     `gorm:"type:text"`
	ReviewedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"index"`
	UpdatedAt  time.Time
}

func (UnbanAppeal) TableName() string { return "unban_appeals" }
