package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	PostVisibilityPublic    = "public"
	PostVisibilityFollowers = "followers"
	PostVisibilityPrivate   = "private"

	PostCommentModeEveryone  = "everyone"
	PostCommentModeFollowers = "followers"
)

type ChannelPost struct {
	ID              string `gorm:"primaryKey;type:varchar(64)"`
	OwnerID         string `gorm:"type:varchar(36);not null;index"`
	ChannelID       string `gorm:"type:varchar(64);not null;index"`
	Content         string `gorm:"type:text;not null"`
	ImagesJSON      string `gorm:"type:text"`
	Visibility      string `gorm:"type:varchar(16);not null;default:'public';index"`
	CommentsEnabled bool   `gorm:"not null;default:true"`
	CommentMode     string `gorm:"type:varchar(16);not null;default:'everyone'"`
	LikeCount       int64  `gorm:"not null;default:0"`
	CommentCount    int64  `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func (ChannelPost) TableName() string { return "channel_posts" }

type PostComment struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	PostID     string `gorm:"type:varchar(64);not null;index"`
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

func (PostComment) TableName() string { return "post_comments" }

type PostLike struct {
	PostID    string `gorm:"primaryKey;type:varchar(64)"`
	UserID    string `gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time
}

func (PostLike) TableName() string { return "post_likes" }

type PostCommentLike struct {
	CommentID string `gorm:"primaryKey;type:varchar(64)"`
	UserID    string `gorm:"primaryKey;type:varchar(36)"`
	CreatedAt time.Time
}

func (PostCommentLike) TableName() string { return "post_comment_likes" }
