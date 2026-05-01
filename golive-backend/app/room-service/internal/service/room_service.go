package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

var ErrRoomNotFound = errcode.New(404, "Not found")

type RoomService struct {
	rooms   *repo.RoomRepo
	social  *repo.SocialRepo
	flvBase string
	now     func() time.Time
}

func NewRoomService(rooms *repo.RoomRepo, flvBase string, social ...*repo.SocialRepo) *RoomService {
	if flvBase == "" {
		flvBase = "http://localhost:8082/live"
	}
	var socialRepo *repo.SocialRepo
	if len(social) > 0 {
		socialRepo = social[0]
	}
	return &RoomService{rooms: rooms, social: socialRepo, flvBase: strings.TrimRight(flvBase, "/"), now: time.Now}
}

// playbackURL builds the public HTTP-FLV URL for a live room. Viewers receive
// this; the raw streamKey stays server-side.
func (s *RoomService) playbackURL(r *model.Room) string {
	if r == nil || r.Status != model.StatusLive || r.StreamKey == "" {
		return ""
	}
	return s.flvBase + "/" + r.StreamKey + ".flv"
}

// NormalizeCategory accepts empty / "all" / "すべて" as no filter; else trimmed input.
func NormalizeCategory(raw string) string {
	c := strings.TrimSpace(raw)
	if c == "" || strings.EqualFold(c, "all") || c == "すべて" {
		return ""
	}
	return c
}

type ListResp struct {
	Items []model.Stream `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

func (s *RoomService) List(ctx context.Context, rawCategory string, page, size int) (*ListResp, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 24
	}
	cat := NormalizeCategory(rawCategory)

	rooms, total, err := s.rooms.List(ctx, repo.ListQuery{Category: cat, Page: page, Size: size})
	if err != nil {
		return nil, err
	}
	now := s.now()
	items := make([]model.Stream, 0, len(rooms))
	for i := range rooms {
		st := rooms[i].ToStream(now)
		st.PlaybackURL = s.playbackURL(&rooms[i])
		if err := s.addSubscriberCount(ctx, &st); err != nil {
			return nil, err
		}
		// streamKey deliberately NOT populated here 鈥?owner-only.
		items = append(items, st)
	}
	return &ListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *RoomService) Get(ctx context.Context, id, viewerID string) (*model.Stream, error) {
	r, err := s.rooms.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, ErrRoomNotFound
		}
		return nil, err
	}
	isOwner := viewerID != "" && viewerID == r.OwnerID
	if r.Status != model.StatusLive && !(isOwner && r.Status == model.StatusPublishing) {
		return nil, ErrRoomNotFound
	}
	st := r.ToStream(s.now())
	st.PlaybackURL = s.playbackURL(r)
	if err := s.addSubscriberCount(ctx, &st); err != nil {
		return nil, err
	}
	if isOwner {
		st.StreamKey = r.StreamKey
	}
	return &st, nil
}

func (s *RoomService) addSubscriberCount(ctx context.Context, st *model.Stream) error {
	if s.social == nil || st == nil || st.ChannelID == "" {
		return nil
	}
	count, err := s.social.FollowerCount(ctx, st.ChannelID)
	if err != nil {
		return err
	}
	st.SubscriberCount = count
	return nil
}
