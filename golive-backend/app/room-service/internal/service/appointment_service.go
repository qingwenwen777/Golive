package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/logger"
)

const (
	maxActiveAppointments = 5
	startLead             = 30 * time.Minute
	startGrace            = 30 * time.Minute
)

type AppointmentService struct {
	appointments *repo.AppointmentRepo
	rooms        *repo.RoomRepo
	social       *repo.SocialRepo
	live         *LiveService
	now          func() time.Time
}

type AppointmentPayload struct {
	ScheduledAt time.Time
	Title       string
	Description string
	Cover       string
	ChannelName string
	Avatar      string
}

type AppointmentDTO struct {
	ID               string `json:"id"`
	RoomID           string `json:"roomId"`
	OwnerID          string `json:"ownerId"`
	ChannelID        string `json:"channelId"`
	Channel          string `json:"channel"`
	Avatar           string `json:"avatar"`
	Verified         bool   `json:"verified"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	Cover            string `json:"cover"`
	ScheduledAt      string `json:"scheduledAt"`
	Status           string `json:"status"`
	StartedAt        string `json:"startedAt,omitempty"`
	EndedAt          string `json:"endedAt,omitempty"`
	ReservationCount int64  `json:"reservationCount"`
	Reserved         bool   `json:"reserved"`
	CanStart         bool   `json:"canStart"`
}

type AppointmentListResp struct {
	Items []AppointmentDTO `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

type NotificationDTO struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Link      string `json:"link,omitempty"`
	ReadAt    string `json:"readAt,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type NotificationListResp struct {
	Items  []NotificationDTO `json:"items"`
	Total  int64             `json:"total"`
	Unread int64             `json:"unread"`
	Page   int               `json:"page"`
	Size   int               `json:"size"`
}

func NewAppointmentService(appointments *repo.AppointmentRepo, rooms *repo.RoomRepo, social *repo.SocialRepo, live *LiveService) *AppointmentService {
	return &AppointmentService{
		appointments: appointments,
		rooms:        rooms,
		social:       social,
		live:         live,
		now:          time.Now,
	}
}

func (s *AppointmentService) Create(ctx context.Context, ownerID string, payload AppointmentPayload) (*AppointmentDTO, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	title, description, cover, err := cleanAppointmentPayload(payload)
	if err != nil {
		return nil, err
	}
	if !payload.ScheduledAt.After(now) {
		return nil, errcode.New(400, "scheduled time must be in the future")
	}
	count, err := s.appointments.ActiveCount(ctx, ownerID, now)
	if err != nil {
		return nil, err
	}
	if count >= maxActiveAppointments {
		return nil, errcode.New(409, "a creator can have at most five active appointments")
	}
	overlap, err := s.appointments.HasOverlappingScheduled(ctx, ownerID, "", payload.ScheduledAt)
	if err != nil {
		return nil, err
	}
	if overlap {
		return nil, errcode.New(409, "appointment time overlaps with another appointment")
	}

	roomID := "appt-" + ownerID + "-" + strconv.FormatInt(now.UnixNano(), 36)
	channelID := "ch-" + ownerID
	channelName := cleanDisplayName(payload.ChannelName, ownerID)
	room := &model.Room{
		ID:          roomID,
		Title:       title,
		Description: description,
		Category:    "Scheduled",
		Cover:       cover,
		Channel:     channelName,
		ChannelID:   channelID,
		Verified:    false,
		Avatar:      cleanAvatar(payload.Avatar, channelName),
		Viewers:     0,
		PeakViewers: 0,
		StartedAt:   payload.ScheduledAt,
		Status:      model.StatusScheduled,
		OwnerID:     ownerID,
	}
	appt := &model.LiveAppointment{
		ID:          roomID,
		OwnerID:     ownerID,
		RoomID:      roomID,
		Title:       title,
		Description: description,
		Cover:       cover,
		ScheduledAt: payload.ScheduledAt,
		Status:      model.AppointmentScheduled,
	}
	if err := s.appointments.Create(ctx, appt, room); err != nil {
		return nil, err
	}
	return s.dto(ctx, *appt, room, ownerID)
}

func (s *AppointmentService) Update(ctx context.Context, ownerID, id string, payload AppointmentPayload) (*AppointmentDTO, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	appt, err := s.appointments.OwnerAppointment(ctx, ownerID, id)
	if err != nil {
		if errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, errcode.New(404, "appointment not found")
		}
		return nil, err
	}
	if appt.Status != model.AppointmentScheduled {
		return nil, errcode.New(409, "only scheduled appointments can be updated")
	}
	title, description, cover, err := cleanAppointmentPayload(payload)
	if err != nil {
		return nil, err
	}
	if !payload.ScheduledAt.After(now) {
		return nil, errcode.New(400, "scheduled time must be in the future")
	}
	overlap, err := s.appointments.HasOverlappingScheduled(ctx, ownerID, id, payload.ScheduledAt)
	if err != nil {
		return nil, err
	}
	if overlap {
		return nil, errcode.New(409, "appointment time overlaps with another appointment")
	}
	appt.Title = title
	appt.Description = description
	appt.Cover = cover
	appt.ScheduledAt = payload.ScheduledAt
	room := &model.Room{
		ID:          appt.RoomID,
		Title:       title,
		Description: description,
		Cover:       cover,
		StartedAt:   payload.ScheduledAt,
	}
	if err := s.appointments.UpdateScheduled(ctx, appt, room); err != nil {
		return nil, err
	}
	fullRoom, _ := s.rooms.GetByID(ctx, appt.RoomID)
	return s.dto(ctx, *appt, fullRoom, ownerID)
}

func (s *AppointmentService) Cancel(ctx context.Context, ownerID, id string) (*AppointmentDTO, error) {
	now := s.now()
	appt, err := s.appointments.Cancel(ctx, ownerID, id, now)
	if err != nil {
		if errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, errcode.New(404, "appointment not found")
		}
		if errors.Is(err, repo.ErrAppointmentNotCancelable) {
			return nil, errcode.New(409, "appointment cannot be cancelled")
		}
		return nil, err
	}
	_ = s.live.broadcastEnded(ctx, appt.RoomID, now)
	room, _ := s.rooms.GetByID(ctx, appt.RoomID)
	return s.dto(ctx, *appt, room, ownerID)
}

func (s *AppointmentService) ListOwner(ctx context.Context, ownerID string, page, size int) (*AppointmentListResp, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	items, total, err := s.appointments.ListOwner(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	return s.listResp(ctx, items, ownerID, page, size, total)
}

func (s *AppointmentService) ListChannel(ctx context.Context, channelKey, viewerID string, page, size int) (*AppointmentListResp, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	ownerID, err := s.rooms.ResolveOwnerID(ctx, channelKey)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return &AppointmentListResp{Items: []AppointmentDTO{}, Total: 0, Page: page, Size: size}, nil
		}
		return nil, err
	}
	items, total, err := s.appointments.ListPublicByOwner(ctx, ownerID, s.now(), page, size)
	if err != nil {
		return nil, err
	}
	return s.listResp(ctx, items, viewerID, page, size, total)
}

func (s *AppointmentService) ListSubscriptionAppointments(ctx context.Context, viewerID string, page, size int) (*AppointmentListResp, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	channelIDs, err := s.social.Following(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	ownerIDs := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		ownerID := strings.TrimPrefix(channelID, "ch-")
		if ownerID != "" {
			ownerIDs = append(ownerIDs, ownerID)
		}
	}
	items, total, err := s.appointments.ListPublicByOwners(ctx, ownerIDs, s.now(), page, size)
	if err != nil {
		return nil, err
	}
	return s.listResp(ctx, items, viewerID, page, size, total)
}

func (s *AppointmentService) ListReserved(ctx context.Context, viewerID string, page, size int) (*AppointmentListResp, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	items, total, err := s.appointments.ListReservedByUser(ctx, viewerID, s.now(), page, size)
	if err != nil {
		return nil, err
	}
	return s.listResp(ctx, items, viewerID, page, size, total)
}

func (s *AppointmentService) Reserve(ctx context.Context, viewerID, id string) (*AppointmentDTO, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	appt, err := s.appointments.Get(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, errcode.New(404, "appointment not found")
		}
		return nil, err
	}
	if !s.isPubliclyActive(*appt) {
		return nil, errcode.New(409, "appointment is no longer available")
	}
	if err := s.appointments.Reserve(ctx, id, viewerID); err != nil {
		return nil, err
	}
	room, _ := s.rooms.GetByID(ctx, appt.RoomID)
	return s.dto(ctx, *appt, room, viewerID)
}

func (s *AppointmentService) Unreserve(ctx context.Context, viewerID, id string) (*AppointmentDTO, error) {
	if err := s.appointments.Unreserve(ctx, id, viewerID); err != nil {
		return nil, err
	}
	appt, err := s.appointments.Get(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, errcode.New(404, "appointment not found")
		}
		return nil, err
	}
	room, _ := s.rooms.GetByID(ctx, appt.RoomID)
	return s.dto(ctx, *appt, room, viewerID)
}

func (s *AppointmentService) Start(ctx context.Context, ownerID, id string) (*model.Stream, error) {
	if err := s.cleanupExpired(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	appt, err := s.appointments.OwnerAppointment(ctx, ownerID, id)
	if err != nil {
		if errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, errcode.New(404, "appointment not found")
		}
		return nil, err
	}
	if appt.Status != model.AppointmentScheduled {
		return nil, errcode.New(409, "appointment cannot be started")
	}
	if now.Before(appt.ScheduledAt.Add(-startLead)) {
		return nil, errcode.New(409, "appointment can be started at most 30 minutes early")
	}
	if now.After(appt.ScheduledAt.Add(startGrace)) {
		if err := s.cleanupExpired(ctx); err != nil {
			return nil, err
		}
		return nil, errcode.New(409, "appointment has expired")
	}
	room, err := s.rooms.GetByID(ctx, appt.RoomID)
	if err != nil {
		return nil, err
	}
	if active, err := s.rooms.ActiveByOwner(ctx, ownerID); err == nil && active.ID != room.ID {
		if err := s.live.endRoom(ctx, active, now); err != nil {
			return nil, err
		}
		if active.StreamKey != "" {
			_ = s.live.live.Delete(ctx, active.StreamKey)
			_ = s.live.live.DeletePublishSession(ctx, active.StreamKey)
		}
		_ = s.live.broadcastEnded(ctx, active.ID, now)
	} else if err != nil && !errors.Is(err, repo.ErrRoomNotFound) {
		return nil, err
	}

	streamKey := s.live.generateKey(room.ID, ownerID, now)
	room.Title = appt.Title
	room.Description = appt.Description
	room.Cover = appt.Cover
	room.StartedAt = now
	room.Status = model.StatusPublishing
	room.StreamKey = streamKey
	room.EndedAt = nil
	if err := s.rooms.Upsert(ctx, room); err != nil {
		return nil, err
	}
	if err := s.live.live.Save(ctx, streamKey, room.ID, s.live.keyTTL); err != nil {
		return nil, err
	}
	if s.live.moderation != nil {
		if err := s.live.moderation.SyncRoomModerators(ctx, room.ID, ownerID); err != nil {
			return nil, err
		}
	}
	if err := s.appointments.MarkLive(ctx, appt.ID, now); err != nil {
		return nil, err
	}
	if err := s.notifyStart(ctx, *appt); err != nil {
		logger.L().Warn("notify appointment start", zap.Error(err), zap.String("appointment", appt.ID))
	}
	st := room.ToStream(now)
	st.StreamKey = streamKey
	return &st, nil
}

func (s *AppointmentService) Notifications(ctx context.Context, userID string, page, size int) (*NotificationListResp, error) {
	items, total, unread, err := s.appointments.ListNotifications(ctx, userID, page, size)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationDTO, 0, len(items))
	for _, item := range items {
		dto := NotificationDTO{
			ID:        item.ID,
			Type:      item.Type,
			Title:     item.Title,
			Body:      item.Body,
			Link:      item.Link,
			CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
		}
		if item.ReadAt != nil {
			dto.ReadAt = item.ReadAt.UTC().Format(time.RFC3339)
		}
		out = append(out, dto)
	}
	return &NotificationListResp{Items: out, Total: total, Unread: unread, Page: page, Size: size}, nil
}

func (s *AppointmentService) MarkNotificationRead(ctx context.Context, userID, id string) error {
	return s.appointments.MarkNotificationRead(ctx, userID, id, s.now())
}

func (s *AppointmentService) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	return s.appointments.MarkAllNotificationsRead(ctx, userID, s.now())
}

func (s *AppointmentService) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	if err := s.ProcessDue(ctx); err != nil {
		logger.L().Warn("appointment scheduler initial run", zap.Error(err))
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ProcessDue(ctx); err != nil {
				logger.L().Warn("appointment scheduler run", zap.Error(err))
			}
		}
	}
}

func (s *AppointmentService) ProcessDue(ctx context.Context) error {
	if err := s.sendReminders(ctx); err != nil {
		return err
	}
	return s.cleanupExpired(ctx)
}

func (s *AppointmentService) listResp(ctx context.Context, items []model.LiveAppointment, viewerID string, page, size int, total int64) (*AppointmentListResp, error) {
	roomIDs := make([]string, 0, len(items))
	appointmentIDs := make([]string, 0, len(items))
	for _, item := range items {
		roomIDs = append(roomIDs, item.RoomID)
		appointmentIDs = append(appointmentIDs, item.ID)
	}
	rooms, err := s.appointments.RoomMap(ctx, roomIDs)
	if err != nil {
		return nil, err
	}
	counts, err := s.appointments.ReservationCounts(ctx, appointmentIDs)
	if err != nil {
		return nil, err
	}
	reserved, err := s.appointments.ReservedSet(ctx, viewerID, appointmentIDs)
	if err != nil {
		return nil, err
	}
	out := make([]AppointmentDTO, 0, len(items))
	now := s.now()
	for _, item := range items {
		room := rooms[item.RoomID]
		out = append(out, s.dtoFrom(item, &room, counts[item.ID], reserved[item.ID], now))
	}
	return &AppointmentListResp{Items: out, Total: total, Page: page, Size: size}, nil
}

func (s *AppointmentService) dto(ctx context.Context, appt model.LiveAppointment, room *model.Room, viewerID string) (*AppointmentDTO, error) {
	counts, err := s.appointments.ReservationCounts(ctx, []string{appt.ID})
	if err != nil {
		return nil, err
	}
	reserved, err := s.appointments.ReservedSet(ctx, viewerID, []string{appt.ID})
	if err != nil {
		return nil, err
	}
	return ptr(s.dtoFrom(appt, room, counts[appt.ID], reserved[appt.ID], s.now())), nil
}

func (s *AppointmentService) dtoFrom(appt model.LiveAppointment, room *model.Room, count int64, reserved bool, now time.Time) AppointmentDTO {
	dto := AppointmentDTO{
		ID:               appt.ID,
		RoomID:           appt.RoomID,
		OwnerID:          appt.OwnerID,
		ChannelID:        "ch-" + appt.OwnerID,
		Title:            appt.Title,
		Description:      appt.Description,
		Cover:            appt.Cover,
		ScheduledAt:      appt.ScheduledAt.UTC().Format(time.RFC3339),
		Status:           appt.Status,
		ReservationCount: count,
		Reserved:         reserved,
		CanStart:         appt.Status == model.AppointmentScheduled && !now.Before(appt.ScheduledAt.Add(-startLead)) && !now.After(appt.ScheduledAt.Add(startGrace)),
	}
	if appt.StartedAt != nil {
		dto.StartedAt = appt.StartedAt.UTC().Format(time.RFC3339)
	}
	if appt.EndedAt != nil {
		dto.EndedAt = appt.EndedAt.UTC().Format(time.RFC3339)
	}
	if room != nil {
		dto.ChannelID = room.ChannelID
		dto.Channel = room.Channel
		dto.Avatar = room.Avatar
		dto.Verified = room.Verified
		if dto.Cover == "" {
			dto.Cover = room.Cover
		}
	}
	return dto
}

func (s *AppointmentService) isPubliclyActive(appt model.LiveAppointment) bool {
	return appt.Status == model.AppointmentScheduled && !appt.ScheduledAt.Before(s.now().Add(-startGrace))
}

func (s *AppointmentService) cleanupExpired(ctx context.Context) error {
	now := s.now()
	expired, err := s.appointments.ExpireDue(ctx, now)
	if err != nil {
		return err
	}
	for _, appt := range expired {
		_ = s.live.broadcastEnded(ctx, appt.RoomID, now)
	}
	return nil
}

func (s *AppointmentService) sendReminders(ctx context.Context) error {
	now := s.now()
	due, err := s.appointments.ReminderDue(ctx, now)
	if err != nil {
		return err
	}
	for _, appt := range due {
		if err := s.notifyWatchers(ctx, appt, "appointment_reminder"); err != nil {
			return err
		}
		if err := s.appointments.MarkReminderSent(ctx, appt.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *AppointmentService) notifyStart(ctx context.Context, appt model.LiveAppointment) error {
	if err := s.notifyWatchers(ctx, appt, "appointment_started"); err != nil {
		return err
	}
	return s.appointments.MarkStartNotified(ctx, appt.ID, s.now())
}

func (s *AppointmentService) notifyWatchers(ctx context.Context, appt model.LiveAppointment, kind string) error {
	watchers, err := s.appointments.WatcherIDs(ctx, appt.ID)
	if err != nil {
		return err
	}
	if len(watchers) == 0 {
		return nil
	}
	now := s.now()
	title := "预约直播即将开始"
	body := appt.Title
	if kind == "appointment_started" {
		title = "预约直播已开播"
		body = appt.Title
	}
	notifications := make([]model.Notification, 0, len(watchers))
	for _, userID := range watchers {
		notifications = append(notifications, model.Notification{
			ID:        notificationID(kind, appt.ID, userID),
			UserID:    userID,
			Type:      kind,
			Title:     title,
			Body:      body,
			Link:      "/live/" + appt.RoomID,
			CreatedAt: now,
		})
	}
	return s.appointments.CreateNotifications(ctx, notifications)
}

func cleanAppointmentPayload(payload AppointmentPayload) (string, string, string, error) {
	title := trimRunes(strings.TrimSpace(payload.Title), 120)
	if title == "" {
		return "", "", "", errcode.New(400, "title is required")
	}
	description := cleanDescription(payload.Description)
	if description == "" {
		return "", "", "", errcode.New(400, "description is required")
	}
	cover := trimRunes(strings.TrimSpace(payload.Cover), 500)
	if cover == "" {
		return "", "", "", errcode.New(400, "cover is required")
	}
	return title, description, cover, nil
}

func notificationID(kind, appointmentID, userID string) string {
	sum := sha256.Sum256([]byte(kind + ":" + appointmentID + ":" + userID))
	return kind + "-" + hex.EncodeToString(sum[:])[:32]
}

func ptr[T any](v T) *T {
	return &v
}
