package model

import "time"

const (
	ModeratorActionAdd    = "add_moderator"
	ModeratorActionRemove = "remove_moderator"
	ModeratorActionMute   = "mute"
	ModeratorActionUnmute = "unmute"
)

const (
	ReportStatusPending   = "pending"
	ReportStatusReviewing = "reviewing"
	ReportStatusResolved  = "resolved"
	ReportStatusDismissed = "dismissed"

	ReportTargetRoom        = "room"
	ReportTargetChannel     = "channel"
	ReportTargetDanmu       = "danmu"
	ReportTargetPost        = "post"
	ReportTargetPostComment = "post_comment"
	ReportTargetSuperChat   = "super_chat"

	ReportReasonSpam       = "spam"
	ReportReasonHarassment = "harassment"
	ReportReasonSexual     = "sexual"
	ReportReasonViolence   = "violence"
	ReportReasonHate       = "hate"
	ReportReasonScam       = "scam"
	ReportReasonIllegal    = "illegal"
	ReportReasonOther      = "other"
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

type ContentReport struct {
	ID              string     `gorm:"primaryKey;type:varchar(64)"`
	ReporterID      string     `gorm:"type:varchar(36);not null;index:idx_content_reports_reporter_created"`
	ReporterName    string     `gorm:"type:varchar(128)"`
	ReporterAvatar  string     `gorm:"type:varchar(500)"`
	TargetType      string     `gorm:"type:varchar(32);not null;index:idx_content_reports_target"`
	TargetID        string     `gorm:"type:varchar(128);not null;index:idx_content_reports_target"`
	TargetURL       string     `gorm:"type:varchar(800)"`
	RoomID          string     `gorm:"type:varchar(64);index"`
	ChannelID       string     `gorm:"type:varchar(64);index"`
	TargetOwnerID   string     `gorm:"type:varchar(36);index"`
	TargetOwnerName string     `gorm:"type:varchar(128)"`
	TargetUserID    string     `gorm:"type:varchar(36);index"`
	TargetUserName  string     `gorm:"type:varchar(128)"`
	TargetTitle     string     `gorm:"type:varchar(240)"`
	TargetText      string     `gorm:"type:text"`
	Reason          string     `gorm:"type:varchar(32);not null;index"`
	Description     string     `gorm:"type:varchar(300)"`
	Status          string     `gorm:"type:varchar(24);not null;default:pending;index"`
	ReviewerID      string     `gorm:"type:varchar(36);index"`
	ResolutionNote  string     `gorm:"type:text"`
	ResolvedAt      *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"index:idx_content_reports_reporter_created"`
	UpdatedAt       time.Time
}

func (ContentReport) TableName() string { return "content_reports" }

type BlockedWord struct {
	ID             string `gorm:"primaryKey;type:varchar(64)"`
	Word           string `gorm:"type:varchar(120);not null"`
	NormalizedWord string `gorm:"type:varchar(120);not null;uniqueIndex"`
	Note           string `gorm:"type:varchar(300)"`
	Enabled        bool   `gorm:"not null;default:true;index"`
	CreatedBy      string `gorm:"type:varchar(36);index"`
	UpdatedBy      string `gorm:"type:varchar(36);index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (BlockedWord) TableName() string { return "blocked_words" }
