package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

var ErrForbiddenAnalytics = errcode.New(403, "Forbidden")

type LiveHistoryResp struct {
	Items []LiveHistoryItem `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

type HotReplayResp struct {
	Items []HotReplayItem `json:"items"`
	Total int64           `json:"total"`
	Days  int             `json:"days"`
	Size  int             `json:"size"`
}

type HotReplayItem struct {
	model.Stream
	Likes        int64 `json:"likes"`
	CommentCount int64 `json:"commentCount"`
	// RevenueCoin feeds the hot score only; it is creator income, so it is
	// never serialised on this public endpoint.
	RevenueCoin int64   `json:"-"`
	HotScore    float64 `json:"hotScore"`
}

// LiveHistoryItem is one past live. The channel history endpoint is public,
// so RevenueCoin, NewSubscribers and TopFan are creator-only: set by
// addOwnerMetrics for the owner and omitted for everyone else.
type LiveHistoryItem struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description,omitempty"`
	Channel         string           `json:"channel"`
	ChannelID       string           `json:"channelId"`
	Cover           string           `json:"cover"`
	Category        string           `json:"category"`
	StartedAt       string           `json:"startedAt"`
	EndedAt         string           `json:"endedAt"`
	Duration        string           `json:"duration"`
	DurationSeconds int64            `json:"durationSeconds"`
	PeakViewers     int64            `json:"peakViewers"`
	DanmuCount      int64            `json:"danmuCount"`
	RevenueCoin     *int64           `json:"revenueCoin,omitempty"`
	NewSubscribers  *int64           `json:"newSubscribers,omitempty"`
	TopFan          *FanContribution `json:"topFan,omitempty"`
	Replay          *model.Replay    `json:"replay,omitempty"`
}

type FanContribution struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	Amount int64  `json:"amount"`
}

type MonthlyMetric struct {
	Month       string `json:"month"`
	RevenueCoin int64  `json:"revenueCoin"`
	Subscribers int64  `json:"subscribers"`
	WatchHours  int64  `json:"watchHours"`
	Streams     int64  `json:"streams"`
	PeakViewers int64  `json:"peakViewers"`
}

type FanBadgeDistributionBucket struct {
	Bucket            string `json:"bucket"`
	MinLevel          int    `json:"minLevel"`
	MaxLevel          int    `json:"maxLevel,omitempty"`
	FanCount          int64  `json:"fanCount"`
	TotalContribution int64  `json:"totalContribution"`
}

type CreatorAnalyticsResp struct {
	ChannelID            string                       `json:"channelId"`
	RevenueCoin          int64                        `json:"revenueCoin"`
	SubscriberCount      int64                        `json:"subscriberCount"`
	Streams              int64                        `json:"streams"`
	WatchHours           int64                        `json:"watchHours"`
	PeakViewers          int64                        `json:"peakViewers"`
	Monthly              []MonthlyMetric              `json:"monthly"`
	FanBadgeDistribution []FanBadgeDistributionBucket `json:"fanBadgeDistribution"`
	History              []LiveHistoryItem            `json:"history"`
}

type LiveAnalysisResp struct {
	Record           LiveHistoryItem   `json:"record"`
	TopFans          []FanContribution `json:"topFans"`
	GiftRevenue      int64             `json:"giftRevenue"`
	SuperChatRevenue int64             `json:"superChatRevenue"`
}

func (s *RoomService) HistoryByChannel(ctx context.Context, channelKey, viewerID string, page, size int, replaysOnly bool) (*LiveHistoryResp, error) {
	ownerID, err := s.rooms.ResolveOwnerID(ctx, channelKey)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return &LiveHistoryResp{Items: []LiveHistoryItem{}, Total: 0, Page: page, Size: size}, nil
		}
		return nil, err
	}
	blocked, err := s.blocksRoomInteraction(ctx, viewerID, ownerID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return &LiveHistoryResp{Items: []LiveHistoryItem{}, Total: 0, Page: page, Size: size}, nil
	}
	if replaysOnly && viewerID != ownerID {
		items, total, err := s.replayItemsForViewer(ctx, ownerID, viewerID, page, size)
		if err != nil {
			return nil, err
		}
		return &LiveHistoryResp{Items: items, Total: total, Page: page, Size: size}, nil
	}
	items, _, total, err := s.historyItemsForOwner(ctx, ownerID, viewerID, page, size)
	if err != nil {
		return nil, err
	}
	return &LiveHistoryResp{Items: items, Total: total, Page: page, Size: size}, nil
}

