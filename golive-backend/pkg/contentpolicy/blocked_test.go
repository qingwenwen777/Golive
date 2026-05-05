package contentpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeText_RemovesWhitespaceAndLowercases(t *testing.T) {
	require.Equal(t, "helloworldlive", NormalizeText("  HeLLo \n\tWorld Live "))
	require.Empty(t, NormalizeText(" \n\t "))
}

func TestHit_MatchesAcrossSpacingAndSkipsBlankWords(t *testing.T) {
	words := []string{"", "  Bad Word  ", "other"}

	require.Equal(t, "bad word", Hit("this contains b a d   w o r d", words))
	require.True(t, Contains("BADWORD appears after normalization", words))
}

func TestHit_ReturnsEmptyForEmptyInputOrNoMatches(t *testing.T) {
	require.Empty(t, Hit("", []string{"bad"}))
	require.Empty(t, Hit("safe message", nil))
	require.Empty(t, Hit("safe message", []string{"bad"}))
	require.False(t, Contains("safe message", []string{"bad"}))
}
