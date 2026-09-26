package service_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

func TestNormalizeCategory(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"   ":          "",
		"all":          "",
		"All":          "",
		"ALL":          "",
		"すべて":          "",
		"Music":        "Music",
		"Apex Legends": "Apex Legends",
		"音楽":           "音楽",
	}
	for in, want := range cases {
		require.Equalf(t, want, service.NormalizeCategory(in), "input=%q", in)
	}
}

// Make sure the JSON wire shape never includes streamKey when the field is
// empty (non-publisher view). This guards the contract literally — a stray
// `streamKey` ever leaking would let viewers republish.
func TestStream_StripsStreamKeyWhenEmpty(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Channel: "c", ChannelID: "ch", Category: "Gaming"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "streamKey")
}

func TestStream_IncludesStreamKeyForOwner(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Channel: "c", ChannelID: "ch", Category: "Gaming", StreamKey: "lk_xxx"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"streamKey":"lk_xxx"`)
}

func TestStream_IncludesDescription(t *testing.T) {
	st := model.Stream{ID: "a", Title: "t", Description: "creator intro", Channel: "c", ChannelID: "ch", Category: "Gaming"}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"description":"creator intro"`)
}
