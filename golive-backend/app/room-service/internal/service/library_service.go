package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const maxUserLibraryItems = 60

type LibraryStream struct {
	model.Stream
	SavedAt   string `json:"savedAt,omitempty"`
	WatchedAt string `json:"watchedAt,omitempty"`
}

type UserLibraryResp struct {
	Items []LibraryStream `json:"items"`
}

type UserLibraryItemInput struct {
	RoomID    string `json:"roomId"`
	SavedAt   string `json:"savedAt,omitempty"`
	WatchedAt string `json:"watchedAt,omitempty"`
}

type SyncUserLibraryReq struct {
	Items []UserLibraryItemInput `json:"items"`
}

func NormalizeLibraryType(raw string) (string, error) {
	switch strings.ReplaceAll(strings.TrimSpace(raw), "-", "_") {
	case model.LibraryTypeHistory:
		return model.LibraryTypeHistory, nil
	case model.LibraryTypeWatchLater:
		return model.LibraryTypeWatchLater, nil
	case model.LibraryTypeLiked:
		return model.LibraryTypeLiked, nil
	default:
		return "", errcode.New(http.StatusBadRequest, "invalid library type").WithReason("invalid_library_type")
	}
}

func (s *RoomService) ListUserLibrary(ctx context.Context, userID, rawType string) (*UserLibraryResp, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errcode.ErrUnauthorized
	}
	libraryType, err := NormalizeLibraryType(rawType)
	if err != nil {
		return nil, err
	}
	rows, err := s.rooms.ListUserLibraryRooms(ctx, userID, libraryType, maxUserLibraryItems)
	if err != nil {
		return nil, err
	}
	rooms := make([]model.Room, 0, len(rows))
	for _, row := range rows {
		rooms = append(rooms, row.Room)
	}
	streams, err := s.streamsFromRooms(ctx, rooms, s.now(), userID)
	if err != nil {
		return nil, err
	}
	items := make([]LibraryStream, 0, len(rows))
	for i, row := range rows {
		items = append(items, libraryStreamFromRoom(streams[i], row.Item))
	}
	return &UserLibraryResp{Items: items}, nil
}

func (s *RoomService) SaveUserLibraryItem(ctx context.Context, userID, rawType string, input UserLibraryItemInput) (*UserLibraryResp, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errcode.ErrUnauthorized
	}
	libraryType, err := NormalizeLibraryType(rawType)
	if err != nil {
		return nil, err
	}
	room, savedAt, err := s.prepareUserLibraryItem(ctx, userID, libraryType, input)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.rooms.UpsertUserLibraryItem(ctx, &model.UserLibraryItem{
		ID:        userLibraryItemID(userID, libraryType, room.ID),
		UserID:    userID,
		Type:      libraryType,
		RoomID:    room.ID,
		SavedAt:   savedAt,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return nil, err
	}
	if libraryType == model.LibraryTypeLiked && s.social != nil {
		_, _ = s.social.Like(ctx, room.ID, userID)
	}
	return s.ListUserLibrary(ctx, userID, libraryType)
}

func (s *RoomService) SyncUserLibrary(ctx context.Context, userID, rawType string, req SyncUserLibraryReq) (*UserLibraryResp, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errcode.ErrUnauthorized
	}
	libraryType, err := NormalizeLibraryType(rawType)
	if err != nil {
		return nil, err
	}
	if len(req.Items) > maxUserLibraryItems {
		req.Items = req.Items[:maxUserLibraryItems]
	}
	for _, input := range req.Items {
		room, savedAt, err := s.prepareUserLibraryItem(ctx, userID, libraryType, input)
		if errors.Is(err, ErrRoomNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		now := s.now()
		if err := s.rooms.MergeUserLibraryItem(ctx, &model.UserLibraryItem{
			ID:        userLibraryItemID(userID, libraryType, room.ID),
			UserID:    userID,
			Type:      libraryType,
			RoomID:    room.ID,
			SavedAt:   savedAt,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			return nil, err
		}
		if libraryType == model.LibraryTypeLiked && s.social != nil {
			_, _ = s.social.Like(ctx, room.ID, userID)
		}
	}
	return s.ListUserLibrary(ctx, userID, libraryType)
}

func (s *RoomService) RemoveUserLibraryItem(ctx context.Context, userID, rawType, roomID string) (*UserLibraryResp, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errcode.ErrUnauthorized
	}
	libraryType, err := NormalizeLibraryType(rawType)
	if err != nil {
		return nil, err
	}
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, errcode.New(http.StatusBadRequest, "roomId is required").WithReason("room_required")
	}
	if err := s.rooms.RemoveUserLibraryItem(ctx, userID, libraryType, roomID); err != nil {
		return nil, err
	}
	if libraryType == model.LibraryTypeHistory {
		if err := s.rooms.RemoveWatchEvent(ctx, userID, roomID); err != nil {
			return nil, err
		}
	}
	if libraryType == model.LibraryTypeLiked && s.social != nil {
		_, _ = s.social.Unlike(ctx, roomID, userID)
	}
	return s.ListUserLibrary(ctx, userID, libraryType)
}

