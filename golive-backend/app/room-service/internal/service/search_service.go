package service

import (
	"context"
	"strings"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

type SearchService struct {
	rooms        *RoomService
	social       *SocialService
	posts        *PostService
	appointments *AppointmentService
}

type SearchResp struct {
	Query        string              `json:"query"`
	Creators     []CreatorSearchItem `json:"creators"`
	Live         []model.Stream      `json:"live"`
	Replays      []model.Stream      `json:"replays"`
	Appointments []AppointmentDTO    `json:"appointments"`
	Posts        []ChannelPostDTO    `json:"posts"`
	Total        int                 `json:"total"`
}

type SearchSuggestion struct {
	Value string `json:"value"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

type SearchSuggestionResp struct {
	Query string             `json:"query"`
	Items []SearchSuggestion `json:"items"`
}

func NewSearchService(rooms *RoomService, social *SocialService, posts *PostService, appointments *AppointmentService) *SearchService {
	return &SearchService{rooms: rooms, social: social, posts: posts, appointments: appointments}
}

func (s *SearchService) Search(ctx context.Context, viewerID, query string, size int) (*SearchResp, error) {
	phrase := repo.NewSearchPhrase(query)
	size = normalizeSearchSize(size, 8, 24)
	resp := &SearchResp{
		Query:        phrase.Raw,
		Creators:     []CreatorSearchItem{},
		Live:         []model.Stream{},
		Replays:      []model.Stream{},
		Appointments: []AppointmentDTO{},
		Posts:        []ChannelPostDTO{},
	}
	if phrase.Empty() {
		return resp, nil
	}

	var err error
	if s.social != nil {
		resp.Creators, err = s.social.SearchCreators(ctx, viewerID, phrase.Raw, size)
		if err != nil {
			return nil, err
		}
	}
	if s.rooms != nil {
		resp.Live, err = s.rooms.SearchLive(ctx, phrase.Raw, size)
		if err != nil {
			return nil, err
		}
		resp.Replays, err = s.rooms.SearchReplays(ctx, viewerID, phrase.Raw, size)
		if err != nil {
			return nil, err
		}
	}
	if s.appointments != nil {
		resp.Appointments, err = s.appointments.SearchUpcoming(ctx, viewerID, phrase.Raw, size)
		if err != nil {
			return nil, err
		}
	}
	if s.posts != nil {
		resp.Posts, err = s.posts.Search(ctx, viewerID, phrase.Raw, size)
		if err != nil {
			return nil, err
		}
	}
	resp.Total = len(resp.Creators) + len(resp.Live) + len(resp.Replays) + len(resp.Appointments) + len(resp.Posts)
	return resp, nil
}

func (s *SearchService) Suggest(ctx context.Context, viewerID, query string, limit int) (*SearchSuggestionResp, error) {
	phrase := repo.NewSearchPhrase(query)
	limit = normalizeSearchSize(limit, 12, 20)
	out := &SearchSuggestionResp{Query: phrase.Raw, Items: []SearchSuggestion{}}
	if phrase.Empty() {
		return out, nil
	}
	results, err := s.Search(ctx, viewerID, phrase.Raw, 4)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	add := func(value, typ, label string) {
		if len(out.Items) >= limit {
			return
		}
		value = trimSuggestion(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			return
		}
		seen[key] = true
		out.Items = append(out.Items, SearchSuggestion{Value: value, Type: typ, Label: label})
	}

	add(phrase.Raw, "query", "")
	for _, creator := range results.Creators {
		add(creator.Name, "creator", creator.Username)
		add(creator.Username, "creator", creator.Name)
	}
	for _, stream := range results.Live {
		add(stream.Title, "live", stream.Channel)
		add(stream.Channel, "creator", stream.Title)
	}
	for _, replay := range results.Replays {
		add(replay.Title, "replay", replay.Channel)
	}
	for _, appointment := range results.Appointments {
		add(appointment.Title, "appointment", appointment.Channel)
	}
	for _, post := range results.Posts {
		add(post.Author.Name, "creator", "")
		add(post.Content, "post", post.Author.Name)
	}
	return out, nil
}