// hotReplayPool is the viewer-independent part of HotReplays: every candidate
// replay in the window with its engagement signals. It is cached briefly per
// window and category and shared by all viewers, so it must not be modified;
// visibility and blocks are applied per viewer.
type hotReplayPool struct {
	rooms       []model.Room
	likes       map[string]int64 // by room id
	comments    map[string]int64 // by room id
	revenue     map[string]int64 // by room id
	subscribers map[string]int64 // by channel id
}

func (s *RoomService) HotReplays(ctx context.Context, viewerID, rawCategory string, days, size int) (*HotReplayResp, error) {
	if days < 1 {
		days = 3
	}
	if days > 30 {
		days = 30
	}
	if size < 1 {
		size = 4
	}
	if size > 12 {
		size = 12
	}
	if s.replay == nil {
		return &HotReplayResp{Items: []HotReplayItem{}, Total: 0, Days: days, Size: size}, nil
	}

	category := NormalizeCategory(rawCategory)
	pool, err := s.hotReplays.get(strconv.Itoa(days)+"|"+category, func() (*hotReplayPool, error) {
		return s.loadHotReplayPool(ctx, days, category)
	})
	if err != nil {
		return nil, err
	}
	blocked, err := s.blockedRoomOwners(ctx, viewerID, pool.rooms)
	if err != nil {
		return nil, err
	}
	visibleRooms := make([]model.Room, 0, len(pool.rooms))
	replaysByRoom := make(map[string]*model.Replay, len(pool.rooms))
	for _, room := range pool.rooms {
		if blocked[room.OwnerID] {
			continue
		}
		replay, err := s.replay.ReplayDTO(ctx, room, viewerID)
		if err != nil {
			return nil, err
		}
		if replay == nil || !replay.CanWatch {
			continue
		}
		visibleRooms = append(visibleRooms, room)
		replaysByRoom[room.ID] = replay
	}
	if len(visibleRooms) == 0 {
		return &HotReplayResp{Items: []HotReplayItem{}, Total: 0, Days: days, Size: size}, nil
	}

	var maxLikes, maxComments, maxRevenue, maxPeak int64
	for _, room := range visibleRooms {
		maxLikes = maxInt64(maxLikes, pool.likes[room.ID])
		maxComments = maxInt64(maxComments, pool.comments[room.ID])
		maxRevenue = maxInt64(maxRevenue, pool.revenue[room.ID])
		maxPeak = maxInt64(maxPeak, maxInt64(room.PeakViewers, room.Viewers))
	}

	now := s.now()
	items := make([]HotReplayItem, 0, len(visibleRooms))
	for _, room := range visibleRooms {
		st := room.ToStream(now)
		st.Replay = replaysByRoom[room.ID]
		st.SubscriberCount = pool.subscribers[st.ChannelID]
		likes := pool.likes[room.ID]
		comments := pool.comments[room.ID]
		revenue := pool.revenue[room.ID]
		peak := maxInt64(room.PeakViewers, room.Viewers)
		st.PeakViewers = peak
		items = append(items, HotReplayItem{
			Stream:       st,
			Likes:        likes,
			CommentCount: comments,
			RevenueCoin:  revenue,
			HotScore:     hotReplayScore(likes, comments, revenue, peak, maxLikes, maxComments, maxRevenue, maxPeak),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].HotScore == items[j].HotScore {
			return items[i].EndedAt > items[j].EndedAt
		}
		return items[i].HotScore > items[j].HotScore
	})
	total := int64(len(items))
	if len(items) > size {
		items = items[:size]
	}
	return &HotReplayResp{Items: items, Total: total, Days: days, Size: size}, nil
}

