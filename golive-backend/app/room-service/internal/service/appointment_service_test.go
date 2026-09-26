package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// substringTextPolicy rejects any text containing word (case-insensitive).
type substringTextPolicy struct{ word string }

func (p *substringTextPolicy) EnsureTextAllowed(_ context.Context, texts ...string) error {
	for _, text := range texts {
		if p.word != "" && strings.Contains(strings.ToLower(text), p.word) {
			return errcode.New(http.StatusBadRequest, "content contains blocked word").WithReason("blocked_word")
		}
	}
	return nil
}

func (p *substringTextPolicy) EnsureUserCanInteract(context.Context, string) error { return nil }

func requireBlockedWord(t *testing.T, err error) {
	t.Helper()
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr), "expected blocked_word error, got %v", err)
	require.Equal(t, "blocked_word", appErr.Reason)
}

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

func TestAppointmentCreateAndUpdatePersistCategory(t *testing.T) {
	ctx := context.Background()
	svc, _ := newAppointmentServiceTestDeps(t)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	payload := AppointmentPayload{
		ScheduledAt: now.Add(2 * time.Hour),
		Title:       "Category appointment",
		Description: "A scheduled live with a category",
		Category:    "Gaming",
		Cover:       "/uploads/gaming.jpg",
		ChannelName: "Creator Channel",
	}
	created, err := svc.Create(ctx, "owner-category", payload)
	require.NoError(t, err)
	require.Equal(t, "Gaming", created.Category)

	room, err := svc.rooms.GetByID(ctx, created.RoomID)
	require.NoError(t, err)
	require.Equal(t, "Gaming", room.Category)

	payload.Category = "Music"
	payload.Title = "Music appointment"
	updated, err := svc.Update(ctx, "owner-category", created.ID, payload)
	require.NoError(t, err)
	require.Equal(t, "Music", updated.Category)

	room, err = svc.rooms.GetByID(ctx, created.RoomID)
	require.NoError(t, err)
	require.Equal(t, "Music", room.Category)
}

func TestAppointmentDeleteRecordRemovesOwnerRecordAndScheduledRoom(t *testing.T) {
	ctx := context.Background()
	svc, _ := newAppointmentServiceTestDeps(t)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(ctx, "owner-delete-record", AppointmentPayload{
		ScheduledAt: now.Add(2 * time.Hour),
		Title:       "Delete me",
		Description: "Remove this scheduled appointment from the studio list",
		Category:    "Music",
		Cover:       "/uploads/delete.jpg",
		ChannelName: "Creator Channel",
	})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteRecord(ctx, "owner-delete-record", created.ID))

	resp, err := svc.ListOwner(ctx, "owner-delete-record", 1, 10)
	require.NoError(t, err)
	require.Zero(t, resp.Total)
	_, err = svc.rooms.GetByID(ctx, created.RoomID)
	require.Error(t, err)
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

	resp, err := svc.Notifications(ctx, "viewer-1", 1, 10, "")
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

	resp, err := svc.Notifications(ctx, "viewer-1", 1, 10, "")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "Creator Display", resp.Items[0].ActorName)
	require.Equal(t, "/uploads/current.png", resp.Items[0].ActorAvatar)
}

func TestAppointmentTextPolicyAppliesToCreateUpdateAndStart(t *testing.T) {
	ctx := context.Background()
	svc, _ := newAppointmentServiceTestDeps(t)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	policy := &substringTextPolicy{word: "forbidden"}
	svc.SetTextPolicy(policy)

	payload := AppointmentPayload{
		ScheduledAt: now.Add(10 * time.Minute),
		Title:       "Forbidden title",
		Description: "A scheduled live",
		Category:    "Gaming",
		Cover:       "/uploads/gaming.jpg",
		ChannelName: "Creator Channel",
	}
	_, err := svc.Create(ctx, "owner-policy", payload)
	requireBlockedWord(t, err)

	payload.Title = "Clean title"
	payload.Description = "forbidden description"
	_, err = svc.Create(ctx, "owner-policy", payload)
	requireBlockedWord(t, err)

	payload.Description = "Clean description"
	created, err := svc.Create(ctx, "owner-policy", payload)
	require.NoError(t, err)

	payload.Description = "now it is FORBIDDEN"
	_, err = svc.Update(ctx, "owner-policy", created.ID, payload)
	requireBlockedWord(t, err)

	// A word blocked after scheduling still stops the appointment from going live.
	policy.word = "clean"
	_, err = svc.Start(ctx, "owner-policy", created.ID)
	requireBlockedWord(t, err)
	appt, err := svc.appointments.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, model.AppointmentScheduled, appt.Status)
}

func newAppointmentTestPayload(scheduledAt time.Time) AppointmentPayload {
	return AppointmentPayload{
		ScheduledAt: scheduledAt,
		Title:       "Big show",
		Description: "A scheduled live",
		Category:    "Gaming",
		Cover:       "/uploads/gaming.jpg",
		ChannelName: "Creator Channel",
	}
}

func appointmentNotificationUsers(t *testing.T, db *gorm.DB, kind string) []string {
	t.Helper()
	var users []string
	require.NoError(t, db.Model(&model.Notification{}).Where("type = ?", kind).Order("user_id").Pluck("user_id", &users).Error)
	return users
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr), "expected %s error, got %v", reason, err)
	require.Equal(t, reason, appErr.Reason)
}

