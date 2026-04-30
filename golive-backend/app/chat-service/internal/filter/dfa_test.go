package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/chat-service/internal/filter"
)

func TestFilter_BasicHit(t *testing.T) {
	f := filter.New([]string{"fuck", "傻逼"})
	require.Equal(t, "I *** you", f.Replace("I fuck you"))
	require.Equal(t, "你这个***", f.Replace("你这个傻逼"))
	require.True(t, f.HasMatch("are you serious fuck off"))
	require.False(t, f.HasMatch("clean text"))
}

func TestFilter_CaseInsensitive(t *testing.T) {
	f := filter.New([]string{"fuck"})
	require.Equal(t, "***", f.Replace("FUCK"))
	require.Equal(t, "***", f.Replace("FuCk"))
	require.Equal(t, "***er", f.Replace("Fucker"))
}

func TestFilter_LongestMatchWins(t *testing.T) {
	// "sh" is a prefix of "shit"; "shit" must win at the same start position.
	f := filter.New([]string{"sh", "shit"}, filter.WithMask("[X]"))
	require.Equal(t, "[X]", f.Replace("shit"))
	require.Equal(t, "[X] up", f.Replace("shit up"))
	// Bare "sh" still hits where "shit" can't continue.
	require.Equal(t, "[X]e", f.Replace("she"))
}

func TestFilter_OverlappingNonOverlap(t *testing.T) {
	// After replacing one match, scanning resumes after it — matches don't
	// overlap in the output.
	f := filter.New([]string{"abc", "cab"}, filter.WithMask("*"))
	// "abcab" → match "abc" at 0, resume at 3, "ab" no match, output "*ab"
	require.Equal(t, "*ab", f.Replace("abcab"))
}

func TestFilter_EmptyAndNoWords(t *testing.T) {
	f := filter.New(nil)
	require.Equal(t, "anything", f.Replace("anything"))
	require.False(t, f.HasMatch("anything"))

	f = filter.New([]string{"  ", "", "ok"})
	require.Equal(t, "***", f.Replace("ok"))
}

func TestFilter_SkipChars(t *testing.T) {
	f := filter.New([]string{"shit"},
		filter.WithMask("###"),
		filter.WithSkipChars(" .*-"))
	// Skip chars only count when we're already mid-word, so a leading
	// dot does NOT spuriously match.
	require.Equal(t, "###", f.Replace("s.h.i.t"))
	require.Equal(t, "###", f.Replace("s h i t"))
	require.Equal(t, "say ###s", f.Replace("say s*h*i*ts"))

	// Without skip chars, the same input must NOT match.
	plain := filter.New([]string{"shit"})
	require.Equal(t, "s.h.i.t", plain.Replace("s.h.i.t"))
}

func TestFilter_MultipleHitsInOneText(t *testing.T) {
	f := filter.New([]string{"fuck", "shit"})
	require.Equal(t, "*** and *** again", f.Replace("fuck and shit again"))
}

func TestFilter_CJK_NoSkipBleed(t *testing.T) {
	// Make sure CJK words don't accidentally absorb neighbouring chars.
	f := filter.New([]string{"傻逼"})
	require.Equal(t, "你***啊", f.Replace("你傻逼啊"))
	require.Equal(t, "傻", f.Replace("傻"), "partial prefix must not match")
}

func TestFilter_HasMatch_PerformanceShape(t *testing.T) {
	// Build a moderately big dictionary; assert no panic on a long input.
	words := []string{}
	for r := 'a'; r <= 'z'; r++ {
		words = append(words, string([]rune{r, r, r}))
	}
	f := filter.New(words)
	long := ""
	for i := 0; i < 10000; i++ {
		long += "abcdefghij"
	}
	_ = f.HasMatch(long) // smoke
}