func (s *RoomService) loadHotReplayPool(ctx context.Context, days int, category string) (*hotReplayPool, error) {
	since := s.now().AddDate(0, 0, -days)
	rooms, err := s.rooms.HotReplayCandidates(ctx, since, 100, category)
	if err != nil {
		return nil, err
	}
	pool := &hotReplayPool{
		rooms:       rooms,
		likes:       map[string]int64{},
		comments:    map[string]int64{},
		revenue:     map[string]int64{},
		subscribers: map[string]int64{},
	}
	if len(rooms) == 0 {
		return pool, nil
	}

	roomIDs := make([]string, 0, len(rooms))
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.ID)
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
	if s.social != nil {
		pool.likes, err = s.social.LikeCounts(ctx, roomIDs)
		if err != nil {
			return nil, err
		}
	}
	pool.subscribers, err = s.subscriberCounts(ctx, rooms)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *RoomService) CreatorAnalytics(ctx context.Context, channelKey, viewerID string) (*CreatorAnalyticsResp, error) {
	ownerID, err := s.rooms.ResolveOwnerID(ctx, channelKey)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) && viewerID != "" && (channelKey == viewerID || strings.TrimPrefix(channelKey, "ch-") == viewerID) {
			ownerID = viewerID
		} else {
			return nil, err
		}
	}
	if viewerID == "" || viewerID != ownerID {
		return nil, ErrForbiddenAnalytics
	}

	history, rooms, _, err := s.historyItemsForOwner(ctx, ownerID, viewerID, 1, 100)
	if err != nil {
		return nil, err
	}

	channelID := "ch-" + ownerID
	monthly, totals := s.monthlyMetrics(ctx, ownerID, channelID, rooms)
	fanBadgeDistribution, err := s.fanBadgeDistribution(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	subscriberCount := int64(0)
	if s.social != nil {
		count, err := s.social.FollowerCount(ctx, channelID)
		if err != nil {
			return nil, err
		}
		subscriberCount = count
	}

	return &CreatorAnalyticsResp{
		ChannelID:            channelID,
		RevenueCoin:          totals.RevenueCoin,
		SubscriberCount:      subscriberCount,
		Streams:              totals.Streams,
		WatchHours:           totals.WatchHours,
		PeakViewers:          totals.PeakViewers,
		Monthly:              monthly,
		FanBadgeDistribution: fanBadgeDistribution,
		History:              history,
	}, nil
}

func (s *RoomService) LiveAnalysis(ctx context.Context, channelKey, roomID, viewerID string) (*LiveAnalysisResp, error) {
	ownerID, err := s.rooms.ResolveOwnerID(ctx, channelKey)
	if err != nil {
		return nil, err
	}
	if viewerID == "" || viewerID != ownerID {
		return nil, ErrForbiddenAnalytics
	}
	room, err := s.rooms.EndedRoomByOwner(ctx, ownerID, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil, ErrRoomNotFound
		}
		return nil, err
	}
	revenue, err := s.revenueTotals(ctx, []string{room.ID})
	if err != nil {
		return nil, err
	}
	fans, err := s.topFansByRooms(ctx, []string{room.ID}, 8)
	if err != nil {
		return nil, err
	}
	danmuCounts, err := s.rooms.DanmuCountsByRooms(ctx, []string{room.ID})
	if err != nil {
		return nil, err
	}
	item := s.historyItem(*room, danmuCounts[room.ID])
	totals := revenue[room.ID]
	s.addOwnerMetrics(ctx, &item, *room, totals.Total(), fans[room.ID])
	topFans := fans[room.ID]
	if topFans == nil {
		topFans = []FanContribution{}
	}
	return &LiveAnalysisResp{
		Record:           item,
		TopFans:          topFans,
		GiftRevenue:      totals.Gift,
		SuperChatRevenue: totals.SuperChat,
	}, nil
}

func (s *RoomService) historyItemsForOwner(ctx context.Context, ownerID, viewerID string, page, size int) ([]LiveHistoryItem, []model.Room, int64, error) {
	rooms, total, err := s.rooms.HistoryByOwner(ctx, ownerID, page, size)
	if err != nil {
		return nil, nil, 0, err
	}
	items, err := s.historyItemsFromRooms(ctx, rooms, viewerID)
	if err != nil {
		return nil, nil, 0, err
	}
	return items, rooms, total, nil
}

