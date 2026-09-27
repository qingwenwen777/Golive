package chatfilter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// Reviewer-found misses: separators are skipped in the text but were kept in
// list entries, so "kill yourself" and "hand-job" could never match.
func TestFilter_EntriesWithSeparatorsMatch(t *testing.T) {
	f := filter.New([]string{"kill yourself", "hand-job"}, filter.WithSkipChars(filter.DefaultSkipChars))
	for in, want := range map[string]string{
		"kill yourself":        "***",
		"just kill   yourself": "just ***",
		"killyourself":         "***",
		"a hand-job":           "a ***",
		"hand job":             "***",
		"handjob":              "***",
		"skill yourself":       "skill yourself",
	} {
		require.Equal(t, want, f.Replace(in), "%q", in)
	}

	// Without skipping, entries match exactly as listed.
	exact := filter.New([]string{"hand-job"})
	require.Equal(t, "a ***", exact.Replace("a hand-job"))
	require.Equal(t, "handjob", exact.Replace("handjob"))
}

// Reviewer-found misses: only " .*-_" were skipped, so any other punctuation
// or whitespace split a word unnoticed.
func TestFilter_AnyPunctuationOrWhitespaceSeparates(t *testing.T) {
	f := filter.New([]string{"fuck", "sb", "傻逼"}, filter.WithSkipChars(filter.DefaultSkipChars))
	for in, want := range map[string]string{
		"f,u,c,k":      "***",
		"f/u/c/k":      "***",
		"f\tuck":       "***",
		"f\nuck":       "***",
		"f\r\nu!c?k":   "***",
		"f…u…c…k":      "***",
		"(f)(u)(c)(k)": "(***)",
		"s　b":          "***",
		"s/b!":         "***!",
		"傻 逼":          "***",
		"傻,逼":          "***",
		"傻、逼":          "***",
		"傻，逼":          "***",
		"傻·逼":          "***",
	} {
		require.Equal(t, want, f.Replace(in), "%q", in)
		require.True(t, f.HasMatch(in), "%q", in)
	}
}

// Reviewer-found misses: Cyrillic and Greek letters that look Latin.
func TestFilter_LookalikeLettersFold(t *testing.T) {
	f := filter.New([]string{"fuck", "shit", "asshole"}, filter.WithSkipChars(filter.DefaultSkipChars))
	for _, in := range []string{
		"fuсk",    // Cyrillic es
		"ѕhit",    // Cyrillic dze
		"ЅНІТ",    // Cyrillic capitals
		"ѕніт",    // Cyrillic small letters shaped like small capitals
		"аѕѕһоlе", // Cyrillic a, dze, shha, o, ie
		"ΑSSΗΟLΕ", // Greek capitals
		"fυck",    // Greek upsilon
	} {
		require.Equal(t, "***", f.Replace(in), "%q", in)
	}
	// Real Cyrillic and Greek text is left alone.
	require.Equal(t, "Привет, как дела?", f.Replace("Привет, как дела?"))
	require.Equal(t, "Καλημέρα σας", f.Replace("Καλημέρα σας"))
	// Entries fold the same way.
	require.Equal(t, "***", filter.New([]string{"ѕhit"}).Replace("shit"))
}

// Text that must stay unmasked with the shipped list (plus "ass" to cover a
// short entry): apostrophes inside words, short entries inside longer words
// (the Scunthorpe problem), words that only meet across a space, and CJK
// text with punctuation. Four-letter entries still match inside longer words
// on purpose ("motherfucker"), so innocent words that contain one need an
// allow list, which the filter doesn't have.
func TestFilter_InnocentTextStaysUnmasked(t *testing.T) {
	f := filter.New([]string{"fuck", "shit", "bitch", "asshole", "傻逼", "垃圾", "sb", "nmsl", "ass"},
		filter.WithSkipChars(filter.DefaultSkipChars))
	for _, in := range []string{
		// Apostrophes
		"let's hit the road",
		"that's hit or miss",
		"Chris's hit song",
		"It’s hit the top 10",
		"what's hit points",
		"it's b",
		"he's b-list at best",
		"Shi'ite",
		// Short entries inside longer words
		"usb",
		"a USB-C cable",
		"flights to Lisbon",
		"/sbin/init",
		"classic bass passage",
		"the embassy's assistant",
		// Words that only meet across whitespace or punctuation
		"this hit",
		"class hole",
		"pass hole",
		"is b",
		"1s hit",
		"it's bad; she's back",
		// CJK with punctuation
		"你好，世界！",
		"今天天气很好。明天见",
		"他说：“好的”",
		"价格：100元/件",
		"一、二、三",
		"傻瓜，别逼我",
		"我不傻，你呢？",
	} {
		require.Equal(t, in, f.Replace(in), "%q", in)
		require.False(t, f.HasMatch(in), "%q", in)
	}
}

// Matching stays linear: every start walks at most the longest entry plus the
// separators inside it. Live chat is capped at 200 characters; this guards
// the filter against much longer input (chat-service, admin tools).
func TestFilter_LongAdversarialInputStaysFast(t *testing.T) {
	f := adversarialFilter()
	for name, in := range adversarialInputs() {
		start := time.Now()
		f.Replace(in)
		f.HasMatch(in)
		require.Less(t, time.Since(start), 2*time.Second, name)
	}
}

func BenchmarkReplace_Adversarial(b *testing.B) {
	f := adversarialFilter()
	for name, in := range adversarialInputs() {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				f.Replace(in)
			}
		})
	}
}

func BenchmarkReplace_Chat(b *testing.B) {
	f := adversarialFilter()
	in := "let's hit the road, you ｆｕｃｋ, 傻、逼 and ѕhit: plug in the usb"
	for b.Loop() {
		f.Replace(in)
	}
}

func adversarialFilter() *filter.Filter {
	return filter.New([]string{"fuck", "shit", "bitch", "asshole", "傻逼", "垃圾", "sb", "nmsl", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaab"},
		filter.WithSkipChars(filter.DefaultSkipChars))
}

// adversarialInputs are about 200,000 runes each.
func adversarialInputs() map[string]string {
	return map[string]string{
		"prefix-dots":     strings.Repeat("a.", 100_000),
		"prefix-runs":     strings.Repeat("a"+strings.Repeat(",", 999), 200),
		"f-space":         strings.Repeat("f ", 100_000),
		"one-long-gap":    "s" + strings.Repeat("\t", 200_000) + "b",
		"cjk-separators":  strings.Repeat("傻、", 100_000),
		"lookalikes":      strings.Repeat("ѕһіт ", 40_000),
		"apostrophe-runs": strings.Repeat("s's ", 50_000),
	}
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
