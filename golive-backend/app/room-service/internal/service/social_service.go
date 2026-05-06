package service

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type SocialService struct {
	social *repo.SocialRepo
	rooms  *repo.RoomRepo
	blocks ChannelBlockChecker
	notify NotificationWriter
}

func NewSocialService(social *repo.SocialRepo, rooms ...*repo.RoomRepo) *SocialService {
	var roomRepo *repo.RoomRepo
	if len(rooms) > 0 {
		roomRepo = rooms[0]
	}
	return &SocialService{social: social, rooms: roomRepo}
}

func (s *SocialService) SetBlockChecker(blocks ChannelBlockChecker) {
	s.blocks = blocks
}

func (s *SocialService) SetNotificationWriter(writer NotificationWriter) {
	s.notify = writer
}

// Follow state ---------------------------------------------------------

type FollowState struct {
	ChannelID       string `json:"channelId"`
	Following       bool   `json:"following"`
	SubscriberCount int64  `json:"subscriberCount"`
}

func (s *SocialService) GetFollow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	selfChannel, err := s.isSelfChannel(ctx, uid, channelID)
	if err != nil {
		return nil, err
	}
	on := false
	if uid != "" && !selfChannel {
		on, err = s.social.IsFollowing(ctx, uid, channelID)
		if err != nil {
			return nil, err
		}
	}
	count, err := s.social.FollowerCount(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if selfChannel {
		selfFollowing, err := s.social.IsFollowing(ctx, uid, channelID)
		if err != nil {
			return nil, err
		}
		if selfFollowing && count > 0 {
			count--
		}
	}
	return &FollowState{ChannelID: channelID, Following: on, SubscriberCount: count}, nil
}

func (s *SocialService) Follow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	selfChannel, err := s.isSelfChannel(ctx, uid, channelID)
	if err != nil {
		return nil, err
	}
	if selfChannel {
		return nil, errcode.New(409, "cannot follow your own channel").WithReason("self_follow")
	}
	if ownerID := ownerIDFromChannelID(channelID); ownerID != "" && s.blocks != nil {
		blocked, err := s.blocks.BlocksInteraction(ctx, uid, ownerID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, errcode.New(403, "blocked from this channel").WithReason("channel_blocked")
		}
	}
	if err := s.social.Follow(ctx, uid, channelID); err != nil {
		return nil, err
	}
	return s.GetFollow(ctx, uid, channelID)
}

func (s *SocialService) Unfollow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	if err := s.social.Unfollow(ctx, uid, channelID); err != nil {
		return nil, err
	}
	return s.GetFollow(ctx, uid, channelID)
}

type SubscriptionChannel struct {
	Key             string        `json:"key"`
	ChannelID       string        `json:"channelId"`
	Name            string        `json:"name"`
	Avatar          string        `json:"avatar"`
	Verified        bool          `json:"verified"`
	Live            bool          `json:"live"`
	Status          string        `json:"status"`
	SubscriberCount int64         `json:"subscriberCount"`
	Stream          *model.Stream `json:"stream,omitempty"`
}

type SubscriptionsResp struct {
	Items []SubscriptionChannel `json:"items"`
}

type CreatorRecommendation struct {
	ID              string  `json:"id"`
	ChannelID       string  `json:"channelId"`
	Name            string  `json:"name"`
	Avatar          string  `json:"avatar"`
	Verified        bool    `json:"verified"`
	SubscriberCount int64   `json:"subscriberCount"`
	LastLiveAt      string  `json:"lastLiveAt,omitempty"`
	LastTitle       string  `json:"lastTitle,omitempty"`
	Following       bool    `json:"following"`
	Score           float64 `json:"-"`
}

type CreatorRecommendationsResp struct {
	Items []CreatorRecommendation `json:"items"`
}

type CreatorSearchItem struct {
	ID              string `json:"id"`
	ChannelID       string `json:"channelId"`
	Username        string `json:"username,omitempty"`
	Name            string `json:"name"`
	Avatar          string `json:"avatar"`
	Cover           string `json:"cover,omitempty"`
	Verified        bool   `json:"verified"`
	SubscriberCount int64  `json:"subscriberCount"`
	Live            bool   `json:"live"`
	LiveRoomID      string `json:"liveRoomId,omitempty"`
	LastLiveAt      string `json:"lastLiveAt,omitempty"`
	LastTitle       string `json:"lastTitle,omitempty"`
	Following       bool   `json:"following"`
	Self            bool   `json:"self"`
	Score           int    `json:"-"`
}

