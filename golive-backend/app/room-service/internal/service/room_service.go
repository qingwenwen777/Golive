package service

import (
	"context"
	"errors"
	"sort"
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
	live    *repo.LiveRepo
	replay  *ReplayService
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

func (s *RoomService) SetReplayService(replay *ReplayService) {
	s.replay = replay
}

func (s *RoomService) SetLiveRepo(live *repo.LiveRepo) {
	s.live = live
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
		st := s.streamFromRoom(ctx, &rooms[i], now)
		if err := s.addSubscriberCount(ctx, &st); err != nil {
			return nil, err
		}
		// streamKey deliberately NOT populated here 鈥?owner-only.
		items = append(items, st)
	}
	return &ListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *RoomService) SearchLive(ctx context.Context, query string, limit int) ([]model.Stream, error) {
	limit = normalizeSearchSize(limit, 8, 24)
	phrase := repo.NewSearchPhrase(query)
	if phrase.Empty() {
		return []model.Stream{}, nil
	}
	rooms, err := s.rooms.SearchLiveRooms(ctx, phrase, limit*6)
	if err != nil {
		return nil, err
	}
	sortRoomsBySearch(rooms, phrase)
	if len(rooms) > limit {
		rooms = rooms[:limit]
	}
	now := s.now()
	items := make([]model.Stream, 0, len(rooms))
	for i := range rooms {
		st := s.streamFromRoom(ctx, &rooms[i], now)
		if err := s.addSubscriberCount(ctx, &st); err != nil {
			return nil, err
		}
		items = append(items, st)
	}
	return items, nil
}

func (s *RoomService) SearchReplays(ctx context.Context, viewerID, query string, limit int) ([]model.Stream, error) {
	limit = normalizeSearchSize(limit, 8, 24)
	phrase := repo.NewSearchPhrase(query)
	if phrase.Empty() || s.replay == nil {
		return []model.Stream{}, nil
	}
	rooms, err := s.rooms.SearchReplayRooms(ctx, phrase, limit*8)
	if err != nil {
		return nil, err
	}
	sortRoomsBySearch(rooms, phrase)
	now := s.now()
	items := make([]model.Stream, 0, minInt(limit, len(rooms)))
	for i := range rooms {
		replay, err := s.replay.ReplayDTO(ctx, rooms[i], viewerID)
		if err != nil {
			return nil, err
		}
		if replay == nil || !replay.CanWatch {
			continue
		}
		st := rooms[i].ToStream(now)
		st.Replay = replay
		if err := s.addSubscriberCount(ctx, &st); err != nil {
			return nil, err
		}
		items = append(items, st)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
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
	isPublicScheduled := r.Status == model.StatusScheduled && !s.now().After(r.StartedAt.Add(30*time.Minute))
	isPublicAppointmentStarting := r.Status == model.StatusPublishing && strings.HasPrefix(r.ID, "appt-") && s.now().Before(r.StartedAt.Add(30*time.Minute))
	var replay *model.Replay
	var replayVisible bool
	if s.replay != nil && (r.Status == model.StatusEnded || isOwner) {
		var err error
		replay, err = s.replay.ReplayDTO(ctx, *r, viewerID)
		if err != nil {
			return nil, err
		}
		if r.Status == model.StatusEnded {
			replayVisible = replay != nil && (replay.CanWatch || replay.CanManage)
		}
	}
	if r.Status != model.StatusLive && !isPublicScheduled && !isPublicAppointmentStarting && !(isOwner && r.Status == model.StatusPublishing) && !replayVisible {
		return nil, ErrRoomNotFound
	}
	st := s.streamFromRoom(ctx, r, s.now())
	st.Replay = replay
	if err := s.addSubscriberCount(ctx, &st); err != nil {
		return nil, err
	}
	if isOwner {
		st.StreamKey = r.StreamKey
	}
	return &st, nil
}

func (s *RoomService) streamFromRoom(ctx context.Context, room *model.Room, now time.Time) model.Stream {
	adjusted := *room
	s.applyLiveViewerMetrics(ctx, &adjusted)
	st := adjusted.ToStream(now)
	st.PlaybackURL = s.playbackURL(&adjusted)
	return st
}

func (s *RoomService) applyLiveViewerMetrics(ctx context.Context, room *model.Room) {
	if s.live == nil || room == nil || room.Status != model.StatusLive {
		return
	}
	metrics, err := s.live.ViewerMetrics(ctx, room.ID)
	if err != nil || metrics == nil {
		return
	}
	room.Viewers = metrics.Viewers
	if metrics.Peak > room.PeakViewers {
		room.PeakViewers = metrics.Peak
	}
	if room.PeakViewers < room.Viewers {
		room.PeakViewers = room.Viewers
	}
}

func sortRoomsBySearch(rooms []model.Room, phrase repo.SearchPhrase) {
	sort.SliceStable(rooms, func(i, j int) bool {
		left := searchScore(phrase, rooms[i].Title, rooms[i].TitleJa, rooms[i].Description, rooms[i].Channel, rooms[i].ChannelID, rooms[i].Category, rooms[i].CategoryJa)
		right := searchScore(phrase, rooms[j].Title, rooms[j].TitleJa, rooms[j].Description, rooms[j].Channel, rooms[j].ChannelID, rooms[j].Category, rooms[j].CategoryJa)
		if left == right {
			return rooms[i].StartedAt.After(rooms[j].StartedAt)
		}
		return left > right
	})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
