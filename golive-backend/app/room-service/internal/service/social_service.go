package service

import (
	"context"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

type SocialService struct {
	social *repo.SocialRepo
}

func NewSocialService(social *repo.SocialRepo) *SocialService {
	return &SocialService{social: social}
}

// Follow state ---------------------------------------------------------

type FollowState struct {
	ChannelID string `json:"channelId"`
	Following bool   `json:"following"`
}

func (s *SocialService) GetFollow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	on, err := s.social.IsFollowing(ctx, uid, channelID)
	if err != nil {
		return nil, err
	}
	return &FollowState{ChannelID: channelID, Following: on}, nil
}

func (s *SocialService) Follow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	if err := s.social.Follow(ctx, uid, channelID); err != nil {
		return nil, err
	}
	return &FollowState{ChannelID: channelID, Following: true}, nil
}

func (s *SocialService) Unfollow(ctx context.Context, uid, channelID string) (*FollowState, error) {
	if err := s.social.Unfollow(ctx, uid, channelID); err != nil {
		return nil, err
	}
	return &FollowState{ChannelID: channelID, Following: false}, nil
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
