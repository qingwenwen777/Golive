// Package chatfilter implements a DFA-based sensitive word filter shared by
// im-gateway (live chat path) and chat-service (Kafka path).
//
// Why DFA over a regex / strings.Contains loop:
//
//	N words, M chars in text, average word length L:
//	  naive scan      → O(N*M)
//	  DFA / trie scan → O(M*L) amortized, single pass over the text
//
// We build a trie keyed by rune (so CJK is first-class). Each text scan
// walks the trie from every starting position; on the longest match we
// emit a mask and resume right after the match (so consecutive hits are
// independent).
//
// Features:
//
//   - rune-level (UTF-8 safe, CJK supported)
//   - normalisation before matching, so trivial bypasses fail: zero-width /
//     format characters and variation selectors are ignored, compatibility
//     forms are folded (NFKD: fullwidth "ｆｕｃｋ", math bold, ligatures),
//     combining marks on alphabetic letters are dropped ("fúck"), Cyrillic
//     and Greek letters that look Latin are folded to it ("fuсk" with a
//     Cyrillic с), and case is folded. Output keeps the original text outside
//     masked spans. List entries are normalised the same way.
//   - longest-match preferred when overlapping prefixes (e.g. ["sh", "shit"] →
//     "shit" wins over "sh")
//   - optional separator skipping: whitespace, punctuation and control
//     characters inside a word are ignored while matching, so "s.h.i.t",
//     "f,u,c,k" and "傻、逼" still hit. They are dropped from list entries
//     too, so "kill yourself" also matches "killyourself".
//   - word boundaries for alphabetic scripts: short words (≤ 3 letters, e.g.
//     "sb") only match as whole words so "usb" stays intact, and a match that
//     skipped separator characters must start at a word boundary so it cannot
//     straddle two words ("this bad" is not "s b"). One that skipped
//     whitespace spans words, so it must end at a boundary too ("an
//     alternative" is not "anal"). An apostrophe between letters belongs to
//     the word ("let's hit" is not "s hit"). CJK has no word boundaries and
//     always matches as a substring.
package chatfilter

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// DefaultSkipChars turns on separator skipping for both services. Every
// whitespace, punctuation and control character is skipped once skipping is
// on (see WithSkipChars); these are the common ones.
const DefaultSkipChars = " .*-_"

// strictWordLen is the longest alphabetic word that must match as a whole
// word. Short entries are substrings of too many innocent words.
const strictWordLen = 3

// node is one trie vertex.
type node struct {
	children map[rune]*node
	end      bool
	wordLen  int  // rune length of the matched word ending here
	alpha    bool // word is all word runes, so word boundaries apply
	strict   bool // word must stand alone (short alphabetic word)
}

// Filter is a built, immutable DFA. Safe for concurrent use.
type Filter struct {
	root *node
	mask string
	skip map[rune]struct{}
	// asciiSkip caches skipClass for ASCII runes, which the trie walk tests
	// at every step.
	asciiSkip [utf8.RuneSelf]uint8
}

// skipClass bits.
const (
	skipRune  = 1 << iota // may be skipped inside a match
	skipSpace             // whitespace: a match skipping it spans words
)

// Option configures a Filter at build time.
type Option func(*Filter)

// WithMask sets the replacement string. Default "***".
func WithMask(s string) Option { return func(f *Filter) { f.mask = s } }

// WithSkipChars turns on skipping of separators inside a word (typical
// bypass attempts: spaces, dots, asterisks, commas). Separators are chars
// plus every Unicode whitespace, punctuation and control character, except
// apostrophes, which belong to the word ("let's"). Symbols such as "$" are
// not separators: leet entries like "$hit" keep them. Pass an empty string
// to disable skipping.
func WithSkipChars(chars string) Option {
	return func(f *Filter) {
		if chars == "" {
			f.skip = nil
			return
		}
		f.skip = make(map[rune]struct{}, len([]rune(chars)))
		for _, r := range chars {
			f.skip[r] = struct{}{}
		}
	}
}

// New builds a filter from a list of words. Empty/whitespace-only words and
// duplicates are silently ignored.
func New(words []string, opts ...Option) *Filter {
	f := &Filter{
		root: &node{children: map[rune]*node{}},
		mask: "***",
	}
	for _, o := range opts {
		o(f)
	}
	for r := range f.asciiSkip {
		f.asciiSkip[r] = f.skipClass(rune(r))
	}
	for _, w := range words {
		f.addWord(w)
	}
	return f
}

