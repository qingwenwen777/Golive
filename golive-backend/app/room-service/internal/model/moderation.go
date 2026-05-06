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

	ReportActionReview        = "review"
	ReportActionDismiss       = "dismiss"
	ReportActionDeleteContent = "delete_content"
	ReportActionWarnUser      = "warn_user"
	ReportActionWarnRoom      = "warn_room"
	ReportActionSiteMute      = "site_mute"
	ReportActionBanUser       = "ban_user"
	ReportActionForceEndLive  = "force_end_live"

	AdminAuditCategoryReview     = "review"
	AdminAuditCategoryPermission = "permission"
	AdminAuditCategorySystem     = "system"

	UserSanctionWarn     = "warn"
	UserSanctionSiteMute = "site_mute"
	UserSanctionBan      = "ban"
	UserSanctionUnban    = "unban"

	UnbanAppealPending   = "pending"
	UnbanAppealReviewing = "reviewing"
	UnbanAppealApproved  = "approved"
	UnbanAppealRejected  = "rejected"
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
	ID               string     `gorm:"primaryKey;type:varchar(64)"`
	GroupID          string     `gorm:"type:varchar(64);not null;default:'';index"`
	ReporterID       string     `gorm:"type:varchar(36);not null;index:idx_content_reports_reporter_created"`
	ReporterName     string     `gorm:"type:varchar(128)"`
	ReporterAvatar   string     `gorm:"type:varchar(500)"`
	TargetType       string     `gorm:"type:varchar(32);not null;index:idx_content_reports_target"`
	TargetID         string     `gorm:"type:varchar(128);not null;index:idx_content_reports_target"`
	TargetURL        string     `gorm:"type:varchar(800)"`
	RoomID           string     `gorm:"type:varchar(64);index"`
	ChannelID        string     `gorm:"type:varchar(64);index"`
	TargetOwnerID    string     `gorm:"type:varchar(36);index"`
	TargetOwnerName  string     `gorm:"type:varchar(128)"`
	TargetUserID     string     `gorm:"type:varchar(36);index"`
	TargetUserName   string     `gorm:"type:varchar(128)"`
	TargetTitle      string     `gorm:"type:varchar(240)"`
	TargetText       string     `gorm:"type:text"`
	Reason           string     `gorm:"type:varchar(32);not null;index"`
	Description      string     `gorm:"type:varchar(300)"`
	Status           string     `gorm:"type:varchar(24);not null;default:pending;index"`
	ReviewerID       string     `gorm:"type:varchar(36);index"`
	ReviewerName     string     `gorm:"type:varchar(128)"`
	ReviewStartedAt  *time.Time `gorm:"index"`
	ReviewExpiresAt  *time.Time `gorm:"index"`
	ResolutionAction string     `gorm:"type:varchar(160);index"`
	DurationMinutes  int
	ResolutionNote   string     `gorm:"type:text"`
	ResolvedAt       *time.Time `gorm:"index"`
	CreatedAt        time.Time  `gorm:"index:idx_content_reports_reporter_created"`
	UpdatedAt        time.Time
}

func (ContentReport) TableName() string { return "content_reports" }

type AdminAuditLog struct {
	ID             string    `gorm:"primaryKey;type:varchar(64)"`
	Category       string    `gorm:"type:varchar(32);not null;index"`
	Action         string    `gorm:"type:varchar(64);not null;index"`
	ActorID        string    `gorm:"type:varchar(36);not null;index"`
	TargetType     string    `gorm:"type:varchar(32);index"`
	TargetID       string    `gorm:"type:varchar(128);index"`
	TargetTitle    string    `gorm:"type:varchar(240)"`
	TargetUserID   string    `gorm:"type:varchar(36);index"`
	TargetUserName string    `gorm:"type:varchar(128)"`
	Note           string    `gorm:"type:text"`
	Metadata       string    `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"index"`
}

func (AdminAuditLog) TableName() string { return "admin_audit_logs" }

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

type UserSanctionLog struct {
	ID              string `gorm:"primaryKey;type:varchar(64)"`
	TargetUserID    string `gorm:"type:varchar(36);not null;index"`
	TargetUserName  string `gorm:"type:varchar(128)"`
	Action          string `gorm:"type:varchar(32);not null;index"`
	OperatorID      string `gorm:"type:varchar(36);not null;index"`
	SourceReportID  string `gorm:"type:varchar(64);index"`
	Note            string `gorm:"type:text"`
	DurationMinutes int
	ExpiresAt       *time.Time `gorm:"index"`
	CreatedAt       time.Time  `gorm:"index"`
}

func (UserSanctionLog) TableName() string { return "user_sanction_logs" }

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
