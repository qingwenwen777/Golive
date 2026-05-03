package model

import "time"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"

	LivePermissionNone     = "none"
	LivePermissionPending  = "pending"
	LivePermissionApproved = "approved"
	LivePermissionRejected = "rejected"
)

// User is the GORM table model. JSON tags match the frontend `User` type
// (camelCase). PasswordHash is intentionally json:"-" so it never leaks.
type User struct {
	ID                         string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username                   string     `gorm:"uniqueIndex;type:varchar(64);not null" json:"username"`
	DisplayName                string     `gorm:"type:varchar(64)" json:"displayName,omitempty"`
	PasswordHash               string     `gorm:"type:varchar(100);not null" json:"-"`
	UsernameUpdatedAt          *time.Time `gorm:"index" json:"-"`
	Avatar                     string     `gorm:"type:varchar(500)" json:"avatar"`
	Cover                      string     `gorm:"type:varchar(500)" json:"cover"`
	CoinBalance                int64      `gorm:"not null;default:0" json:"coinBalance"`
	Verified                   bool       `gorm:"not null;default:false" json:"verified,omitempty"`
	Role                       string     `gorm:"type:varchar(16);not null;default:user" json:"role"`
	LivePermissionStatus       string     `gorm:"type:varchar(16);not null;default:none" json:"livePermissionStatus"`
	LivePermissionRejectReason string     `gorm:"type:text" json:"livePermissionRejectReason,omitempty"`
	CreatedAt                  time.Time  `json:"-"`
	UpdatedAt                  time.Time  `json:"-"`
}

func (User) TableName() string { return "users" }

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

// PublicUser is the DTO returned to the frontend. Equivalent to `User` in
// src/types/user.ts. We re-marshal explicitly to be safe against future
// internal-only fields creeping in.
type PublicUser struct {
	ID                         string `json:"id"`
	Username                   string `json:"username"`
	DisplayName                string `json:"displayName,omitempty"`
	UsernameUpdatedAt          string `json:"usernameUpdatedAt,omitempty"`
	UsernameChangeAvailableAt  string `json:"usernameChangeAvailableAt,omitempty"`
	Avatar                     string `json:"avatar"`
	Cover                      string `json:"cover"`
	CoinBalance                int64  `json:"coinBalance"`
	Verified                   bool   `json:"verified,omitempty"`
	Role                       string `json:"role"`
	LivePermissionStatus       string `json:"livePermissionStatus"`
	LivePermissionRejectReason string `json:"livePermissionRejectReason,omitempty"`
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
	pu := PublicUser{
		ID:                         u.ID,
		Username:                   u.Username,
		DisplayName:                u.DisplayName,
		Avatar:                     u.Avatar,
		Cover:                      u.Cover,
		CoinBalance:                u.CoinBalance,
		Verified:                   u.Verified,
		Role:                       role,
		LivePermissionStatus:       liveStatus,
		LivePermissionRejectReason: u.LivePermissionRejectReason,
	}
	if u.UsernameUpdatedAt != nil {
		pu.UsernameUpdatedAt = u.UsernameUpdatedAt.UTC().Format(time.RFC3339)
		pu.UsernameChangeAvailableAt = u.UsernameUpdatedAt.Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return pu
}
