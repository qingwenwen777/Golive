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
	blocks  ChannelBlockChecker
	flvBase string
	now     func() time.Time
}

type ChannelBlockChecker interface {
	BlocksInteraction(ctx context.Context, viewerID, creatorID string) (bool, error)
	CreatorBlocks(ctx context.Context, creatorID, viewerID string) (bool, error)
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

func (s *RoomService) SetBlockChecker(blocks ChannelBlockChecker) {
	s.blocks = blocks
}

// playbackURL builds the public HTTP-FLV URL for a live room. Viewers receive
// this; it names the stream by room id so the publish key stays server-side.
func (s *RoomService) playbackURL(r *model.Room) string {
	if r == nil || r.Status != model.StatusLive || r.StreamKey == "" {
		return ""
	}
	return s.flvBase + "/" + r.ID + ".flv"
}

// NormalizeCategory accepts empty / "all" / "すべて" as no filter; else trimmed input.
func NormalizeCategory(raw string) string {
	c := strings.TrimSpace(raw)
	if c == "" || strings.EqualFold(c, "all") || c == "すべて" {
		return ""
	}
	return c
}

// maxListPageSize caps List's page size: every listed room costs follow-up
// lookups (blocks, owner profile, viewer metrics, followers).
const maxListPageSize = 100

type ListResp struct {
	Items []model.Stream `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

func (s *RoomService) List(ctx context.Context, viewerID, rawCategory string, page, size int) (*ListResp, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 24
	}
	if size > maxListPageSize {
		size = maxListPageSize
	}
	cat := NormalizeCategory(rawCategory)

	rooms, total, err := s.rooms.List(ctx, repo.ListQuery{Category: cat, Page: page, Size: size})
	if err != nil {
		return nil, err
	}
	now := s.now()
	items := make([]model.Stream, 0, len(rooms))
	for i := range rooms {
		blocked, err := s.blocksRoomInteraction(ctx, viewerID, rooms[i].OwnerID)
		if err != nil {
			return nil, err
		}
		if blocked {
			continue
		}
		st := s.streamFromRoom(ctx, &rooms[i], now, "")
		if err := s.addSubscriberCount(ctx, &st); err != nil {
			return nil, err
		}
		// streamKey is deliberately not populated here; it is owner-only.
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
		st := s.streamFromRoom(ctx, &rooms[i], now, "")
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
		st := s.streamFromRoom(ctx, &rooms[i], now, viewerID)
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
	blocked, err := s.blocksRoomInteraction(ctx, viewerID, r.OwnerID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrRoomNotFound
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
	st := s.streamFromRoom(ctx, r, s.now(), viewerID)
	st.Replay = replay
	if err := s.addSubscriberCount(ctx, &st); err != nil {
		return nil, err
	}
	if isOwner {
		st.StreamKey = obsStreamKey(r.ID, r.StreamKey)
	}
	return &st, nil
}

func (s *RoomService) streamFromRoom(ctx context.Context, room *model.Room, now time.Time, viewerID string) model.Stream {
	adjusted := *room
	s.applyLiveViewerMetrics(ctx, &adjusted)
	s.applyOwnerProfile(ctx, &adjusted)
	st := adjusted.ToStream(now)
	st.PlaybackURL = s.playbackURL(&adjusted)
	st.FanClubMember = s.canWatchFanClubRoom(ctx, &adjusted, viewerID)
	if adjusted.FanClubOnly && !st.FanClubMember {
		st.PlaybackURL = ""
	}
	return st
}

func (s *RoomService) applyOwnerProfile(ctx context.Context, room *model.Room) {
	if room == nil || strings.TrimSpace(room.OwnerID) == "" {
		return
	}
	profile, err := s.rooms.OwnerProfile(ctx, room.OwnerID)
	if err != nil {
		return
	}
	room.Verified = profile.Verified
	name := strings.TrimSpace(profile.DisplayName)
	if name == "" {
		name = strings.TrimSpace(profile.Username)
	}
	if name != "" {
		room.Channel = name
	}
	if strings.TrimSpace(profile.Avatar) != "" {
		room.Avatar = profile.Avatar
	}
}

func (s *RoomService) canWatchFanClubRoom(ctx context.Context, room *model.Room, viewerID string) bool {
	if room == nil || !room.FanClubOnly {
		return false
	}
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" {
		return false
	}
	if viewerID == room.OwnerID {
		return true
	}
	ok, err := s.rooms.IsFanClubMember(ctx, viewerID, room.OwnerID)
	return err == nil && ok
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

func (s *RoomService) blocksRoomInteraction(ctx context.Context, viewerID, ownerID string) (bool, error) {
	if s.blocks == nil || viewerID == "" || ownerID == "" || viewerID == ownerID {
		return false, nil
	}
	return s.blocks.BlocksInteraction(ctx, viewerID, ownerID)
}