func (s *RoomService) ClearUserLibrary(ctx context.Context, userID, rawType string) (*UserLibraryResp, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errcode.ErrUnauthorized
	}
	libraryType, err := NormalizeLibraryType(rawType)
	if err != nil {
		return nil, err
	}
	if libraryType == model.LibraryTypeLiked && s.social != nil {
		rows, err := s.rooms.ListUserLibraryRooms(ctx, userID, libraryType, maxUserLibraryItems)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			_, _ = s.social.Unlike(ctx, row.Item.RoomID, userID)
		}
	}
	if err := s.rooms.ClearUserLibrary(ctx, userID, libraryType); err != nil {
		return nil, err
	}
	if libraryType == model.LibraryTypeHistory {
		if err := s.rooms.ClearWatchEvents(ctx, userID); err != nil {
			return nil, err
		}
	}
	return &UserLibraryResp{Items: []LibraryStream{}}, nil
}

func (s *RoomService) saveHistoryLibraryItem(ctx context.Context, userID string, room *model.Room, watchedAt time.Time) error {
	if strings.TrimSpace(userID) == "" || room == nil {
		return nil
	}
	now := s.now()
	return s.rooms.UpsertUserLibraryItem(ctx, &model.UserLibraryItem{
		ID:        userLibraryItemID(userID, model.LibraryTypeHistory, room.ID),
		UserID:    userID,
		Type:      model.LibraryTypeHistory,
		RoomID:    room.ID,
		SavedAt:   watchedAt,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *SocialService) saveLikedLibraryItem(ctx context.Context, userID, roomID string) error {
	if s.rooms == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(roomID) == "" {
		return nil
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil
		}
		return err
	}
	now := time.Now()
	return s.rooms.UpsertUserLibraryItem(ctx, &model.UserLibraryItem{
		ID:        userLibraryItemID(userID, model.LibraryTypeLiked, room.ID),
		UserID:    userID,
		Type:      model.LibraryTypeLiked,
		RoomID:    room.ID,
		SavedAt:   now,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *SocialService) removeLikedLibraryItem(ctx context.Context, userID, roomID string) error {
	if s.rooms == nil {
		return nil
	}
	return s.rooms.RemoveUserLibraryItem(ctx, userID, model.LibraryTypeLiked, roomID)
}

func (s *RoomService) prepareUserLibraryItem(ctx context.Context, userID, libraryType string, input UserLibraryItemInput) (*model.Room, time.Time, error) {
	roomID := strings.TrimSpace(input.RoomID)
	if roomID == "" {
		return nil, time.Time{}, errcode.New(http.StatusBadRequest, "roomId is required").WithReason("room_required")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, time.Time{}, ErrRoomNotFound
		}
		return nil, time.Time{}, err
	}
	blocked, err := s.blocksRoomInteraction(ctx, userID, room.OwnerID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if blocked {
		return nil, time.Time{}, errcode.New(http.StatusForbidden, "blocked from this channel").WithReason("channel_blocked")
	}
	return room, libraryItemTimestamp(libraryType, input, s.now()), nil
}

func libraryStreamFromRoom(stream model.Stream, item model.UserLibraryItem) LibraryStream {
	out := LibraryStream{Stream: stream}
	stamp := item.SavedAt.UTC().Format(time.RFC3339)
	if item.Type == model.LibraryTypeHistory {
		out.WatchedAt = stamp
	} else {
		out.SavedAt = stamp
	}
	return out
}

func libraryItemTimestamp(libraryType string, input UserLibraryItemInput, fallback time.Time) time.Time {
	raw := strings.TrimSpace(input.SavedAt)
	if libraryType == model.LibraryTypeHistory {
		raw = strings.TrimSpace(input.WatchedAt)
		if raw == "" {
			raw = strings.TrimSpace(input.SavedAt)
		}
	}
	if raw == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func userLibraryItemID(userID, libraryType, roomID string) string {
	sum := sha1.Sum([]byte(userID + ":" + libraryType + ":" + roomID))
	return "lib-" + hex.EncodeToString(sum[:])[:32]
}
