package service

import (
	"sort"
	"strings"
	"unicode"

	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func normalizeSearchSize(size, fallback, max int) int {
	if size < 1 {
		return fallback
	}
	if size > max {
		return max
	}
	return size
}

func searchScore(phrase repo.SearchPhrase, values ...string) int {
	if phrase.Empty() {
		return 0
	}
	raw := strings.ToLower(phrase.Raw)
	best := 0
	for _, value := range values {
		text := strings.ToLower(strings.TrimSpace(value))
		if text == "" {
			continue
		}
		score := 0
		switch {
		case text == raw:
			score += 1000
		case strings.HasPrefix(text, raw):
			score += 850
		case strings.Contains(text, raw):
			score += 620
		}
		compact := compactText(text)
		if phrase.Compact != "" {
			switch {
			case compact == phrase.Compact:
				score += 780
			case strings.HasPrefix(compact, phrase.Compact):
				score += 620
			case strings.Contains(compact, phrase.Compact):
				score += 440
			}
		}
		tokenHits := 0
		for _, token := range phrase.Tokens {
			switch {
			case strings.HasPrefix(text, token):
				score += 180
				tokenHits++
			case strings.Contains(text, token):
				score += 110
				tokenHits++
			case strings.Contains(compact, compactText(token)):
				score += 80
				tokenHits++
			}
		}
		if tokenHits == len(phrase.Tokens) && tokenHits > 0 {
			score += 240
		}
		if score > best {
			best = score
		}
	}
	return best
}

func sortBySearchScore[T any](items []T, score func(T) int, newer func(T, T) bool) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := score(items[i]), score(items[j])
		if left == right {
			if newer == nil {
				return false
			}
			return newer(items[i], items[j])
		}
		return left > right
	})
}

func compactText(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) || r == '_' || r == '-' || r == '.' || r == '·' || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func trimSuggestion(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > 64 {
		return string(runes[:64])
	}
	return value
}
