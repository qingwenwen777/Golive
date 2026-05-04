package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func TestNewSearchPhraseNormalizesWhitespaceAndSeparators(t *testing.T) {
	phrase := repo.NewSearchPhrase("  yuuka_chan-815  ")
	require.Equal(t, "yuuka_chan-815", phrase.Raw)
	require.Equal(t, []string{"yuuka_chan-815"}, phrase.Tokens)
	require.Equal(t, "yuukachan815", phrase.Compact)
}

func TestSearchScorePrefersCompactMatches(t *testing.T) {
	phrase := repo.NewSearchPhrase("yuuka chan815")
	compactScore := searchScore(phrase, "yuuka_chan815")
	missScore := searchScore(phrase, "random creator")
	require.Greater(t, compactScore, missScore)
}