func (s *SocialService) ListSubscriptions(ctx context.Context, uid string) (*SubscriptionsResp, error) {
	channelIDs, err := s.social.Following(ctx, uid)
	if err != nil {
		return nil, err
	}
	latest := map[string]model.Room{}
	if s.rooms != nil {
		latest, err = s.rooms.LatestByChannelIDs(ctx, channelIDs)
		if err != nil {
			return nil, err
		}
	}

	items := make([]SubscriptionChannel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		selfChannel, err := s.isSelfChannel(ctx, uid, channelID)
		if err != nil {
			return nil, err
		}
		if selfChannel {
			continue
		}
		count, err := s.social.FollowerCount(ctx, channelID)
		if err != nil {
			return nil, err
		}
		profile, hasProfile, err := s.ownerProfileForChannel(ctx, channelID)
		if err != nil {
			return nil, err
		}
		name := fallbackChannelName(channelID)
		avatar := ""
		verified := false
		if hasProfile {
			name = ownerProfileName(profile)
			avatar = strings.TrimSpace(profile.Avatar)
			verified = profile.Verified
		}
		item := SubscriptionChannel{
			Key:             channelID,
			ChannelID:       channelID,
			Name:            name,
			Avatar:          avatar,
			Verified:        verified,
			Status:          model.StatusEnded,
			SubscriberCount: count,
		}
		if room, ok := latest[channelID]; ok {
			stream := room.ToStream(time.Now())
			stream.SubscriberCount = count
			item.Key = room.ChannelID
			item.Live = room.Status == model.StatusLive
			item.Status = room.Status
			item.Stream = &stream
			if !hasProfile {
				item.Name = fallbackStreamChannelName(room)
				item.Avatar = strings.TrimSpace(room.Avatar)
				item.Verified = room.Verified
			}
		}
		if strings.TrimSpace(item.Avatar) == "" {
			item.Avatar = generatedAvatar(item.Name)
		}
		if item.Stream == nil {
			item.Stream = &model.Stream{
				ID:              channelID,
				Channel:         item.Name,
				ChannelID:       channelID,
				Avatar:          item.Avatar,
				Verified:        item.Verified,
				Status:          model.StatusEnded,
				IsLive:          false,
				SubscriberCount: count,
			}
		} else {
			item.Stream.Channel = item.Name
			item.Stream.ChannelID = item.ChannelID
			item.Stream.Avatar = item.Avatar
			item.Stream.Verified = item.Verified
		}
		items = append(items, item)
	}
	return &SubscriptionsResp{Items: items}, nil
}

