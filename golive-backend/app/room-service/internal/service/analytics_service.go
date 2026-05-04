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

var ErrForbiddenAnalytics = errcode.New(403, "Forbidden")

type LiveHistoryResp struct {
	Items []LiveHistoryItem `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

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
	RevenueCoin     int64            `json:"revenueCoin"`
	NewSubscribers  int64            `json:"newSubscribers"`
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
	rows, err := s.revenueRows(ctx, []string{room.ID})
	if err != nil {
		return nil, err
	}
	danmuCounts, err := s.rooms.DanmuCountsByRooms(ctx, []string{room.ID})
	if err != nil {
		return nil, err
	}
	item := s.historyItem(*room, rows, danmuCounts[room.ID])
	item.NewSubscribers = s.subscribersBetween(ctx, room.ChannelID, room.StartedAt, endedAtOf(*room))
	topFans := topFans(rows, 8)
	giftRevenue, scRevenue := int64(0), int64(0)
	for _, row := range rows {
		if row.Kind == "super_chat" {
			scRevenue += row.Amount
		} else {
			giftRevenue += row.Amount
		}
	}
	return &LiveAnalysisResp{
		Record:           item,
		TopFans:          topFans,
		GiftRevenue:      giftRevenue,
		SuperChatRevenue: scRevenue,
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
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.ID)
	}
	rows, err := s.revenueRows(ctx, roomIDs)
	if err != nil {
		return nil, err
	}
	rowsByRoom := map[string][]repo.RevenueRow{}
	for _, row := range rows {
		rowsByRoom[row.RoomID] = append(rowsByRoom[row.RoomID], row)
	}
	danmuCounts, err := s.rooms.DanmuCountsByRooms(ctx, roomIDs)
	if err != nil {
		return nil, err
	}

	for _, room := range rooms {
		item := s.historyItem(room, rowsByRoom[room.ID], danmuCounts[room.ID])
		item.NewSubscribers = s.subscribersBetween(ctx, room.ChannelID, room.StartedAt, endedAtOf(room))
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

func (s *RoomService) historyItem(room model.Room, rows []repo.RevenueRow, danmuCount int64) LiveHistoryItem {
	endedAt := endedAtOf(room)
	duration := endedAt.Sub(room.StartedAt)
	if duration < 0 {
		duration = 0
	}
	revenue := int64(0)
	for _, row := range rows {
		revenue += row.Amount
	}
	fans := topFans(rows, 1)
	var topFan *FanContribution
	if len(fans) > 0 {
		topFan = &fans[0]
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
		RevenueCoin:     revenue,
		TopFan:          topFan,
	}
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

func (s *RoomService) revenueRows(ctx context.Context, roomIDs []string) ([]repo.RevenueRow, error) {
	rows, err := s.rooms.RevenueRowsByRooms(ctx, roomIDs)
	if err != nil && isMissingAnalyticsTable(err) {
		return nil, nil
	}
	return rows, err
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

func topFans(rows []repo.RevenueRow, limit int) []FanContribution {
	type agg struct {
		userID string
		name   string
		avatar string
		amount int64
	}
	byUser := map[string]*agg{}
	for _, row := range rows {
		key := row.UserID
		if key == "" {
			key = row.UserName
		}
		if key == "" {
			continue
		}
		item := byUser[key]
		if item == nil {
			item = &agg{userID: row.UserID, name: row.UserName, avatar: row.Avatar}
			byUser[key] = item
		}
		item.amount += row.Amount
	}
	out := make([]FanContribution, 0, len(byUser))
	for _, item := range byUser {
		out = append(out, FanContribution{
			UserID: item.userID,
			Name:   item.name,
			Avatar: item.avatar,
			Amount: item.amount,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Amount > out[i].Amount {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
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
