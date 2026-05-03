package service

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func newAppointmentServiceTestDeps(t *testing.T) (*AppointmentService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	appointments := repo.NewAppointmentRepo(db)
	require.NoError(t, appointments.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE users (
	id text primary key,
	username text,
	display_name text,
	avatar text,
	verified boolean
)`).Error)

	return NewAppointmentService(appointments, rooms, nil, nil), db
}

func TestNotificationsBackfillActorFromLiveLinkAndProfile(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)

	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified) VALUES (?, ?, ?, ?, ?)`,
		"owner-1", "creator", "Creator Display", "/uploads/avatar.png", true,
	).Error)
	require.NoError(t, db.Create(&model.Room{
		ID:        "room-1",
		OwnerID:   "owner-1",
		Channel:   "Room Channel",
		ChannelID: "ch-owner-1",
		Avatar:    "/room-avatar.png",
		Cover:     "/cover.png",
		Title:     "Room title",
		Status:    model.StatusEnded,
		StartedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&model.Notification{
		ID:        "notification-1",
		UserID:    "viewer-1",
		Type:      "appointment_started",
		Title:     "Appointment started",
		Body:      "Show title",
		Link:      "/live/room-1",
		CreatedAt: time.Now(),
	}).Error)

	resp, err := svc.Notifications(ctx, "viewer-1", 1, 10)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "owner-1", resp.Items[0].ActorID)
	require.Equal(t, "creator", resp.Items[0].ActorUsername)
	require.Equal(t, "Creator Display", resp.Items[0].ActorName)
	require.Equal(t, "/uploads/avatar.png", resp.Items[0].ActorAvatar)
	require.True(t, resp.Items[0].ActorVerified)
}

func TestNotificationsPreferCurrentProfileAvatar(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)

	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, display_name, avatar, verified) VALUES (?, ?, ?, ?, ?)`,
		"owner-1", "creator", "Creator Display", "/uploads/current.png", false,
	).Error)
	require.NoError(t, db.Create(&model.Notification{
		ID:          "notification-1",
		UserID:      "viewer-1",
		Type:        "appointment_started",
		Title:       "Appointment started",
		Body:        "Show title",
		Link:        "/live/missing-room",
		ActorID:     "owner-1",
		ActorName:   "Old name",
		ActorAvatar: "/uploads/old.png",
		CreatedAt:   time.Now(),
	}).Error)

	resp, err := svc.Notifications(ctx, "viewer-1", 1, 10)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "Creator Display", resp.Items[0].ActorName)
	require.Equal(t, "/uploads/current.png", resp.Items[0].ActorAvatar)
}
