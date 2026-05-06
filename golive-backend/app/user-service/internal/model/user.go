package model

import (
	"time"

	"github.com/qingwenwen777/golive/pkg/userlevel"
)

const (
	RoleUser      = "user"
	RoleAdmin     = "admin"
	RoleModerator = "moderator"

	LivePermissionNone     = "none"
	LivePermissionPending  = "pending"
	LivePermissionApproved = "approved"
	LivePermissionRejected = "rejected"

	PlatformVerificationNone     = "none"
	PlatformVerificationPending  = "pending"
	PlatformVerificationApproved = "approved"
	PlatformVerificationRejected = "rejected"
)

// User is the GORM table model. JSON tags match the frontend `User` type
// (camelCase). PasswordHash is intentionally json:"-" so it never leaks.
type User struct {
	ID                               string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username                         string     `gorm:"uniqueIndex;type:varchar(64);not null" json:"username"`
	Email                            string     `gorm:"uniqueIndex;type:varchar(255)" json:"-"`
	GoogleSub                        *string    `gorm:"uniqueIndex;type:varchar(255)" json:"-"`
	GoogleLinkedAt                   *time.Time `gorm:"index" json:"-"`
	DisplayName                      string     `gorm:"type:varchar(64)" json:"displayName,omitempty"`
	PasswordHash                     string     `gorm:"type:varchar(100);not null" json:"-"`
	UsernameUpdatedAt                *time.Time `gorm:"index" json:"-"`
	Avatar                           string     `gorm:"type:varchar(500)" json:"avatar"`
	Cover                            string     `gorm:"type:varchar(500)" json:"cover"`
	CoinBalance                      int64      `gorm:"not null;default:0" json:"coinBalance"`
	FrozenCoins                      int64      `gorm:"not null;default:0" json:"frozenCoins"`
	TotalTopupCoins                  int64      `gorm:"-" json:"-"`
	Banned                           bool       `gorm:"not null;default:false" json:"banned,omitempty"`
	BanReason                        string     `gorm:"type:text" json:"banReason,omitempty"`
	Verified                         bool       `gorm:"not null;default:false" json:"verified,omitempty"`
	Role                             string     `gorm:"type:varchar(16);not null;default:user" json:"role"`
	LivePermissionStatus             string     `gorm:"type:varchar(16);not null;default:none" json:"livePermissionStatus"`
	LivePermissionRejectReason       string     `gorm:"type:text" json:"livePermissionRejectReason,omitempty"`
	PlatformVerificationStatus       string     `gorm:"type:varchar(16);not null;default:none" json:"platformVerificationStatus"`
	PlatformVerificationRejectReason string     `gorm:"type:text" json:"platformVerificationRejectReason,omitempty"`
	CreatedAt                        time.Time  `json:"-"`
	UpdatedAt                        time.Time  `json:"-"`
}

func (User) TableName() string { return "users" }

type InviteCode struct {
	ID        string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Code      string     `gorm:"uniqueIndex;type:varchar(32);not null" json:"code"`
	CreatedBy string     `gorm:"index;type:varchar(36);not null" json:"createdBy"`
	UsedBy    string     `gorm:"index;type:varchar(36)" json:"usedBy,omitempty"`
	UsedAt    *time.Time `gorm:"index" json:"usedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func (InviteCode) TableName() string { return "invite_codes" }

type CreatorApplication struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID       string     `gorm:"index;type:varchar(36);not null" json:"userId"`
	Reason       string     `gorm:"type:text" json:"reason"`
	Status       string     `gorm:"index;type:varchar(16);not null;default:pending" json:"status"`
	ReviewerID   string     `gorm:"type:varchar(36)" json:"reviewerId,omitempty"`
	RejectReason string     `gorm:"type:text" json:"rejectReason,omitempty"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (CreatorApplication) TableName() string { return "creator_applications" }

type PlatformApplication struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID       string     `gorm:"index;type:varchar(36);not null" json:"userId"`
	Reason       string     `gorm:"type:text" json:"reason"`
	Status       string     `gorm:"index;type:varchar(16);not null;default:pending" json:"status"`
	ReviewerID   string     `gorm:"type:varchar(36)" json:"reviewerId,omitempty"`
	RejectReason string     `gorm:"type:text" json:"rejectReason,omitempty"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (PlatformApplication) TableName() string { return "platform_applications" }

// PublicUser is the DTO returned to the frontend. Equivalent to `User` in
// src/types/user.ts. We re-marshal explicitly to be safe against future
// internal-only fields creeping in.
type PublicUser struct {
	ID                               string             `json:"id"`
	Username                         string             `json:"username"`
	Email                            string             `json:"email,omitempty"`
	DisplayName                      string             `json:"displayName,omitempty"`
	UsernameUpdatedAt                string             `json:"usernameUpdatedAt,omitempty"`
	UsernameChangeAvailableAt        string             `json:"usernameChangeAvailableAt,omitempty"`
	Avatar                           string             `json:"avatar"`
	Cover                            string             `json:"cover"`
	CoinBalance                      int64              `json:"coinBalance"`
	FrozenCoins                      int64              `json:"frozenCoins,omitempty"`
	LevelInfo                        userlevel.Snapshot `json:"levelInfo"`
	Banned                           bool               `json:"banned,omitempty"`
	BanReason                        string             `json:"banReason,omitempty"`
	Verified                         bool               `json:"verified,omitempty"`
	Role                             string             `json:"role"`
	LivePermissionStatus             string             `json:"livePermissionStatus"`
	LivePermissionRejectReason       string             `json:"livePermissionRejectReason,omitempty"`
	PlatformVerificationStatus       string             `json:"platformVerificationStatus"`
	PlatformVerificationRejectReason string             `json:"platformVerificationRejectReason,omitempty"`
	GoogleLinked                     bool               `json:"googleLinked,omitempty"`
}

func (u *User) Public() PublicUser {
	role := u.Role
	if role == "" {
		role = RoleUser
	}
	liveStatus := u.LivePermissionStatus
	if liveStatus == "" {
		liveStatus = LivePermissionNone
	}
	platformStatus := u.PlatformVerificationStatus
	if platformStatus == "" {
		platformStatus = PlatformVerificationNone
	}
	pu := PublicUser{
		ID:                               u.ID,
		Username:                         u.Username,
		Email:                            u.Email,
		DisplayName:                      u.DisplayName,
		Avatar:                           u.Avatar,
		Cover:                            u.Cover,
		CoinBalance:                      u.CoinBalance,
		FrozenCoins:                      u.FrozenCoins,
		LevelInfo:                        userlevel.SnapshotForTotalTopup(u.TotalTopupCoins),
		Banned:                           u.Banned,
		BanReason:                        u.BanReason,
		Verified:                         u.Verified,
		Role:                             role,
		LivePermissionStatus:             liveStatus,
		LivePermissionRejectReason:       u.LivePermissionRejectReason,
		PlatformVerificationStatus:       platformStatus,
		PlatformVerificationRejectReason: u.PlatformVerificationRejectReason,
		GoogleLinked:                     u.GoogleSub != nil && *u.GoogleSub != "",
	}
	if u.UsernameUpdatedAt != nil {
		pu.UsernameUpdatedAt = u.UsernameUpdatedAt.UTC().Format(time.RFC3339)
		pu.UsernameChangeAvailableAt = u.UsernameUpdatedAt.Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return pu
}
