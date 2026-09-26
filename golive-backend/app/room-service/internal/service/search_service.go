package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

type SearchService struct {
	rooms        *RoomService
	social       *SocialService
	posts        *PostService
	appointments *AppointmentService
	suggestions  *ttlCache[*suggestionPool]
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
	return &SearchService{
		rooms:        rooms,
		social:       social,
		posts:        posts,
		appointments: appointments,
		suggestions:  newTTLCache[*suggestionPool](suggestCacheTTL, suggestCacheEntries),
	}
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

const (
	suggestCacheTTL     = 10 * time.Second
	suggestCacheEntries = 4096
	// Suggest runs on every keystroke, so it reads a few candidates from the
	// cheap sources only (creators, live rooms, replays) and skips the
	// per-result enrichment Search does.
	suggestCandidates = 12
	suggestPerSource  = 4
)

// suggestionPool is the viewer-independent part of Suggest for one query,
// cached briefly and shared, so it must not be modified. Only replay
// visibility depends on the viewer.
type suggestionPool struct {
	creators []SearchSuggestion
	live     []SearchSuggestion
	replays  []suggestedReplay // ranked candidates, filtered per viewer
}

type suggestedReplay struct {
	room    model.Room
	channel string
}

func (s *SearchService) Suggest(ctx context.Context, viewerID, query string, limit int) (*SearchSuggestionResp, error) {
	phrase := repo.NewSearchPhrase(query)
	limit = normalizeSearchSize(limit, 12, 20)
	out := &SearchSuggestionResp{Query: phrase.Raw, Items: []SearchSuggestion{}}
	if phrase.Empty() {
		return out, nil
	}
	pool, err := s.suggestions.get(strings.ToLower(phrase.Raw), func() (*suggestionPool, error) {
		return s.loadSuggestionPool(ctx, phrase)
	})
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
	for _, item := range pool.creators {
		add(item.Value, item.Type, item.Label)
	}
	for _, item := range pool.live {
		add(item.Value, item.Type, item.Label)
	}
	replays := 0
	for _, candidate := range pool.replays {
		if replays >= suggestPerSource || len(out.Items) >= limit {
			break
		}
		replay, err := s.rooms.replay.ReplayDTO(ctx, candidate.room, viewerID)
		if err != nil {
			return nil, err
		}
		if replay == nil || !replay.CanWatch {
			continue
		}
		replays++
		add(candidate.room.Title, "replay", candidate.channel)
	}
	return out, nil
}

func (s *SearchService) loadSuggestionPool(ctx context.Context, phrase repo.SearchPhrase) (*suggestionPool, error) {
	pool := &suggestionPool{}
	if s.social != nil && s.social.rooms != nil {
		rows, err := s.social.rooms.SearchCreators(ctx, phrase, suggestCandidates)
		if err != nil {
			return nil, err
		}
		pool.creators = creatorSuggestions(phrase, rows)
	}
	if s.rooms == nil {
		return pool, nil
	}
	liveRooms, err := s.rooms.rooms.SearchLiveRooms(ctx, phrase, suggestCandidates)
	if err != nil {
		return nil, err
	}
	sortRoomsBySearch(liveRooms, phrase)
	if len(liveRooms) > suggestPerSource {
		liveRooms = liveRooms[:suggestPerSource]
	}
	var replayRooms []model.Room
	if s.rooms.replay != nil {
		replayRooms, err = s.rooms.rooms.SearchReplayRooms(ctx, phrase, suggestCandidates)
		if err != nil {
			return nil, err
		}
		sortRoomsBySearch(replayRooms, phrase)
	}
	// Titles are labelled with the owner's current name, as in Search.
	profiles := roomLookups{profiles: s.rooms.ownerProfiles(ctx, append(append([]model.Room{}, liveRooms...), replayRooms...))}
	for _, room := range liveRooms {
		profiles.apply(&room)
		pool.live = append(pool.live,
			SearchSuggestion{Value: room.Title, Type: "live", Label: room.Channel},
			SearchSuggestion{Value: room.Channel, Type: "creator", Label: room.Title},
		)
	}
	for _, room := range replayRooms {
		channel := room
		profiles.apply(&channel)
		pool.replays = append(pool.replays, suggestedReplay{room: room, channel: channel.Channel})
	}
	return pool, nil
}

// creatorSuggestions ranks creator rows as SearchCreators does, without the
// follower lookups, and returns name and username suggestions for the best.
func creatorSuggestions(phrase repo.SearchPhrase, rows []repo.CreatorSearchRow) []SearchSuggestion {
	type ranked struct {
		name, username string
		live           bool
		score          int
	}
	items := make([]ranked, 0, len(rows))
	for _, row := range rows {
		channelID := strings.TrimSpace(row.ChannelID)
		if channelID == "" {
			channelID = "ch-" + row.ID
		}
		name := creatorSearchName(row)
		if name == "" {
			name = fallbackChannelName(channelID)
		}
		item := ranked{name: name, username: strings.TrimSpace(row.Username), live: row.LiveRoomID != ""}
		item.score = searchScore(phrase, item.name, item.username, channelID, strings.TrimSpace(row.LastTitle))
		if item.live {
			item.score += 80
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].live && !items[j].live
		}
		return items[i].score > items[j].score
	})
	if len(items) > suggestPerSource {
		items = items[:suggestPerSource]
	}
	out := make([]SearchSuggestion, 0, len(items)*2)
	for _, item := range items {
		out = append(out,
			SearchSuggestion{Value: item.name, Type: "creator", Label: item.username},
			SearchSuggestion{Value: item.username, Type: "creator", Label: item.name},
		)
	}
	return out
}
