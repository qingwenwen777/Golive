package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var ErrAppointmentNotFound = errors.New("appointment not found")
var ErrAppointmentNotCancelable = errors.New("appointment cannot be cancelled")

const appointmentWindow = time.Hour

type AppointmentRepo struct {
	db *gorm.DB
}

func NewAppointmentRepo(db *gorm.DB) *AppointmentRepo { return &AppointmentRepo{db: db} }

func (r *AppointmentRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.LiveAppointment{},
		&model.AppointmentReservation{},
		&model.Notification{},
	)
}

func (r *AppointmentRepo) ActiveCount(ctx context.Context, ownerID string, now time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.LiveAppointment{}).
		Where("owner_id = ? AND status = ? AND scheduled_at >= ?", ownerID, model.AppointmentScheduled, now.Add(-30*time.Minute)).
		Count(&count).Error
	return count, err
}

func (r *AppointmentRepo) HasOverlappingScheduled(ctx context.Context, ownerID, excludeID string, scheduledAt time.Time) (bool, error) {
	tx := r.db.WithContext(ctx).Model(&model.LiveAppointment{}).
		Where("owner_id = ? AND status = ?", ownerID, model.AppointmentScheduled).
		Where("scheduled_at > ? AND scheduled_at < ?", scheduledAt.Add(-appointmentWindow), scheduledAt.Add(appointmentWindow))
	if excludeID != "" {
		tx = tx.Where("id <> ?", excludeID)
	}
	var count int64
	if err := tx.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AppointmentRepo) Create(ctx context.Context, appt *model.LiveAppointment, room *model.Room) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(room).Error; err != nil {
			return err
		}
		return tx.Create(appt).Error
	})
}

func (r *AppointmentRepo) UpdateScheduled(ctx context.Context, appt *model.LiveAppointment, room *model.Room) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.LiveAppointment{}).Where("id = ? AND owner_id = ? AND status = ?", appt.ID, appt.OwnerID, model.AppointmentScheduled).
			Updates(map[string]any{
				"title":        appt.Title,
				"description":  appt.Description,
				"cover":        appt.Cover,
				"scheduled_at": appt.ScheduledAt,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Room{}).Where("id = ?", room.ID).
			Updates(map[string]any{
				"title":       room.Title,
				"description": room.Description,
				"cover":       room.Cover,
				"started_at":  room.StartedAt,
			}).Error
	})
}

func (r *AppointmentRepo) Get(ctx context.Context, id string) (*model.LiveAppointment, error) {
	var appt model.LiveAppointment
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&appt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppointmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &appt, nil
}

