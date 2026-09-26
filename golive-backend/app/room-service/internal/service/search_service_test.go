package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

func TestSuggestUsesCheapSourcesAndPerViewerReplayVisibility(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	for id, update := range map[string]func(*model.Room){
		"live-07":   func(r *model.Room) { r.Title = "Zebra jam" },
		"replay-07": func(r *model.Room) { r.Title = "Zebra jam replay" },
		"replay-08": func(r *model.Room) {
			r.Title = "Zebra followers"
			r.ReplayVisibility = model.PostVisibilityFollowers
		},
	} {
		room, err := f.rooms.GetByID(ctx, id)
		require.NoError(t, err)
		update(room)
		require.NoError(t, f.rooms.Upsert(ctx, room))
	}
	values := func(resp *SearchSuggestionResp) []string {
		out := []string{}
		for _, item := range resp.Items {
			out = append(out, item.Type+":"+item.Value)
		}
		return out
	}

	anonymous, err := f.search.Suggest(ctx, "", "Zebra", 20)
	require.NoError(t, err)
	require.Equal(t, "Zebra", anonymous.Query)
	require.Equal(t, []string{
		"query:Zebra",
		"creator:Creator", "creator:creator7", "creator:creator8",
		"live:Zebra jam", "creator:Creator 7",
		"replay:Zebra jam replay",
	}, values(anonymous))
	require.Equal(t, "Creator 7", anonymous.Items[4].Label, "live titles carry the owner's current name")

	require.NoError(t, f.social.Follow(ctx, f.viewerID, "ch-"+f.owners[8]))
	f.counter.reset()
	follower, err := f.search.Suggest(ctx, f.viewerID, "zebra", 20)
	require.NoError(t, err)
	require.Zero(t, f.counter.sql.Load(), "the query's candidates are cached case-insensitively")
	require.Equal(t, "zebra", follower.Query)
	require.Contains(t, values(follower), "replay:Zebra followers", "followers-only replays are checked per viewer")

	limited, err := f.search.Suggest(ctx, "", "zebra", 3)
	require.NoError(t, err)
	require.Len(t, limited.Items, 3)

	short, err := f.search.Suggest(ctx, "", "z", 20)
	require.NoError(t, err)
	require.Equal(t, []string{"query:z"}, values(short), "one-character fallback queries don't scan")
}
