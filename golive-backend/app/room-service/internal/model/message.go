package model

import "time"

const (
	MessagePreferenceScopeAll       = "all"
	MessagePreferenceScopeFollowing = "following"
	MessagePreferenceScopeNone      = "none"

	FanGroupRoleOwner  = "owner"
	FanGroupRoleAdmin  = "admin"
	FanGroupRoleMember = "member"
)

type DirectThread struct {
	ID                  string     `gorm:"primaryKey;type:varchar(64)"`
	ViewerID            string     `gorm:"type:varchar(36);not null;uniqueIndex:idx_direct_thread_pair,priority:1;index"`
	CreatorID           string     `gorm:"type:varchar(36);not null;uniqueIndex:idx_direct_thread_pair,priority:2;index"`
	ChannelID           string     `gorm:"type:varchar(64);not null;index"`
	LastMessageID       string     `gorm:"type:varchar(64)"`
	LastMessagePreview  string     `gorm:"type:varchar(300)"`
	LastSenderID        string     `gorm:"type:varchar(36);index"`
	LastMessageAt       *time.Time `gorm:"index"`
	ViewerUnread        int64      `gorm:"not null;default:0"`
	CreatorUnread       int64      `gorm:"not null;default:0"`
	ViewerPinnedAt      *time.Time `gorm:"index"`
	CreatorPinnedAt     *time.Time `gorm:"index"`
	ViewerMuted         bool       `gorm:"not null;default:false"`
	CreatorMuted        bool       `gorm:"not null;default:false"`
	ViewerPushDisabled  bool       `gorm:"not null;default:false"`
	CreatorPushDisabled bool       `gorm:"not null;default:false"`
	ViewerArchivedAt    *time.Time `gorm:"index"`
	CreatorArchivedAt   *time.Time `gorm:"index"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (DirectThread) TableName() string { return "direct_threads" }

type DirectMessage struct {
	ID         string    `gorm:"primaryKey;type:varchar(64)"`
	ThreadID   string    `gorm:"type:varchar(64);not null;index"`
	SenderID   string    `gorm:"type:varchar(36);not null;index"`
	ReceiverID string    `gorm:"type:varchar(36);not null;index"`
	Body       string    `gorm:"type:varchar(1200);not null"`
	CreatedAt  time.Time `gorm:"not null;index"`
}

func (DirectMessage) TableName() string { return "direct_messages" }

type UserBlock struct {
	BlockerID    string    `gorm:"primaryKey;type:varchar(36)"`
	TargetUserID string    `gorm:"primaryKey;type:varchar(36);index"`
	TargetRole   string    `gorm:"type:varchar(16);not null;default:'user';index"`
	Reason       string    `gorm:"type:varchar(300)"`
	CreatedAt    time.Time `gorm:"not null;index"`
	UpdatedAt    time.Time
}

func (UserBlock) TableName() string { return "user_blocks" }

type MessagePreference struct {
	UserID                 string `gorm:"primaryKey;type:varchar(36)"`
	MessageReminderEnabled bool   `gorm:"not null;default:true"`
	ReplyReminderScope     string `gorm:"type:varchar(16);not null;default:'all'"`
	MentionReminderScope   string `gorm:"type:varchar(16);not null;default:'all'"`
	LikeReminderEnabled    bool   `gorm:"not null;default:true"`
	FoldUnfollowedMessages bool   `gorm:"not null;default:false"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (MessagePreference) TableName() string { return "message_preferences" }

type FanGroupChat struct {
	ID          string `gorm:"primaryKey;type:varchar(64)"`
	CreatorID   string `gorm:"type:varchar(36);not null;uniqueIndex:idx_fan_group_creator_no,priority:1;index"`
	GroupNo     int    `gorm:"not null;uniqueIndex:idx_fan_group_creator_no,priority:2"`
	Name        string `gorm:"type:varchar(96);not null"`
	MemberCount int    `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (FanGroupChat) TableName() string { return "fan_group_chats" }

type FanGroupMember struct {
	GroupID    string     `gorm:"primaryKey;type:varchar(64);index"`
	UserID     string     `gorm:"primaryKey;type:varchar(36);index"`
	Role       string     `gorm:"type:varchar(16);not null;default:'member';index"`
	MutedUntil *time.Time `gorm:"index"`
	KickedAt   *time.Time `gorm:"index"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (FanGroupMember) TableName() string { return "fan_group_members" }