func (r *AppointmentRepo) GetByRoomID(ctx context.Context, roomID string) (*model.LiveAppointment, error) {
	var appt model.LiveAppointment
	err := r.db.WithContext(ctx).Where("room_id = ?", roomID).Take(&appt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppointmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &appt, nil
}

func (r *AppointmentRepo) OwnerAppointment(ctx context.Context, ownerID, id string) (*model.LiveAppointment, error) {
	var appt model.LiveAppointment
	err := r.db.WithContext(ctx).Where("owner_id = ? AND id = ?", ownerID, id).Take(&appt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppointmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &appt, nil
}

func (r *AppointmentRepo) ListOwner(ctx context.Context, ownerID string, page, size int) ([]model.LiveAppointment, int64, error) {
	page, size = normalizePageSize(page, size)
	tx := r.db.WithContext(ctx).Model(&model.LiveAppointment{}).Where("owner_id = ?", ownerID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LiveAppointment
	err := tx.Order("scheduled_at DESC, created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AppointmentRepo) ListPublicByOwner(ctx context.Context, ownerID string, now time.Time, page, size int) ([]model.LiveAppointment, int64, error) {
	page, size = normalizePageSize(page, size)
	tx := publicAppointmentQuery(r.db.WithContext(ctx), now).Where("owner_id = ?", ownerID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LiveAppointment
	err := tx.Order("scheduled_at ASC, created_at ASC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AppointmentRepo) ListPublicByOwners(ctx context.Context, ownerIDs []string, now time.Time, page, size int) ([]model.LiveAppointment, int64, error) {
	page, size = normalizePageSize(page, size)
	if len(ownerIDs) == 0 {
		return []model.LiveAppointment{}, 0, nil
	}
	tx := publicAppointmentQuery(r.db.WithContext(ctx), now).Where("owner_id IN ?", ownerIDs)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LiveAppointment
	err := tx.Order("scheduled_at ASC, created_at ASC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AppointmentRepo) ListPublicUpcoming(ctx context.Context, now, until time.Time, page, size int) ([]model.LiveAppointment, int64, error) {
	page, size = normalizePageSize(page, size)
	tx := publicAppointmentQuery(r.db.WithContext(ctx), now).Where("scheduled_at < ?", until)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LiveAppointment
	err := tx.Order("scheduled_at ASC, created_at ASC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AppointmentRepo) ListReservedByUser(ctx context.Context, userID string, now time.Time, page, size int) ([]model.LiveAppointment, int64, error) {
	page, size = normalizePageSize(page, size)
	tx := r.db.WithContext(ctx).Model(&model.LiveAppointment{}).
		Joins("JOIN appointment_reservations ar ON ar.appointment_id = live_appointments.id").
		Where("ar.user_id = ?", userID).
		Where("live_appointments.status = ? AND live_appointments.scheduled_at >= ?", model.AppointmentScheduled, now.Add(-30*time.Minute))
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.LiveAppointment
	err := tx.Order("live_appointments.scheduled_at ASC, live_appointments.created_at ASC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *AppointmentRepo) RoomMap(ctx context.Context, roomIDs []string) (map[string]model.Room, error) {
	out := make(map[string]model.Room, len(roomIDs))
	if len(roomIDs) == 0 {
		return out, nil
	}
	var rooms []model.Room
	if err := r.db.WithContext(ctx).Where("id IN ?", roomIDs).Find(&rooms).Error; err != nil {
		return nil, err
	}
	for _, room := range rooms {
		out[room.ID] = room
	}
	return out, nil
}

func (r *AppointmentRepo) ReservationCounts(ctx context.Context, appointmentIDs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(appointmentIDs))
	if len(appointmentIDs) == 0 {
		return out, nil
	}
	type row struct {
		AppointmentID string
		Count         int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&model.AppointmentReservation{}).
		Select("appointment_id, COUNT(*) AS count").
		Where("appointment_id IN ?", appointmentIDs).
		Group("appointment_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AppointmentID] = row.Count
	}
	return out, nil
}

func (r *AppointmentRepo) ReservedSet(ctx context.Context, userID string, appointmentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(appointmentIDs))
	if userID == "" || len(appointmentIDs) == 0 {
		return out, nil
	}
	var rows []model.AppointmentReservation
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND appointment_id IN ?", userID, appointmentIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.AppointmentID] = true
	}
	return out, nil
}

func (r *AppointmentRepo) Reserve(ctx context.Context, appointmentID, userID string) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.AppointmentReservation{
		AppointmentID: appointmentID,
		UserID:        userID,
		CreatedAt:     time.Now().UTC(),
	}).Error
}

func (r *AppointmentRepo) Unreserve(ctx context.Context, appointmentID, userID string) error {
	return r.db.WithContext(ctx).Where("appointment_id = ? AND user_id = ?", appointmentID, userID).
		Delete(&model.AppointmentReservation{}).Error
}

func (r *AppointmentRepo) WatcherIDs(ctx context.Context, appointmentID string) ([]string, error) {
	var rows []model.AppointmentReservation
	if err := r.db.WithContext(ctx).Where("appointment_id = ?", appointmentID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.UserID)
	}
	return out, nil
}

func (r *AppointmentRepo) Cancel(ctx context.Context, ownerID, appointmentID string, canceledAt time.Time) (*model.LiveAppointment, error) {
	var appt model.LiveAppointment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner_id = ? AND id = ?", ownerID, appointmentID).Take(&appt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAppointmentNotFound
			}
			return err
		}
		if appt.Status != model.AppointmentScheduled {
			return ErrAppointmentNotCancelable
		}
		if err := tx.Model(&model.LiveAppointment{}).Where("id = ?", appointmentID).Updates(map[string]any{
			"status":   model.AppointmentCanceled,
			"ended_at": canceledAt,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Room{}).Where("id = ?", appt.RoomID).Updates(map[string]any{
			"status":   model.StatusCanceled,
			"ended_at": canceledAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	appt.Status = model.AppointmentCanceled
	appt.EndedAt = &canceledAt
	return &appt, nil
}

func (r *AppointmentRepo) MarkLive(ctx context.Context, appointmentID string, startedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.LiveAppointment{}).
		Where("id = ? AND status = ?", appointmentID, model.AppointmentScheduled).
		Updates(map[string]any{
			"status":     model.AppointmentLive,
			"started_at": startedAt,
		}).Error
}

func (r *AppointmentRepo) MarkCompletedByRoom(ctx context.Context, roomID string, endedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.LiveAppointment{}).
		Where("room_id = ? AND status = ?", roomID, model.AppointmentLive).
		Updates(map[string]any{
			"status":   model.AppointmentCompleted,
			"ended_at": endedAt,
		}).Error
}

func (r *AppointmentRepo) ExpireDue(ctx context.Context, now time.Time) ([]model.LiveAppointment, error) {
	var items []model.LiveAppointment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = ? AND scheduled_at < ?", model.AppointmentScheduled, now.Add(-30*time.Minute)).
			Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		ids := make([]string, 0, len(items))
		roomIDs := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
			roomIDs = append(roomIDs, item.RoomID)
		}
		if err := tx.Model(&model.LiveAppointment{}).Where("id IN ?", ids).Updates(map[string]any{
			"status":   model.AppointmentExpired,
			"ended_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Room{}).Where("id IN ?", roomIDs).Updates(map[string]any{
			"status":   model.StatusExpired,
			"ended_at": now,
		}).Error
	})
	return items, err
}

