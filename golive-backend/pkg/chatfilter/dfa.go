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
//     combining marks on alphabetic letters are dropped ("fúck"), and case
//     is folded. Output keeps the original text outside masked spans.
//   - longest-match preferred when overlapping prefixes (e.g. ["sh", "shit"] →
//     "shit" wins over "sh")
//   - optional skip-character set (e.g. spaces / dots / asterisks ignored
//     while matching, so "s.h.i.t" still hits)
//   - word boundaries for alphabetic scripts: short words (≤ 3 letters, e.g.
//     "sb") only match as whole words so "usb" stays intact, and a match that
//     skipped separator characters must start at a word boundary so it cannot
//     straddle two words ("this bad" is not "s b"). CJK has no word
//     boundaries and always matches as a substring.
package chatfilter

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// DefaultSkipChars are the separators both services ignore inside a word.
const DefaultSkipChars = " .*-_"

// strictWordLen is the longest alphabetic word that must match as a whole
// word. Short entries are substrings of too many innocent words.
const strictWordLen = 3

// node is one trie vertex.
type node struct {
	children map[rune]*node
	end      bool
	wordLen  int  // rune length of the matched word ending here
	strict   bool // word must stand alone (short alphabetic word)
}

// Filter is a built, immutable DFA. Safe for concurrent use.
type Filter struct {
	root *node
	mask string
	skip map[rune]struct{}
}

// Option configures a Filter at build time.
type Option func(*Filter)

// WithMask sets the replacement string. Default "***".
func WithMask(s string) Option { return func(f *Filter) { f.mask = s } }

// WithSkipChars marks runes that may appear inside a word and be ignored
// during matching (typical bypass attempts: spaces, dots, asterisks).
// Pass an empty string to disable.
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
	for _, w := range words {
		f.addWord(w)
	}
	return f
}

func (f *Filter) addWord(w string) {
	runes := normalize([]rune(strings.TrimSpace(w))).runes
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
	skipped := false
	for i := start; i < len(t); i++ {
		r := t[i]
		if f.skip != nil && cur != f.root {
			// Allow skipping inside a word, but not before any match
			// has started — otherwise "..." would always "match" empty.
			if _, sk := f.skip[r]; sk {
				skipped = true
				continue
			}
		}
		next, ok := cur.children[r]
		if !ok {
			break
		}
		cur = next
		if cur.end && acceptable(t, start, i, cur.strict, skipped) {
			best = i + 1
			// keep going — there might be a longer word continuing past here
		}
	}
	return best
}

// acceptable applies the word-boundary rules to a candidate match t[start..last].
func acceptable(t []rune, start, last int, strict, skipped bool) bool {
	leftOK := start == 0 || !isWordRune(t[start-1])
	if strict {
		rightOK := last+1 >= len(t) || !isWordRune(t[last+1])
		return leftOK && rightOK
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
			n.runes = append(n.runes, unicode.ToLower(d))
			n.src = append(n.src, i)
		}
	}
	return n
}

// ignorable reports invisible runes commonly inserted to split a word:
// format characters (zero-width space/joiners, soft hyphen, BOM, bidi
// controls), variation selectors and the combining grapheme joiner.
func ignorable(r rune) bool {
	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Variation_Selector, r) || r == '\u034f'
}
