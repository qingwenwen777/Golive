package service

import (
	"context"
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
			item.Key = room.ChannelID
			item.Name = fallbackStreamChannelName(room)
			item.Avatar = room.Avatar
			item.Verified = room.Verified
			item.Live = room.Status == model.StatusLive
			item.Status = room.Status
			item.Stream = &stream
		}
		items = append(items, item)
	}
	return &SubscriptionsResp{Items: items}, nil
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