func (r *AppointmentRepo) ReminderDue(ctx context.Context, now time.Time) ([]model.LiveAppointment, error) {
	var items []model.LiveAppointment
	err := r.db.WithContext(ctx).
		Where("status = ? AND reminder_sent_at IS NULL AND scheduled_at <= ? AND scheduled_at >= ?",
			model.AppointmentScheduled, now.Add(10*time.Minute), now.Add(-30*time.Minute)).
		Order("scheduled_at ASC").
		Find(&items).Error
	return items, err
}

func (r *AppointmentRepo) MarkReminderSent(ctx context.Context, appointmentID string, sentAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.LiveAppointment{}).Where("id = ?", appointmentID).
		Update("reminder_sent_at", sentAt).Error
}

func (r *AppointmentRepo) MarkStartNotified(ctx context.Context, appointmentID string, sentAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.LiveAppointment{}).Where("id = ?", appointmentID).
		Update("start_notified_at", sentAt).Error
}

func (r *AppointmentRepo) DueStartWindowForOwner(ctx context.Context, ownerID string, now time.Time) (*model.LiveAppointment, error) {
	var appt model.LiveAppointment
	err := r.db.WithContext(ctx).
		Where("owner_id = ? AND status = ? AND scheduled_at <= ? AND scheduled_at >= ?",
			ownerID, model.AppointmentScheduled, now.Add(30*time.Minute), now.Add(-30*time.Minute)).
		Order("scheduled_at ASC").
		Take(&appt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAppointmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &appt, nil
}

func (r *AppointmentRepo) CreateNotifications(ctx context.Context, notifications []model.Notification) error {
	if len(notifications) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&notifications).Error
}

func (r *AppointmentRepo) ListNotifications(ctx context.Context, userID string, page, size int) ([]model.Notification, int64, int64, error) {
	page, size = normalizePageSize(page, size)
	tx := r.db.WithContext(ctx).Model(&model.Notification{}).Where("user_id = ?", userID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}
	var unread int64
	if err := tx.Where("read_at IS NULL").Count(&unread).Error; err != nil {
		return nil, 0, 0, err
	}
	var items []model.Notification
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, unread, err
}

func (r *AppointmentRepo) MarkNotificationRead(ctx context.Context, userID, id string, readAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("user_id = ? AND id = ? AND read_at IS NULL", userID, id).
		Update("read_at", readAt).Error
}

func (r *AppointmentRepo) MarkAllNotificationsRead(ctx context.Context, userID string, readAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", readAt).Error
}

func publicAppointmentQuery(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Model(&model.LiveAppointment{}).
		Where("status = ? AND scheduled_at >= ?", model.AppointmentScheduled, now.Add(-30*time.Minute))
}

func normalizePageSize(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
