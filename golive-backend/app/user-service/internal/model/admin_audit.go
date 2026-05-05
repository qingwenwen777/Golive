package model

import "time"

const (
	AdminAuditCategoryReview     = "review"
	AdminAuditCategoryPermission = "permission"
	AdminAuditCategorySystem     = "system"
)

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