// Blocked users can neither reserve a creator's appointment nor keep getting
// reminders through a reservation made before the block.
func TestAppointmentBlocksStopReservationsAndReminders(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)
	messages := repo.NewMessageRepo(db)
	require.NoError(t, messages.AutoMigrate())
	svc.SetBlockChecker(NewMessageService(messages, svc.rooms, nil))
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(ctx, "owner-block", newAppointmentTestPayload(now.Add(2*time.Hour)))
	require.NoError(t, err)
	_, err = svc.Reserve(ctx, "fan-1", created.ID)
	require.NoError(t, err)
	_, err = svc.Reserve(ctx, "fan-2", created.ID)
	require.NoError(t, err)

	require.NoError(t, messages.UpsertBlock(ctx, "owner-block", "fan-1", "user", "", now))
	require.NoError(t, messages.UpsertBlock(ctx, "fan-3", "owner-block", "creator", "", now))
	_, err = svc.Reserve(ctx, "fan-3", created.ID)
	requireReason(t, err, "channel_blocked")

	now = now.Add(2*time.Hour - 5*time.Minute)
	require.NoError(t, svc.ProcessDue(ctx))
	require.Equal(t, []string{"fan-2"}, appointmentNotificationUsers(t, db, "appointment_reminder"))
}

func reserveMany(t *testing.T, db *gorm.DB, appointmentID string, n int) {
	t.Helper()
	rows := make([]model.AppointmentReservation, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, model.AppointmentReservation{AppointmentID: appointmentID, UserID: fmt.Sprintf("watcher-%05d", i), CreatedAt: time.Now()})
	}
	require.NoError(t, db.CreateInBatches(&rows, 500).Error)
}

// A reminder for an audience spanning several watcher pages reaches everyone
// and is marked sent. SQLite caps a statement at 32,766 placeholders, so the
// old single INSERT failed here just as it did past 65,535 on MySQL.
func TestAppointmentReminderReachesLargeAudience(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	created, err := svc.Create(ctx, "owner-popular", newAppointmentTestPayload(now.Add(2*time.Hour)))
	require.NoError(t, err)
	audience := 2*watcherPageSize + 600
	reserveMany(t, db, created.ID, audience)

	now = now.Add(2*time.Hour - 5*time.Minute)
	require.NoError(t, svc.ProcessDue(ctx))
	require.Len(t, appointmentNotificationUsers(t, db, "appointment_reminder"), audience)
	appt, err := svc.appointments.Get(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, appt.ReminderSentAt)
}

// failingBlockChecker errors for any lookup involving failOwner.
type failingBlockChecker struct{ failOwner string }

func (c *failingBlockChecker) BlocksInteraction(_ context.Context, viewerID, creatorID string) (bool, error) {
	if c.failOwner != "" && (viewerID == c.failOwner || creatorID == c.failOwner) {
		return false, errors.New("block lookup failed")
	}
	return false, nil
}

func (c *failingBlockChecker) CreatorBlocks(context.Context, string, string) (bool, error) {
	return false, nil
}

// One appointment whose reminder fails must not stop the scheduler from
// reminding later appointments; the failed one is retried on the next run.
func TestAppointmentReminderFailureDoesNotStallLaterAppointments(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)
	checker := &failingBlockChecker{}
	svc.SetBlockChecker(checker)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	bad, err := svc.Create(ctx, "owner-bad", newAppointmentTestPayload(now.Add(2*time.Hour)))
	require.NoError(t, err)
	good, err := svc.Create(ctx, "owner-good", newAppointmentTestPayload(now.Add(2*time.Hour+3*time.Minute)))
	require.NoError(t, err)
	_, err = svc.Reserve(ctx, "fan-bad", bad.ID)
	require.NoError(t, err)
	_, err = svc.Reserve(ctx, "fan-good", good.ID)
	require.NoError(t, err)

	checker.failOwner = "owner-bad"
	now = now.Add(2*time.Hour - 5*time.Minute)
	require.NoError(t, svc.ProcessDue(ctx))
	require.Equal(t, []string{"fan-good"}, appointmentNotificationUsers(t, db, "appointment_reminder"))
	appt, err := svc.appointments.Get(ctx, bad.ID)
	require.NoError(t, err)
	require.Nil(t, appt.ReminderSentAt)
	appt, err = svc.appointments.Get(ctx, good.ID)
	require.NoError(t, err)
	require.NotNil(t, appt.ReminderSentAt)

	checker.failOwner = ""
	now = now.Add(time.Minute)
	require.NoError(t, svc.ProcessDue(ctx))
	require.Equal(t, []string{"fan-bad", "fan-good"}, appointmentNotificationUsers(t, db, "appointment_reminder"))
	appt, err = svc.appointments.Get(ctx, bad.ID)
	require.NoError(t, err)
	require.NotNil(t, appt.ReminderSentAt)
}

// A start notification that failed when the appointment went live is resent
// by the scheduler instead of being lost.
func TestAppointmentStartNotificationIsRetried(t *testing.T) {
	ctx := context.Background()
	svc, db := newAppointmentServiceTestDeps(t)
	now := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	created, err := svc.Create(ctx, "owner-live", newAppointmentTestPayload(now.Add(20*time.Minute)))
	require.NoError(t, err)
	_, err = svc.Reserve(ctx, "fan-1", created.ID)
	require.NoError(t, err)
	now = now.Add(15 * time.Minute)
	require.NoError(t, svc.appointments.MarkLive(ctx, created.ID, now))

	now = now.Add(time.Minute)
	require.NoError(t, svc.ProcessDue(ctx))
	require.Equal(t, []string{"fan-1"}, appointmentNotificationUsers(t, db, "appointment_started"))
	appt, err := svc.appointments.Get(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, appt.StartNotifiedAt)
}
