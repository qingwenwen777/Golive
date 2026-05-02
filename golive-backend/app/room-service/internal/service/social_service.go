package service

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

type SocialService struct {
	social *repo.SocialRepo
	rooms  *repo.RoomRepo
}

func NewSocialService(social *repo.SocialRepo, rooms ...*repo.RoomRepo) *SocialService {
	var roomRepo *repo.RoomRepo
	if len(rooms) > 0 {
		roomRepo = rooms[0]
	}
	return &SocialService{social: social, rooms: roomRepo}
}

// Follow state ---------------------------------------------------------

type FollowState struct {
	ChannelID       string `json:"channelId"`
	Following       bool   `json:"following"`
	SubscriberCount int64  `json:"subscriberCount"`
}

func (s *SocialService) GetFollow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	on, err := s.social.IsFollowing(ctx, uid, channelID)
	if err != nil {
		return nil, err
	}
	count, err := s.social.FollowerCount(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return &FollowState{ChannelID: channelID, Following: on, SubscriberCount: count}, nil
}

func (s *SocialService) Follow(ctx context.Context, uid, channelID string) (*FollowState, error) {
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
		count, err := s.social.FollowerCount(ctx, channelID)
		if err != nil {
			return nil, err
		}
		item := SubscriptionChannel{
			Key:             channelID,
			ChannelID:       channelID,
			Name:            fallbackChannelName(channelID),
			Status:          model.StatusEnded,
			SubscriberCount: count,
		}
		if room, ok := latest[channelID]; ok {
			stream := room.ToStream(time.Now())
			stream.SubscriberCount = count
			item.Key = room.ChannelID
			item.Name = fallbackStreamChannelName(room)
			item.Avatar = room.Avatar
			item.Verified = room.Verified
			item.Live = room.Status == model.StatusLive
			item.Status = room.Status
			item.Stream = &stream
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
				Status:          model.StatusEnded,
				IsLive:          false,
				SubscriberCount: count,
			}
		} else if strings.TrimSpace(item.Stream.Avatar) == "" {
			item.Stream.Avatar = item.Avatar
		}
		items = append(items, item)
	}
	return &SubscriptionsResp{Items: items}, nil
}

func (s *SocialService) RecommendedCreators(ctx context.Context, uid string, limit int) (*CreatorRecommendationsResp, error) {
	if limit < 1 {
		limit = 8
	}
	if limit > 24 {
		limit = 24
	}
	if s.rooms == nil {
		return &CreatorRecommendationsResp{Items: []CreatorRecommendation{}}, nil
	}

	candidates, err := s.rooms.CreatorRecommendationCandidates(ctx, limit*4)
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
	st, err := s.social.Like(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Unlike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	st, err := s.social.Unlike(ctx, streamID, uid)
	if err != nil {
		return nil, err
	}
	return toLikeState(streamID, st), nil
}

func (s *SocialService) Dislike(ctx context.Context, uid, streamID string) (*LikeState, error) {
	st, err := s.social.Dislike(ctx, streamID, uid)
	if err != nil {
		return nil, err
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
