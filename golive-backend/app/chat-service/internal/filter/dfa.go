// Package filter implements a DFA-based sensitive word filter.
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
//   - case-insensitive on ASCII letters (Unicode case-folding skipped — overkill)
//   - longest-match preferred when overlapping prefixes (e.g. ["sh", "shit"] →
//     "shit" wins over "sh")
//   - optional skip-character set (e.g. spaces / dots / asterisks ignored
//     while matching, so "s.h.i.t" still hits)
package filter

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// node is one trie vertex.
type node struct {
	children map[rune]*node
	end      bool
	wordLen  int // rune length of the matched word ending here
}

// Filter is a built, immutable DFA. Safe for concurrent use.
type Filter struct {
	root *node
	mask string
	skip map[rune]struct{}
	once sync.Once
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
	w = strings.TrimSpace(w)
	if w == "" {
		return
	}
	cur := f.root
	count := 0
	for _, r := range w {
		r = lower(r)
		next, ok := cur.children[r]
		if !ok {
			next = &node{children: map[rune]*node{}}
			cur.children[r] = next
		}
		cur = next
		count++
	}
	cur.end = true
	cur.wordLen = count
}

// HasMatch reports whether text contains any sensitive word.
func (f *Filter) HasMatch(text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if matched, _ := f.longestMatchAt(runes, i); matched > 0 {
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
	runes := []rune(text)
	var out strings.Builder
	out.Grow(len(text))

	for i := 0; i < len(runes); {
		consumed, matched := f.longestMatchAt(runes, i)
		if matched > 0 {
			out.WriteString(f.mask)
			i += consumed
			continue
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

// longestMatchAt walks the trie starting at runes[i] and returns
// (consumed, matchedRunes) where:
//
//	consumed     — how many input runes the match covered (incl. skip chars)
//	matchedRunes — rune length of the longest matched WORD (0 if none)
func (f *Filter) longestMatchAt(runes []rune, start int) (consumed, matchedRunes int) {
	cur := f.root
	bestConsumed, bestMatched := 0, 0
	for i := start; i < len(runes); i++ {
		r := runes[i]
		if f.skip != nil {
			if _, sk := f.skip[r]; sk && cur != f.root {
				// Allow skipping inside a word, but not before any match
				// has started — otherwise "..." would always "match" empty.
				continue
			}
		}
		next, ok := cur.children[lower(r)]
		if !ok {
			break
		}
		cur = next
		if cur.end {
			bestConsumed = i - start + 1
			bestMatched = cur.wordLen
			// keep going — there might be a longer word continuing past here
		}
	}
	return bestConsumed, bestMatched
}

// lower returns the lowercase variant for ASCII letters; non-ASCII passes
// through (Unicode case folding intentionally not applied — see package doc).
func lower(r rune) rune {
	if r < utf8.RuneSelf && r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	if r < 0x80 {
		return r
	}
	if unicode.IsUpper(r) {
		return unicode.ToLower(r)
	}
	return r
}
