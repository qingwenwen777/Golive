package model

import "time"

// User is the GORM table model. JSON tags match the frontend `User` type
// (camelCase). PasswordHash is intentionally json:"-" so it never leaks.
type User struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username     string    `gorm:"uniqueIndex;type:varchar(64);not null" json:"username"`
	DisplayName  string    `gorm:"type:varchar(64)" json:"displayName,omitempty"`
	PasswordHash string    `gorm:"type:varchar(100);not null" json:"-"`
	Avatar       string    `gorm:"type:varchar(500)" json:"avatar"`
	CoinBalance  int64     `gorm:"not null;default:0" json:"coinBalance"`
	Verified     bool      `gorm:"not null;default:false" json:"verified,omitempty"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

func (User) TableName() string { return "users" }

// PublicUser is the DTO returned to the frontend. Equivalent to `User` in
// src/types/user.ts. We re-marshal explicitly to be safe against future
// internal-only fields creeping in.
type PublicUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty"`
	Avatar      string `json:"avatar"`
	CoinBalance int64  `json:"coinBalance"`
	Verified    bool   `json:"verified,omitempty"`
}

func (u *User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Avatar:      u.Avatar,
		CoinBalance: u.CoinBalance,
		Verified:    u.Verified,
	}
}
