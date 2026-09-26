package chatfilter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	filter "github.com/qingwenwen777/golive/pkg/chatfilter"
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
	// Words of four letters or more still match inside longer words.
	require.Equal(t, "mother***er", f.Replace("motherfucker"))
}

func TestFilter_LongestMatchWins(t *testing.T) {
	// "fuck" is a prefix of "fucking"; "fucking" must win at the same start.
	f := filter.New([]string{"fuck", "fucking"}, filter.WithMask("[X]"))
	require.Equal(t, "[X]", f.Replace("fucking"))
	require.Equal(t, "[X] up", f.Replace("fucking up"))
	// Bare "fuck" still hits where "fucking" can't continue.
	require.Equal(t, "[X]ed", f.Replace("fucked"))
}

func TestFilter_OverlappingNonOverlap(t *testing.T) {
	// After replacing one match, scanning resumes after it — matches don't
	// overlap in the output.
	f := filter.New([]string{"abcd", "dabc"}, filter.WithMask("*"))
	// "abcdabc" → match "abcd" at 0, resume at 4, "abc" no match → "*abc"
	require.Equal(t, "*abc", f.Replace("abcdabc"))
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

// Reviewer-found false positives: a short word matched inside an innocent
// word, and skip characters let a match straddle two words.
func TestFilter_WordBoundaries(t *testing.T) {
	f := filter.New([]string{"sb", "shit", "bad"}, filter.WithSkipChars(filter.DefaultSkipChars))

	require.Equal(t, "usb", f.Replace("usb"))
	require.Equal(t, "this is fine", f.Replace("this is fine"))
	require.Equal(t, "this hit", f.Replace("this hit"))
	require.False(t, f.HasMatch("usb cable"))

	// Short words still hit when they stand alone, including obfuscated.
	require.Equal(t, "you ***", f.Replace("you sb"))
	require.Equal(t, "*** !", f.Replace("SB !"))
	require.Equal(t, "***", f.Replace("s.b"))
	require.Equal(t, "you ***", f.Replace("you s b"))

	// "this bad": "bad" alone is still a hit, but "s b" across the space is not.
	require.Equal(t, "this ***", f.Replace("this bad"))
	require.Equal(t, "badge", f.Replace("badge"))
}

// Reviewer-found false positives: an apostrophe counted as a word boundary,
// so the "s" of "let's" started a word and "s hit" read as "shit".
func TestFilter_ApostrophesStayInsideWords(t *testing.T) {
	f := filter.New([]string{"shit", "sb"}, filter.WithSkipChars(filter.DefaultSkipChars))
	for _, in := range []string{
		"let's hit the road",
		"that's hit or miss",
		"Chris's hit song",
		"It’s hit the top 10",
		"what's hit points",
		"it's b",
	} {
		require.Equal(t, in, f.Replace(in))
		require.False(t, f.HasMatch(in), "%q", in)
	}
	// Quote marks around a word are not apostrophes.
	require.Equal(t, "'***'", f.Replace("'sb'"))
	require.Equal(t, "‘***’", f.Replace("‘s b’"))
}

// A match that skips whitespace spans words, so it must end at a word
// boundary as well as start at one: "an alternative" is not "an al" + "ternative".
func TestFilter_MatchAcrossWordsMustEndAtBoundary(t *testing.T) {
	f := filter.New([]string{"anal", "shit", "傻逼"}, filter.WithSkipChars(filter.DefaultSkipChars))
	require.Equal(t, "an alternative", f.Replace("an alternative"))
	require.Equal(t, "in an algorithm", f.Replace("in an algorithm"))
	require.False(t, f.HasMatch("an alarm"))

	// Spaced-out words still hit, with or without trailing text.
	require.Equal(t, "***", f.Replace("a n a l"))
	require.Equal(t, "*** happens", f.Replace("s h i t happens"))
	require.Equal(t, "***.s", f.Replace("s h i t.s"))
	// Inside one token the match may still run into a suffix.
	require.Equal(t, "***s", f.Replace("s.h.i.ts"))
	// CJK has no word boundaries, next to Latin text or not.
	require.Equal(t, "ok***lol", f.Replace("ok傻 逼lol"))
}

func TestFilter_Normalisation(t *testing.T) {
	f := filter.New([]string{"fuck", "傻逼"}, filter.WithSkipChars(filter.DefaultSkipChars))

	for _, in := range []string{
		"f\u200buck",       // zero-width space
		"fu\u00adck",       // soft hyphen
		"f\u200du\u200cck", // zero-width joiner / non-joiner
		"fu\ufeffck",       // BOM
		"ｆｕｃｋ",             // fullwidth
		"ＦＵＣＫ",             // fullwidth upper case
		"𝐟𝐮𝐜𝐤",             // mathematical bold
		"fúck",             // precomposed accent
		"fu\u0301ck",       // combining accent
		"f\ufe0fuck",       // variation selector
		"fu\u034fck",       // combining grapheme joiner
		"fu\u2060ck",       // word joiner
	} {
		require.Equal(t, "***", f.Replace(in), "%q", in)
		require.True(t, f.HasMatch(in), "%q", in)
	}

	require.Equal(t, "你***啊", f.Replace("你傻\u200b逼啊"))
	require.Equal(t, "***", f.Replace("傻\u3000逼"), "ideographic space folds to a skip char")

	// Text outside a match keeps its original form.
	require.Equal(t, "ｈｅｌｌｏ *** ｗｏｒｌｄ", f.Replace("ｈｅｌｌｏ ｆｕｃｋ ｗｏｒｌｄ"))
	require.Equal(t, "café ok", f.Replace("café ok"))
}

func TestFilter_WordListIsNormalised(t *testing.T) {
	f := filter.New([]string{"ＦＵＣＫ"})
	require.Equal(t, "***", f.Replace("fuck"))
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

func TestLoadWords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "words.txt")
	require.NoError(t, os.WriteFile(path, []byte("# comment\n\nfuck\n  sb  \n"), 0o600))
	words, err := filter.LoadWords(path)
	require.NoError(t, err)
	require.Equal(t, []string{"fuck", "sb"}, words)
}