func (f *Filter) addWord(w string) {
	var runes []rune
	for _, r := range normalize([]rune(strings.TrimSpace(w))).runes {
		// Separators are skipped in the text, so an entry that kept one
		// ("kill yourself", "hand-job") could never match.
		if skip, _ := f.skippable(r); !skip {
			runes = append(runes, r)
		}
	}
	if len(runes) == 0 {
		return
	}
	cur := f.root
	alphabetic := true
	for _, r := range runes {
		next, ok := cur.children[r]
		if !ok {
			next = &node{children: map[rune]*node{}}
			cur.children[r] = next
		}
		cur = next
		if !isWordRune(r) {
			alphabetic = false
		}
	}
	cur.end = true
	cur.wordLen = len(runes)
	cur.alpha = alphabetic
	cur.strict = alphabetic && len(runes) <= strictWordLen
}

// HasMatch reports whether text contains any sensitive word.
func (f *Filter) HasMatch(text string) bool {
	if text == "" || f.root == nil || len(f.root.children) == 0 {
		return false
	}
	t := normalize([]rune(text)).runes
	for i := range t {
		if f.longestMatchAt(t, i) > 0 {
			return true
		}
	}
	return false
}

// Replace returns text with every match swapped for the mask.
//
// Behavior on overlap: prefer the longest match starting at each position.
// After a match, resume scanning at the position immediately after it
// (matches do not overlap with each other in the output).
func (f *Filter) Replace(text string) string {
	if text == "" || f.root == nil || len(f.root.children) == 0 {
		return text
	}
	orig := []rune(text)
	t := normalize(orig)
	var out strings.Builder
	out.Grow(len(text))

	copied := 0 // original runes before this index are already written
	for i := 0; i < len(t.runes); {
		end := f.longestMatchAt(t.runes, i)
		if end == 0 {
			i++
			continue
		}
		first, last := t.src[i], t.src[end-1]
		out.WriteString(string(orig[copied:first]))
		out.WriteString(f.mask)
		copied = last + 1
		for i < len(t.runes) && t.src[i] <= last {
			i++
		}
	}
	out.WriteString(string(orig[copied:]))
	return out.String()
}

// longestMatchAt walks the trie starting at t[start] and returns the
// exclusive end index (in t) of the longest acceptable match, or 0 if none.
// Skip characters inside the match are covered by it; trailing ones are not.
func (f *Filter) longestMatchAt(t []rune, start int) int {
	cur := f.root
	best := 0
	skipped, spaced := false, false
	for i := start; i < len(t); i++ {
		r := t[i]
		if cur != f.root {
			// Allow skipping inside a word, but not before any match
			// has started — otherwise "..." would always "match" empty.
			if sk, space := f.skippable(r); sk {
				skipped = true
				spaced = spaced || space
				continue
			}
		}
		next, ok := cur.children[r]
		if !ok {
			break
		}
		cur = next
		if cur.end && acceptable(t, start, i, cur, skipped, spaced) {
			best = i + 1
			// keep going — there might be a longer word continuing past here
		}
	}
	return best
}

// acceptable applies the word-boundary rules to a candidate match
// t[start..last] of the word ending at n. skipped reports whether the match
// skipped separators, spaced whether any of them was whitespace.
func acceptable(t []rune, start, last int, n *node, skipped, spaced bool) bool {
	if !n.alpha {
		return true
	}
	leftOK := !wordAt(t, start-1)
	if n.strict || spaced {
		return leftOK && !wordAt(t, last+1)
	}
	if skipped {
		return leftOK
	}
	return true
}

// isWordRune reports whether r belongs to a word in a script that separates
// words with spaces. CJK and other scripts without spaces are excluded so
// their matches are never boundary-restricted.
func isWordRune(r rune) bool {
	if r < utf8.RuneSelf {
		return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
	}
	if unicode.IsDigit(r) {
		return true
	}
	return unicode.IsLetter(r) && unicode.In(r, unicode.Latin, unicode.Greek, unicode.Cyrillic)
}

// wordAt reports whether t[i] is part of a word: a word rune, or an
// apostrophe between two of them ("let's", "It’s"). Out of range is not.
func wordAt(t []rune, i int) bool {
	if i < 0 || i >= len(t) {
		return false
	}
	if isWordRune(t[i]) {
		return true
	}
	return isApostrophe(t[i]) && i > 0 && i+1 < len(t) && isWordRune(t[i-1]) && isWordRune(t[i+1])
}

// isApostrophe reports the runes typed as an apostrophe inside words.
func isApostrophe(r rune) bool {
	switch r {
	case '\'', '’', '‘', 'ʼ', '`':
		return true
	}
	return false
}

// skippable reports whether r may be skipped inside a match, and whether it
// is whitespace, which means the match spans words.
func (f *Filter) skippable(r rune) (skip, space bool) {
	var c uint8
	if r < utf8.RuneSelf {
		c = f.asciiSkip[r]
	} else {
		c = f.skipClass(r)
	}
	return c&skipRune != 0, c&skipSpace != 0
}