func (s *RoomService) replayItemsForViewer(ctx context.Context, ownerID, viewerID string, page, size int) ([]LiveHistoryItem, int64, error) {
	if s.replay == nil {
		return []LiveHistoryItem{}, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 24
	}
	if size > 100 {
		size = 100
	}
	rooms, err := s.rooms.ReplayCandidateRoomsByOwner(ctx, ownerID)
	if err != nil {
		return nil, 0, err
	}
	visible := make([]model.Room, 0, len(rooms))
	for _, room := range rooms {
		replay, err := s.replay.ReplayDTO(ctx, room, viewerID)
		if err != nil {
			return nil, 0, err
		}
		if replay != nil && replay.CanWatch {
			visible = append(visible, room)
		}
	}
	total := int64(len(visible))
	start := (page - 1) * size
	if start >= len(visible) {
		return []LiveHistoryItem{}, total, nil
	}
	end := start + size
	if end > len(visible) {
		end = len(visible)
	}
	items, err := s.historyItemsFromRooms(ctx, visible[start:end], viewerID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *RoomService) historyItemsFromRooms(ctx context.Context, rooms []model.Room, viewerID string) ([]LiveHistoryItem, error) {
	items := make([]LiveHistoryItem, 0, len(rooms))
	if len(rooms) == 0 {
		return items, nil
	}
	roomIDs := make([]string, 0, len(rooms))
	var ownedIDs []string
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.ID)
		if viewerOwnsRoom(room, viewerID) {
			ownedIDs = append(ownedIDs, room.ID)
		}
	}
	revenueByRoom := map[string]repo.RevenueTotals{}
	topFanByRoom := map[string][]FanContribution{}
	if len(ownedIDs) > 0 {
		var err error
		revenueByRoom, err = s.revenueTotals(ctx, ownedIDs)
		if err != nil {
			return nil, err
		}
		topFanByRoom, err = s.topFansByRooms(ctx, ownedIDs, 1)
		if err != nil {
			return nil, err
		}
	}
	danmuCounts, err := s.rooms.DanmuCountsByRooms(ctx, roomIDs)
	if err != nil {
		return nil, err
	}

	for _, room := range rooms {
		item := s.historyItem(room, danmuCounts[room.ID])
		if viewerOwnsRoom(room, viewerID) {
			s.addOwnerMetrics(ctx, &item, room, revenueByRoom[room.ID].Total(), topFanByRoom[room.ID])
		}
		if s.replay != nil {
			replay, err := s.replay.ReplayDTO(ctx, room, viewerID)
			if err != nil {
				return nil, err
			}
			item.Replay = replay
		}
		items = append(items, item)
	}
	return items, nil
}

func viewerOwnsRoom(room model.Room, viewerID string) bool {
	return viewerID != "" && viewerID == room.OwnerID
}

// historyItem builds the public part of a history record.
func (s *RoomService) historyItem(room model.Room, danmuCount int64) LiveHistoryItem {
	endedAt := endedAtOf(room)
	duration := endedAt.Sub(room.StartedAt)
	if duration < 0 {
		duration = 0
	}
	peak := room.PeakViewers
	if room.Viewers > peak {
		peak = room.Viewers
	}

	return LiveHistoryItem{
		ID:              room.ID,
		Title:           room.Title,
		Description:     room.Description,
		Channel:         room.Channel,
		ChannelID:       room.ChannelID,
		Cover:           room.Cover,
		Category:        room.Category,
		StartedAt:       room.StartedAt.UTC().Format(time.RFC3339),
		EndedAt:         endedAt.UTC().Format(time.RFC3339),
		Duration:        model.FormatDuration(duration),
		DurationSeconds: int64(duration.Seconds()),
		PeakViewers:     peak,
		DanmuCount:      danmuCount,
	}
}

// addOwnerMetrics fills the creator-only revenue, top fan and new-subscriber
// fields from the room's revenue and ranked top fans. Only call it when the
// viewer owns the room.
func (s *RoomService) addOwnerMetrics(ctx context.Context, item *LiveHistoryItem, room model.Room, revenue int64, topFans []FanContribution) {
	item.RevenueCoin = &revenue
	if len(topFans) > 0 {
		topFan := topFans[0]
		item.TopFan = &topFan
	}
	newSubscribers := s.subscribersBetween(ctx, room.ChannelID, room.StartedAt, endedAtOf(room))
	item.NewSubscribers = &newSubscribers
}

func (s *RoomService) monthlyMetrics(ctx context.Context, ownerID, channelID string, rooms []model.Room) ([]MonthlyMetric, CreatorAnalyticsResp) {
	now := s.now()
	startMonth := time.Date(now.Year(), now.Month()-5, 1, 0, 0, 0, 0, now.Location())
	rows, err := s.rooms.RevenueRowsByOwnerSince(ctx, ownerID, startMonth)
	if err != nil && isMissingAnalyticsTable(err) {
		rows = nil
	}
	months := make([]MonthlyMetric, 6)
	for i := range months {
		monthStart := time.Date(startMonth.Year(), startMonth.Month()+time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)
		months[i].Month = monthStart.Format("2006-01")
		if s.social != nil {
			count, err := s.social.FollowerCountBetween(ctx, channelID, monthStart, monthEnd)
			if err == nil {
				months[i].Subscribers = count
			}
		}
	}
	for _, row := range rows {
		idx := monthIndex(startMonth, row.CreatedAt)
		if idx >= 0 && idx < len(months) {
			months[idx].RevenueCoin += row.Amount
		}
	}
	for _, room := range rooms {
		idx := monthIndex(startMonth, room.StartedAt)
		if idx < 0 || idx >= len(months) {
			continue
		}
		months[idx].Streams++
		endedAt := endedAtOf(room)
		dur := endedAt.Sub(room.StartedAt)
		if dur > 0 {
			months[idx].WatchHours += int64(dur.Hours())
		}
		peak := room.PeakViewers
		if room.Viewers > peak {
			peak = room.Viewers
		}
		if peak > months[idx].PeakViewers {
			months[idx].PeakViewers = peak
		}
	}
	totals := CreatorAnalyticsResp{}
	for _, month := range months {
		totals.RevenueCoin += month.RevenueCoin
		totals.Streams += month.Streams
		totals.WatchHours += month.WatchHours
		if month.PeakViewers > totals.PeakViewers {
			totals.PeakViewers = month.PeakViewers
		}
	}
	return months, totals
}

