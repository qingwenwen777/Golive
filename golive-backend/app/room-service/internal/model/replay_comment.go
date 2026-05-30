package model

import (
	"time"

	"gorm.io/gorm"
)

// ReplayComment is a viewer comment on a finished live replay (VOD).
// It mirrors PostComment so the threaded-comment behavior stays consistent.
type ReplayComment struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	RoomID     string `gorm:"type:varchar(64);not null;index"`
	UserID     string `gorm:"type:varchar(36);not null;index"`
	ParentID   string `gorm:"type:varchar(64);index"`
	RootID     string `gorm:"type:varchar(64);index"`
	Depth      int    `gorm:"not null;default:0"`
	Content    string `gorm:"type:text;not null"`
	LikeCount  int64  `gorm:"not null;default:0"`
	ReplyCount int64  `gorm:"not null;default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (ReplayComment) TableName() string { return "replay_comments" }

type ReplayCommentLike struct {
	CommentID string `gorm:"primaryKey;type:varchar(64)"`
	UserID    string `gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time
}

func (ReplayCommentLike) TableName() string { return "replay_comment_likes" }
