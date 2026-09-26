package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	defaultRecommendedLiveSize = 12
	maxRecommendedLiveSize     = 12
	recommendationLookback     = 90 * 24 * time.Hour
)

// liveRecommendationPool is the viewer-independent part of RecommendedLive:
// the candidate rooms with live metrics and owner profiles applied, and their
// heat signals. It is cached briefly per category and shared by all viewers,
// so it must not be modified.
type liveRecommendationPool struct {
	rooms       []model.Room
	revenue     map[string]int64 // by room id
	comments    map[string]int64 // by room id
	subscribers map[string]int64 // by channel id

	maxComments, maxRevenue, maxViewers, maxSubscribers int64
}

func (s *RoomService) RecommendedLive(ctx context.Context, viewerID, rawCategory string, size int) (*ListResp, error) {
	if size < 1 {
		size = defaultRecommendedLiveSize
	}
	if size > maxRecommendedLiveSize {
		size = maxRecommendedLiveSize
	}
	category := NormalizeCategory(rawCategory)

	pool, err := s.liveRecommendations.get(category, func() (*liveRecommendationPool, error) {
		return s.loadLiveRecommendationPool(ctx, category)
	})
	if err != nil {
		return nil, err
	}
	if len(pool.rooms) == 0 {
		return &ListResp{Items: []model.Stream{}, Total: 0, Page: 1, Size: size}, nil
	}

	now := s.now()
	following := map[string]bool{}
	if s.social != nil && viewerID != "" {
		followed, err := s.social.Following(ctx, viewerID)
		if err != nil {
			return nil, err
		}
		for _, channelID := range followed {
			following[channelID] = true
		}
	}

	lookback := now.Add(-recommendationLookback)
	watchCategoryScore, maxWatchScore, err := s.watchCategoryPreference(ctx, viewerID, lookback)
	if err != nil {
		return nil, err
	}
	giftPreference, maxGiftScore, err := s.giftPreference(ctx, viewerID, lookback)
	if err != nil {
		return nil, err
	}
	blocked, err := s.blockedRoomOwners(ctx, viewerID, pool.rooms)
	if err != nil {
		return nil, err
	}

	type scoredRoom struct {
		room  model.Room
		score float64
	}
	scored := make([]scoredRoom, 0, len(pool.rooms))
	for _, room := range pool.rooms {
		if blocked[room.OwnerID] {
			continue
		}
		categoryKey := preferenceCategoryKey(room.Category)
		viewers := maxInt64(room.Viewers, room.PeakViewers)
		heatScore := normalized(pool.comments[room.ID], pool.maxComments)*0.42 +
			normalized(pool.revenue[room.ID], pool.maxRevenue)*0.38 +
			normalized(viewers, pool.maxViewers)*0.20
		followScore := 0.0
		if following[room.ChannelID] {
			followScore = 1
		}
		giftScore := normalized(giftPreference.score(room), maxGiftScore)
		watchScore := normalized(watchCategoryScore[categoryKey], maxWatchScore)
		subscriberScore := normalized(pool.subscribers[room.ChannelID], pool.maxSubscribers)
		recencyScore := liveRecencyScore(now, room.StartedAt)
		certifiedScore := 0.0
		if room.Verified {
			certifiedScore = 1
		}

		score := followScore*0.30 +
			giftScore*0.18 +
			watchScore*0.17 +
			heatScore*0.25 +
			subscriberScore*0.08 +
			recencyScore*0.02 +
			certifiedScore*0.06
		scored = append(scored, scoredRoom{room: room, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].room.StartedAt.After(scored[j].room.StartedAt)
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > size {
		scored = scored[:size]
	}

	// Pool rooms already carry live metrics and owner profiles; only
	// fan-club membership depends on the viewer.
	selected := make([]model.Room, 0, len(scored))
	for _, item := range scored {
		selected = append(selected, item.room)
	}
	lookups := roomLookups{fanClub: s.fanClubCreators(ctx, selected, viewerID)}
	items := make([]model.Stream, 0, len(selected))
	for i := range selected {
		st := s.streamWithLookups(&selected[i], now, viewerID, lookups)
		st.SubscriberCount = pool.subscribers[selected[i].ChannelID]
		items = append(items, st)
	}
	return &ListResp{Items: items, Total: int64(len(scored)), Page: 1, Size: size}, nil
}

func (s *RoomService) loadLiveRecommendationPool(ctx context.Context, category string) (*liveRecommendationPool, error) {
	rooms, _, err := s.rooms.List(ctx, repo.ListQuery{Category: category, Page: 1, Size: 200})
	if err != nil {
		return nil, err
	}
	pool := &liveRecommendationPool{
		rooms:       rooms,
		revenue:     map[string]int64{},
		comments:    map[string]int64{},
		subscribers: map[string]int64{},
	}
	if len(rooms) == 0 {
		return pool, nil
	}

	lookups := s.loadRoomLookups(ctx, rooms, "")
	roomIDs := make([]string, 0, len(rooms))
	for i := range rooms {
		lookups.apply(&rooms[i])
		roomIDs = append(roomIDs, rooms[i].ID)
	}
	revenue, err := s.revenueTotals(ctx, roomIDs)
	if err != nil {
		return nil, err
	}
	for roomID, totals := range revenue {
		pool.revenue[roomID] = totals.Total()
	}
	pool.comments, err = s.rooms.DanmuCountsByRooms(ctx, roomIDs)
	if err != nil {
		return nil, err
	}
	pool.subscribers, err = s.subscriberCounts(ctx, rooms)
	if err != nil {
		return nil, err
	}

	for _, room := range rooms {
		pool.maxComments = maxInt64(pool.maxComments, pool.comments[room.ID])
		pool.maxRevenue = maxInt64(pool.maxRevenue, pool.revenue[room.ID])
		pool.maxViewers = maxInt64(pool.maxViewers, maxInt64(room.Viewers, room.PeakViewers))
		pool.maxSubscribers = maxInt64(pool.maxSubscribers, pool.subscribers[room.ChannelID])
	}
	return pool, nil
}

func (s *RoomService) RecordWatch(ctx context.Context, viewerID, roomID string) error {
	if viewerID == "" {
		return errcode.New(401, "Unauthorized")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return ErrRoomNotFound
		}
		return err
	}
	if room.Status != model.StatusLive && room.Status != model.StatusEnded {
		return nil
	}
	blocked, err := s.blocksRoomInteraction(ctx, viewerID, room.OwnerID)
	if err != nil {
		return err
	}
	if blocked {
		return errcode.New(403, "blocked from this channel").WithReason("channel_blocked")
	}
	now := s.now()
	if err := s.rooms.RecordWatchEvent(ctx, &model.RoomWatchEvent{
		ID:              watchEventID(viewerID, room.ID),
		UserID:          viewerID,
		RoomID:          room.ID,
		ChannelID:       room.ChannelID,
		OwnerID:         room.OwnerID,
		Category:        room.Category,
		WatchCount:      1,
		WatchDate:       beijingWatchDate(now),
		DailyWatchCount: 1,
		LastWatchedAt:   now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return err
	}
	return s.saveHistoryLibraryItem(ctx, viewerID, room, now)
}

type giftPreferenceScores struct {
	byOwner    map[string]int64
	byChannel  map[string]int64
	byCategory map[string]int64
}

func (g giftPreferenceScores) score(room model.Room) int64 {
	return g.byOwner[room.OwnerID] + g.byChannel[room.ChannelID] + g.byCategory[preferenceCategoryKey(room.Category)]
}

func (s *RoomService) watchCategoryPreference(ctx context.Context, viewerID string, since time.Time) (map[string]int64, int64, error) {
	out := map[string]int64{}
	if viewerID == "" {
		return out, 0, nil
	}
	rows, err := s.rooms.WatchCategoryRows(ctx, viewerID, since)
	if err != nil {
		if isMissingAnalyticsTable(err) {
			return out, 0, nil
		}
		return nil, 0, err
	}
	var maxScore int64
	for _, row := range rows {
		key := preferenceCategoryKey(row.Category)
		if key == "" {
			continue
		}
		out[key] += row.Count
		maxScore = maxInt64(maxScore, out[key])
	}
	return out, maxScore, nil
}

func (s *RoomService) giftPreference(ctx context.Context, viewerID string, since time.Time) (giftPreferenceScores, int64, error) {
	out := giftPreferenceScores{
		byOwner:    map[string]int64{},
		byChannel:  map[string]int64{},
		byCategory: map[string]int64{},
	}
	if viewerID == "" {
		return out, 0, nil
	}
	rows, err := s.rooms.UserRevenuePreferenceRows(ctx, viewerID, since)
	if err != nil {
		if isMissingAnalyticsTable(err) {
			return out, 0, nil
		}
		return out, 0, err
	}
	var maxScore int64
	for _, row := range rows {
		if row.OwnerID != "" {
			out.byOwner[row.OwnerID] += row.Amount
			maxScore = maxInt64(maxScore, out.byOwner[row.OwnerID])
		}
		if row.ChannelID != "" {
			out.byChannel[row.ChannelID] += row.Amount
			maxScore = maxInt64(maxScore, out.byChannel[row.ChannelID])
		}
		if key := preferenceCategoryKey(row.Category); key != "" {
			out.byCategory[key] += row.Amount
			maxScore = maxInt64(maxScore, out.byCategory[key])
		}
	}
	return out, maxScore, nil
}

func liveRecencyScore(now, startedAt time.Time) float64 {
	if startedAt.IsZero() {
		return 0
	}
	age := now.Sub(startedAt)
	if age < 0 {
		return 1
	}
	if age >= 6*time.Hour {
		return 0
	}
	return 1 - age.Hours()/6
}

func preferenceCategoryKey(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func watchEventID(userID, roomID string) string {
	sum := sha256.Sum256([]byte(userID + ":" + roomID))
	return "watch-" + hex.EncodeToString(sum[:])[:24]
}

func beijingWatchDate(now time.Time) string {
	return now.UTC().Add(8 * time.Hour).Format("2006-01-02")
}