func (s *SocialService) RecommendedCreators(ctx context.Context, uid string, limit int, rawCategory ...string) (*CreatorRecommendationsResp, error) {
	if limit < 1 {
		limit = 8
	}
	if limit > 24 {
		limit = 24
	}
	if s.rooms == nil {
		return &CreatorRecommendationsResp{Items: []CreatorRecommendation{}}, nil
	}
	category := ""
	if len(rawCategory) > 0 {
		category = NormalizeCategory(rawCategory[0])
	}

	candidates, err := s.rooms.CreatorRecommendationCandidates(ctx, limit*4, category)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	items := make([]CreatorRecommendation, 0, len(candidates))
	for _, candidate := range candidates {
		if uid != "" && candidate.ID == uid {
			continue
		}
		channelID := strings.TrimSpace(candidate.ChannelID)
		if channelID == "" {
			channelID = "ch-" + candidate.ID
		}
		name := recommendedCreatorName(candidate)
		subscriberCount, err := s.social.FollowerCount(ctx, channelID)
		if err != nil {
			return nil, err
		}
		following, err := s.social.IsFollowing(ctx, uid, channelID)
		if err != nil {
			return nil, err
		}

		item := CreatorRecommendation{
			ID:              candidate.ID,
			ChannelID:       channelID,
			Name:            name,
			Avatar:          strings.TrimSpace(candidate.Avatar),
			Verified:        candidate.Verified,
			SubscriberCount: subscriberCount,
			LastTitle:       strings.TrimSpace(candidate.LastTitle),
			Following:       following,
			Score:           recommendationScore(candidate, subscriberCount, following, now),
		}
		if item.Avatar == "" {
			item.Avatar = generatedAvatar(name)
		}
		if candidate.LastLiveAt != nil && !candidate.LastLiveAt.IsZero() {
			item.LastLiveAt = candidate.LastLiveAt.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Name < items[j].Name
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return &CreatorRecommendationsResp{Items: items}, nil
}

func (s *SocialService) SearchCreators(ctx context.Context, uid, query string, limit int) ([]CreatorSearchItem, error) {
	limit = normalizeSearchSize(limit, 8, 24)
	if s.rooms == nil || s.social == nil {
		return []CreatorSearchItem{}, nil
	}
	phrase := repo.NewSearchPhrase(query)
	if phrase.Empty() {
		return []CreatorSearchItem{}, nil
	}
	rows, err := s.rooms.SearchCreators(ctx, phrase, limit*6)
	if err != nil {
		return nil, err
	}
	items := make([]CreatorSearchItem, 0, len(rows))
	for _, row := range rows {
		channelID := strings.TrimSpace(row.ChannelID)
		if channelID == "" {
			channelID = "ch-" + row.ID
		}
		name := creatorSearchName(row)
		if name == "" {
			name = fallbackChannelName(channelID)
		}
		count, err := s.social.FollowerCount(ctx, channelID)
		if err != nil {
			return nil, err
		}
		self := uid != "" && uid == row.ID
		following := false
		if uid != "" && !self {
			following, err = s.social.IsFollowing(ctx, uid, channelID)
			if err != nil {
				return nil, err
			}
		}
		item := CreatorSearchItem{
			ID:              row.ID,
			ChannelID:       channelID,
			Username:        strings.TrimSpace(row.Username),
			Name:            name,
			Avatar:          strings.TrimSpace(row.Avatar),
			Cover:           strings.TrimSpace(row.Cover),
			Verified:        row.Verified,
			SubscriberCount: count,
			Live:            row.LiveRoomID != "",
			LiveRoomID:      row.LiveRoomID,
			LastTitle:       strings.TrimSpace(row.LastTitle),
			Following:       following,
			Self:            self,
		}
		if item.Avatar == "" {
			item.Avatar = generatedAvatar(name)
		}
		if row.LastLiveAt != nil && !row.LastLiveAt.IsZero() {
			item.LastLiveAt = row.LastLiveAt.UTC().Format(time.RFC3339)
		}
		item.Score = searchScore(phrase, item.Name, item.Username, item.ChannelID, item.LastTitle)
		if item.Live {
			item.Score += 80
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			if items[i].Live != items[j].Live {
				return items[i].Live
			}
			return items[i].SubscriberCount > items[j].SubscriberCount
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func creatorSearchName(row repo.CreatorSearchRow) string {
	for _, value := range []string{row.Channel, row.DisplayName, row.Username} {
		name := strings.TrimSpace(value)
		if name != "" && !repo.IsUUIDLike(name) {
			return name
		}
	}
	return ""
}

func recommendedCreatorName(candidate repo.CreatorRecommendationCandidate) string {
	for _, value := range []string{candidate.Channel, candidate.DisplayName, candidate.Username} {
		if name := strings.TrimSpace(value); name != "" && !repo.IsUUIDLike(name) {
			return name
		}
	}
	id := candidate.ID
	if len(id) > 8 {
		id = id[:8]
	}
	if id == "" {
		return "Creator"
	}
	return "Creator " + id
}

func recommendationScore(candidate repo.CreatorRecommendationCandidate, subscriberCount int64, following bool, now time.Time) float64 {
	score := float64(subscriberCount)*8 + float64(candidate.StreamCount)*3 + float64(candidate.PeakViewers)*0.05
	if candidate.LastLiveAt != nil && !candidate.LastLiveAt.IsZero() {
		age := now.Sub(*candidate.LastLiveAt)
		switch {
		case age <= 7*24*time.Hour:
			score += 30
		case age <= 30*24*time.Hour:
			score += 18
		case age <= 90*24*time.Hour:
			score += 8
		default:
			score += 2
		}
	} else {
		score += 1
	}
	if !following {
		score += 6
	}
	return score
}

func (s *SocialService) isSelfChannel(ctx context.Context, uid, channelID string) (bool, error) {
	if uid == "" || channelID == "" {
		return false, nil
	}
	if ownerIDFromChannelID(channelID) == uid {
		return true, nil
	}
	if s.rooms == nil {
		return false, nil
	}
	ownerID, err := s.rooms.ResolveOwnerID(ctx, channelID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ownerID == uid, nil
}

func (s *SocialService) ownerProfileForChannel(ctx context.Context, channelID string) (repo.OwnerProfile, bool, error) {
	if s.rooms == nil || strings.TrimSpace(channelID) == "" {
		return repo.OwnerProfile{}, false, nil
	}
	ownerID := ownerIDFromChannelID(channelID)
	if ownerID == "" {
		resolved, err := s.rooms.ResolveOwnerID(ctx, channelID)
		if errors.Is(err, repo.ErrRoomNotFound) {
			return repo.OwnerProfile{}, false, nil
		}
		if err != nil {
			return repo.OwnerProfile{}, false, err
		}
		ownerID = resolved
	}
	profile, err := s.rooms.OwnerProfile(ctx, ownerID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return repo.OwnerProfile{}, false, nil
	}
	if err != nil {
		return repo.OwnerProfile{}, false, err
	}
	return profile, true, nil
}

func ownerIDFromChannelID(channelID string) string {
	if strings.HasPrefix(channelID, "ch-") {
		return strings.TrimPrefix(channelID, "ch-")
	}
	return ""
}

func fallbackChannelName(channelID string) string {
	id := strings.TrimPrefix(channelID, "ch-")
	if len(id) > 8 {
		id = id[:8]
	}
	if id == "" {
		return "Creator"
	}
	return "Creator " + id
}

func ownerProfileName(profile repo.OwnerProfile) string {
	for _, value := range []string{profile.DisplayName, profile.Username} {
		name := strings.TrimSpace(value)
		if name != "" && !repo.IsUUIDLike(name) {
			return name
		}
	}
	id := strings.TrimSpace(profile.ID)
	if len(id) > 8 {
		id = id[:8]
	}
	if id == "" {
		return "Creator"
	}
	return "Creator " + id
}

func generatedAvatar(name string) string {
	seed := strings.TrimSpace(name)
	if seed == "" {
		seed = "Creator"
	}
	return "https://api.dicebear.com/7.x/initials/svg?seed=" + url.QueryEscape(seed)
}

func fallbackStreamChannelName(room model.Room) string {
	name := strings.TrimSpace(room.Channel)
	if name != "" {
		return name
	}
	return fallbackChannelName(room.ChannelID)
}

// Like state -----------------------------------------------------------

type LikeState struct {
	StreamID string `json:"streamId"`
	Liked    bool   `json:"liked"`
	Disliked bool   `json:"disliked"`
	Likes    int64  `json:"likes"`
}

func (s *SocialService) GetLike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	st, err := s.social.GetLike(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Like(ctx context.Context, uid, streamID string) (*LikeState, error) {
	if err := s.ensureRoomInteractionAllowed(ctx, uid, streamID); err != nil {
		return nil, err
	}
	prev, _ := s.social.GetLike(ctx, streamID, uid)
	st, err := s.social.Like(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	if prev == nil || !prev.Liked {
		_ = s.notifyRoomLiked(ctx, uid, streamID)
	}
	if s.rooms != nil {
		_ = s.saveLikedLibraryItem(ctx, uid, streamID)
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Unlike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	st, err := s.social.Unlike(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	if s.rooms != nil {
		_ = s.removeLikedLibraryItem(ctx, uid, streamID)
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Dislike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	if err := s.ensureRoomInteractionAllowed(ctx, uid, streamID); err != nil {
		return nil, err
	}
	st, err := s.social.Dislike(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	if s.rooms != nil {
		_ = s.removeLikedLibraryItem(ctx, uid, streamID)
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Undislike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	st, err := s.social.Undislike(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	return toLikeState(streamID, st), nil
}

func toLikeState(sid string, st *repo.LikeState) *LikeState {
	return &LikeState{
		StreamID: sid,
		Liked:    st.Liked,
		Disliked: st.Disliked,
		Likes:    st.Likes,
	}
}

func (s *SocialService) ensureRoomInteractionAllowed(ctx context.Context, uid, streamID string) error {
	if s.blocks == nil || s.rooms == nil || uid == "" {
		return nil
	}
	room, err := s.rooms.GetByID(ctx, streamID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	blocked, err := s.blocks.BlocksInteraction(ctx, uid, room.OwnerID)
	if err != nil {
		return err
	}
	if blocked {
		return errcode.New(403, "blocked from this channel").WithReason("channel_blocked")
	}
	return nil
}

func (s *SocialService) notifyRoomLiked(ctx context.Context, actorID, streamID string) error {
	if s.notify == nil || s.rooms == nil || actorID == "" {
		return nil
	}
	room, err := s.rooms.GetByID(ctx, streamID)
	if err != nil || room == nil || room.OwnerID == "" || room.OwnerID == actorID {
		return nil
	}
	profile, _ := s.rooms.OwnerProfile(ctx, actorID)
	return s.notify.CreateNotifications(ctx, []model.Notification{{
		ID:            "room-liked-" + notificationHash(streamID, actorID),
		UserID:        room.OwnerID,
		Type:          "room_liked",
		Title:         "你的直播间收到新的赞",
		Body:          strings.TrimSpace(room.Title),
		Link:          "/live/" + room.ID,
		ActorID:       actorID,
		ActorUsername: profile.Username,
		ActorName:     ownerProfileName(profile),
		ActorAvatar:   profile.Avatar,
		ActorVerified: profile.Verified,
		CreatedAt:     time.Now(),
	}})
}
