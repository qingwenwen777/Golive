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

	// Viewer-independent candidate pools for the public recommendation and
	// hot-replay rails, keyed by category (and window). Per-viewer signals
	// and block filtering are applied on top of the cached pool.
	liveRecommendations *ttlCache[*liveRecommendationPool]
	hotReplays          *ttlCache[*hotReplayPool]
}

const (
	liveRecommendationCacheTTL = 10 * time.Second
	hotReplayCacheTTL          = 15 * time.Second
	// Keys are categories, so this only guards against unbounded input.
	roomPoolCacheEntries = 256
)

type ChannelBlockChecker interface {
	BlocksInteraction(ctx context.Context, viewerID, creatorID string) (bool, error)
	CreatorBlocks(ctx context.Context, creatorID, viewerID string) (bool, error)
}

// ChannelBlockBatchChecker is optionally implemented by a ChannelBlockChecker
// to answer BlocksInteraction for a whole page of creators in one query. It
// returns the creators blocked either way.
type ChannelBlockBatchChecker interface {
	BlocksInteractionAmong(ctx context.Context, viewerID string, creatorIDs []string) (map[string]bool, error)
}

func NewRoomService(rooms *repo.RoomRepo, flvBase string, social ...*repo.SocialRepo) *RoomService {
	if flvBase == "" {
		flvBase = "http://localhost:8082/live"
	}
	var socialRepo *repo.SocialRepo
	if len(social) > 0 {
		socialRepo = social[0]
	}
	return &RoomService{
		rooms:               rooms,
		social:              socialRepo,
		flvBase:             strings.TrimRight(flvBase, "/"),
		now:                 time.Now,
		liveRecommendations: newTTLCache[*liveRecommendationPool](liveRecommendationCacheTTL, roomPoolCacheEntries),
		hotReplays:          newTTLCache[*hotReplayPool](hotReplayCacheTTL, roomPoolCacheEntries),
	}
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

// playbackURL builds the HTTP-FLV URL for a live room. Viewers allowed to
// watch receive this; it names the stream by its play name, which keeps the
// publish key server-side and cannot be derived from the public room id.
func (s *RoomService) playbackURL(r *model.Room) string {
	if r == nil || r.Status != model.StatusLive || r.StreamKey == "" {
		return ""
	}
	return s.flvBase + "/" + playStreamName(r.ID, r.StreamKey) + ".flv"
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
	blocked, err := s.blockedRoomOwners(ctx, viewerID, rooms)
	if err != nil {
		return nil, err
	}
	visible := make([]model.Room, 0, len(rooms))
	for _, room := range rooms {
		if !blocked[room.OwnerID] {
			visible = append(visible, room)
		}
	}
	// streamKey is deliberately not populated here; it is owner-only.
	items, err := s.streamsFromRooms(ctx, visible, s.now(), "")
	if err != nil {
		return nil, err
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
	return s.streamsFromRooms(ctx, rooms, s.now(), "")
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
	visible := make([]model.Room, 0, minInt(limit, len(rooms)))
	replays := make([]*model.Replay, 0, cap(visible))
	for i := range rooms {
		replay, err := s.replay.ReplayDTO(ctx, rooms[i], viewerID)
		if err != nil {
			return nil, err
		}
		if replay == nil || !replay.CanWatch {
			continue
		}
		visible = append(visible, rooms[i])
		replays = append(replays, replay)
		if len(visible) >= limit {
			break
		}
	}
	items, err := s.streamsFromRooms(ctx, visible, s.now(), viewerID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Replay = replays[i]
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
	return s.streamWithLookups(room, now, viewerID, s.loadRoomLookups(ctx, []model.Room{*room}, viewerID))
}

// streamsFromRooms builds the streams for a page of rooms, fetching owner
// profiles, live viewer metrics, fan-club membership and follower counts with
// one query or pipeline each instead of once per room.
func (s *RoomService) streamsFromRooms(ctx context.Context, rooms []model.Room, now time.Time, viewerID string) ([]model.Stream, error) {
	items := make([]model.Stream, 0, len(rooms))
	if len(rooms) == 0 {
		return items, nil
	}
	subscribers, err := s.subscriberCounts(ctx, rooms)
	if err != nil {
		return nil, err
	}
	lookups := s.loadRoomLookups(ctx, rooms, viewerID)
	for i := range rooms {
		st := s.streamWithLookups(&rooms[i], now, viewerID, lookups)
		st.SubscriberCount = subscribers[st.ChannelID]
		items = append(items, st)
	}
	return items, nil
}

func (s *RoomService) streamWithLookups(room *model.Room, now time.Time, viewerID string, lookups roomLookups) model.Stream {
	adjusted := *room
	lookups.apply(&adjusted)
	st := adjusted.ToStream(now)
	st.PlaybackURL = s.playbackURL(&adjusted)
	st.FanClubMember = lookups.fanClubMember(&adjusted, viewerID)
	if adjusted.FanClubOnly && !st.FanClubMember {
		st.PlaybackURL = ""
	}
	return st
}

// roomLookups is the per-room data a stream needs beyond the rooms row. Any
// map may be nil, which leaves that part of the room as stored.
type roomLookups struct {
	profiles map[string]repo.OwnerProfile  // by owner id
	metrics  map[string]repo.ViewerMetrics // by room id, live rooms only
	fanClub  map[string]bool               // creator ids whose fan club the viewer is in
}

// loadRoomLookups fetches roomLookups for rooms. Lookup failures leave the
// stored values in place, as a missing profile or metric always has.
func (s *RoomService) loadRoomLookups(ctx context.Context, rooms []model.Room, viewerID string) roomLookups {
	return roomLookups{
		profiles: s.ownerProfiles(ctx, rooms),
		metrics:  s.liveViewerMetrics(ctx, rooms),
		fanClub:  s.fanClubCreators(ctx, rooms, viewerID),
	}
}

func (s *RoomService) ownerProfiles(ctx context.Context, rooms []model.Room) map[string]repo.OwnerProfile {
	ownerIDs := make([]string, 0, len(rooms))
	seen := make(map[string]bool, len(rooms))
	for _, room := range rooms {
		if strings.TrimSpace(room.OwnerID) == "" || seen[room.OwnerID] {
			continue
		}
		seen[room.OwnerID] = true
		ownerIDs = append(ownerIDs, room.OwnerID)
	}
	if len(ownerIDs) == 0 {
		return nil
	}
	profiles, err := s.rooms.OwnerProfiles(ctx, ownerIDs)
	if err != nil {
		return nil
	}
	return profiles
}

func (s *RoomService) liveViewerMetrics(ctx context.Context, rooms []model.Room) map[string]repo.ViewerMetrics {
	if s.live == nil {
		return nil
	}
	roomIDs := make([]string, 0, len(rooms))
	for _, room := range rooms {
		if room.Status == model.StatusLive {
			roomIDs = append(roomIDs, room.ID)
		}
	}
	if len(roomIDs) == 0 {
		return nil
	}
	metrics, err := s.live.ViewerMetricsByRooms(ctx, roomIDs)
	if err != nil {
		return nil
	}
	return metrics
}

func (s *RoomService) fanClubCreators(ctx context.Context, rooms []model.Room, viewerID string) map[string]bool {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" {
		return nil
	}
	creatorIDs := make([]string, 0, len(rooms))
	seen := map[string]bool{}
	for _, room := range rooms {
		creatorID := strings.TrimSpace(room.OwnerID)
		if !room.FanClubOnly || creatorID == "" || creatorID == viewerID || seen[creatorID] {
			continue
		}
		seen[creatorID] = true
		creatorIDs = append(creatorIDs, creatorID)
	}
	if len(creatorIDs) == 0 {
		return nil
	}
	members, err := s.rooms.FanClubCreators(ctx, viewerID, creatorIDs)
	if err != nil {
		return nil
	}
	return members
}

// apply overlays live viewer metrics and the owner's current profile.
func (l roomLookups) apply(room *model.Room) {
	if metrics, ok := l.metrics[room.ID]; ok && room.Status == model.StatusLive {
		room.Viewers = metrics.Viewers
		if metrics.Peak > room.PeakViewers {
			room.PeakViewers = metrics.Peak
		}
		if room.PeakViewers < room.Viewers {
			room.PeakViewers = room.Viewers
		}
	}
	if strings.TrimSpace(room.OwnerID) == "" {
		return
	}
	profile, ok := l.profiles[room.OwnerID]
	if !ok {
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

func (l roomLookups) fanClubMember(room *model.Room, viewerID string) bool {
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
	return l.fanClub[strings.TrimSpace(room.OwnerID)]
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

// subscriberCounts returns follower counts by channel id for rooms in one
// Redis pipeline.
func (s *RoomService) subscriberCounts(ctx context.Context, rooms []model.Room) (map[string]int64, error) {
	if s.social == nil {
		return map[string]int64{}, nil
	}
	channelIDs := make([]string, 0, len(rooms))
	for _, room := range rooms {
		if room.ChannelID != "" {
			channelIDs = append(channelIDs, room.ChannelID)
		}
	}
	return s.social.FollowerCounts(ctx, channelIDs)
}

func (s *RoomService) blocksRoomInteraction(ctx context.Context, viewerID, ownerID string) (bool, error) {
	if s.blocks == nil || viewerID == "" || ownerID == "" || viewerID == ownerID {
		return false, nil
	}
	return s.blocks.BlocksInteraction(ctx, viewerID, ownerID)
}

// blockedRoomOwners returns the owners of rooms that block, or are blocked
// by, viewerID. It asks the block checker once per page when it supports
// ChannelBlockBatchChecker, and once per distinct owner otherwise.
func (s *RoomService) blockedRoomOwners(ctx context.Context, viewerID string, rooms []model.Room) (map[string]bool, error) {
	blocked := map[string]bool{}
	if s.blocks == nil || viewerID == "" {
		return blocked, nil
	}
	ownerIDs := make([]string, 0, len(rooms))
	seen := map[string]bool{}
	for _, room := range rooms {
		if room.OwnerID == "" || room.OwnerID == viewerID || seen[room.OwnerID] {
			continue
		}
		seen[room.OwnerID] = true
		ownerIDs = append(ownerIDs, room.OwnerID)
	}
	if len(ownerIDs) == 0 {
		return blocked, nil
	}
	if batch, ok := s.blocks.(ChannelBlockBatchChecker); ok {
		found, err := batch.BlocksInteractionAmong(ctx, viewerID, ownerIDs)
		if err != nil {
			return nil, err
		}
		for ownerID, isBlocked := range found {
			if isBlocked {
				blocked[ownerID] = true
			}
		}
		return blocked, nil
	}
	for _, ownerID := range ownerIDs {
		isBlocked, err := s.blocks.BlocksInteraction(ctx, viewerID, ownerID)
		if err != nil {
			return nil, err
		}
		if isBlocked {
			blocked[ownerID] = true
		}
	}
	return blocked, nil
}