func (s *RoomService) fanBadgeDistribution(ctx context.Context, ownerID string) ([]FanBadgeDistributionBucket, error) {
	buckets := []FanBadgeDistributionBucket{
		{Bucket: "under20", MinLevel: 1, MaxLevel: 19},
		{Bucket: "level20To39", MinLevel: 20, MaxLevel: 39},
		{Bucket: "level40To59", MinLevel: 40, MaxLevel: 59},
		{Bucket: "level60Plus", MinLevel: 60},
	}
	rows, err := s.rooms.FanBadgeDistribution(ctx, ownerID)
	if err != nil {
		if isMissingAnalyticsTable(err) {
			return buckets, nil
		}
		return nil, err
	}
	byBucket := map[string]repo.FanBadgeDistributionRow{}
	for _, row := range rows {
		byBucket[row.Bucket] = row
	}
	for i := range buckets {
		row, ok := byBucket[buckets[i].Bucket]
		if !ok {
			continue
		}
		buckets[i].FanCount = row.FanCount
		buckets[i].TotalContribution = row.TotalContribution
	}
	return buckets, nil
}

// revenueTotals returns successful gift and super chat income by room id,
// treating missing order tables as no income.
func (s *RoomService) revenueTotals(ctx context.Context, roomIDs []string) (map[string]repo.RevenueTotals, error) {
	totals, err := s.rooms.RevenueTotalsByRooms(ctx, roomIDs)
	if err != nil && isMissingAnalyticsTable(err) {
		return map[string]repo.RevenueTotals{}, nil
	}
	return totals, err
}

// topFansByRooms returns each room's top limit fans, highest spend first. The
// aggregation and ranking run in SQL, so the cost doesn't grow with the number
// of orders loaded into the service.
func (s *RoomService) topFansByRooms(ctx context.Context, roomIDs []string, limit int) (map[string][]FanContribution, error) {
	out := map[string][]FanContribution{}
	rows, err := s.rooms.TopFansByRooms(ctx, roomIDs, limit)
	if err != nil {
		if isMissingAnalyticsTable(err) {
			return out, nil
		}
		return nil, err
	}
	for _, row := range rows {
		out[row.RoomID] = append(out[row.RoomID], FanContribution{
			UserID: row.UserID,
			Name:   row.UserName,
			Avatar: row.Avatar,
			Amount: row.Amount,
		})
	}
	return out, nil
}

func (s *RoomService) subscribersBetween(ctx context.Context, channelID string, start, end time.Time) int64 {
	if s.social == nil {
		return 0
	}
	count, err := s.social.FollowerCountBetween(ctx, channelID, start, end)
	if err != nil {
		return 0
	}
	return count
}

func endedAtOf(room model.Room) time.Time {
	if room.EndedAt != nil {
		return *room.EndedAt
	}
	return room.UpdatedAt
}

func monthIndex(start time.Time, value time.Time) int {
	return (value.Year()-start.Year())*12 + int(value.Month()-start.Month())
}

func isMissingAnalyticsTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist")
}

func hotReplayScore(likes, comments, revenue, peak, maxLikes, maxComments, maxRevenue, maxPeak int64) float64 {
	return normalized(likes, maxLikes)*0.35 +
		normalized(comments, maxComments)*0.25 +
		normalized(revenue, maxRevenue)*0.25 +
		normalized(peak, maxPeak)*0.15
}

func normalized(value, maxValue int64) float64 {
	if value <= 0 || maxValue <= 0 {
		return 0
	}
	return float64(value) / float64(maxValue)
}

func maxInt64(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
