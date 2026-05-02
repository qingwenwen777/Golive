package model

import "time"

const (
	AppointmentScheduled = "scheduled"
	AppointmentLive      = "live"
	AppointmentCompleted = "completed"
	AppointmentExpired   = "expired"
	AppointmentCanceled  = "canceled"
)

type LiveAppointment struct {
	ID             string     `gorm:"primaryKey;type:varchar(64)"`
	OwnerID        string     `gorm:"type:varchar(36);not null;index:idx_owner_status_time,priority:1"`
	RoomID         string     `gorm:"type:varchar(64);not null;uniqueIndex"`
	Title          string     `gorm:"type:varchar(255);not null"`
	Description    string     `gorm:"type:text"`
	Cover          string     `gorm:"type:varchar(500)"`
	ScheduledAt    time.Time  `gorm:"not null;index:idx_owner_status_time,priority:3;index"`
	Status         string     `gorm:"type:varchar(16);not null;default:'scheduled';index:idx_owner_status_time,priority:2"`
	StartedAt      *time.Time `gorm:"index"`
	EndedAt        *time.Time
	ReminderSentAt *time.Time
	StartNotifiedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (LiveAppointment) TableName() string { return "live_appointments" }

type AppointmentReservation struct {
	AppointmentID string    `gorm:"primaryKey;type:varchar(64);index"`
	UserID        string    `gorm:"primaryKey;type:varchar(36);index"`
	CreatedAt     time.Time `gorm:"not null;index"`
}

func (AppointmentReservation) TableName() string { return "appointment_reservations" }

type Notification struct {
	ID        string     `gorm:"primaryKey;type:varchar(64)"`
	UserID    string     `gorm:"type:varchar(36);not null;index"`
	Type      string     `gorm:"type:varchar(32);not null;index"`
	Title     string     `gorm:"type:varchar(255);not null"`
	Body      string     `gorm:"type:varchar(1000)"`
	Link      string     `gorm:"type:varchar(500)"`
	ReadAt    *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"not null;index"`
}

func (Notification) TableName() string { return "notifications" }