// skipClass is skippable's answer for r as skipRune/skipSpace bits.
func (f *Filter) skipClass(r rune) uint8 {
	if f.skip == nil {
		return 0
	}
	if !separator(r) {
		if _, ok := f.skip[r]; !ok {
			return 0
		}
	}
	if unicode.IsSpace(r) {
		return skipRune | skipSpace
	}
	return skipRune
}

// separator reports whitespace, punctuation and control characters, which can
// split a word without changing it. Apostrophes belong to their word.
func separator(r rune) bool {
	return (unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsControl(r)) && !isApostrophe(r)
}

// normalized is the folded form of a text used for matching.
type normalized struct {
	runes []rune // folded runes
	src   []int  // src[i] = index of the original rune runes[i] came from
}

// normalize folds text for matching. Each original rune maps to zero or more
// folded runes so matches can be mapped back onto the original text.
func normalize(orig []rune) normalized {
	n := normalized{
		runes: make([]rune, 0, len(orig)),
		src:   make([]int, 0, len(orig)),
	}
	for i, r := range orig {
		if r < utf8.RuneSelf {
			if 'A' <= r && r <= 'Z' {
				r += 'a' - 'A'
			}
			n.runes = append(n.runes, r)
			n.src = append(n.src, i)
			continue
		}
		if ignorable(r) {
			continue
		}
		for _, d := range norm.NFKD.String(string(r)) {
			// Combining marks on alphabetic letters ("fúck", zalgo text) are
			// decoration; on other scripts (kana voicing marks) they matter.
			if unicode.Is(unicode.Mn, d) && len(n.runes) > 0 && isWordRune(n.runes[len(n.runes)-1]) {
				continue
			}
			n.runes = append(n.runes, fold(d))
			n.src = append(n.src, i)
		}
	}
	return n
}

// fold lower-cases r, mapping a Cyrillic or Greek letter that looks Latin to
// that Latin letter first.
func fold(r rune) rune {
	if 0x0370 <= r && r <= 0x052f { // Greek, Cyrillic and Cyrillic Supplement
		if l, ok := lookalikes[r]; ok {
			return l
		}
	}
	return unicode.ToLower(r)
}

// lookalikes maps Cyrillic and Greek letters to the Latin letter they are
// read as ("fuсk" with a Cyrillic с, "ѕhit" with a Cyrillic ѕ). Capitals and
// small letters map separately where their shapes differ (Greek Η reads as
// h, η as n). Real Cyrillic or Greek words don't fold into Latin entries:
// they would need a look-alike for every letter.
var lookalikes = map[rune]rune{
	// Cyrillic
	'А': 'a', 'а': 'a', 'В': 'b', 'в': 'b', 'Е': 'e', 'е': 'e',
	'К': 'k', 'к': 'k', 'М': 'm', 'м': 'm', 'Н': 'h', 'н': 'h',
	'О': 'o', 'о': 'o', 'Р': 'p', 'р': 'p', 'С': 'c', 'с': 'c',
	'Т': 't', 'т': 't', 'У': 'y', 'у': 'y', 'Х': 'x', 'х': 'x',
	'Ѕ': 's', 'ѕ': 's', 'І': 'i', 'і': 'i', 'Ј': 'j', 'ј': 'j',
	'Һ': 'h', 'һ': 'h', 'Ӏ': 'l', 'ӏ': 'l', 'Ү': 'y', 'ү': 'y',
	'Ԁ': 'd', 'ԁ': 'd', 'Ԛ': 'q', 'ԛ': 'q', 'Ԝ': 'w', 'ԝ': 'w',
	// Greek
	'Α': 'a', 'α': 'a', 'Β': 'b', 'β': 'b', 'Ε': 'e', 'ε': 'e',
	'Ζ': 'z', 'Η': 'h', 'η': 'n', 'Ι': 'i', 'ι': 'i', 'Κ': 'k',
	'κ': 'k', 'Μ': 'm', 'Ν': 'n', 'ν': 'v', 'Ο': 'o', 'ο': 'o',
	'Ρ': 'p', 'ρ': 'p', 'Τ': 't', 'τ': 't', 'Υ': 'y', 'υ': 'u',
	'Χ': 'x', 'χ': 'x', 'γ': 'y', 'ω': 'w', 'Ϲ': 'c', 'ϲ': 'c',
}

// ignorable reports invisible runes commonly inserted to split a word:
// format characters (zero-width space/joiners, soft hyphen, BOM, bidi
// controls), variation selectors and the combining grapheme joiner.
func ignorable(r rune) bool {
	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Variation_Selector, r) || r == '\u034f'
}
