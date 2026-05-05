package contentpolicy

import (
	"strings"
	"unicode"
)

const (
	RedisBlockedWordsKey = "content:blocked_words"
	RedisSiteBanPrefix   = "site:ban:"
	RedisSiteMutePrefix  = "site:mute:"
)

func NormalizeWord(word string) string {
	return strings.ToLower(strings.TrimSpace(word))
}

func NormalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func Hit(text string, words []string) string {
	normalizedText := NormalizeText(text)
	if normalizedText == "" || len(words) == 0 {
		return ""
	}
	for _, word := range words {
		normalizedWord := NormalizeWord(word)
		if normalizedWord == "" {
			continue
		}
		if strings.Contains(normalizedText, NormalizeText(normalizedWord)) {
			return normalizedWord
		}
	}
	return ""
}

func Contains(text string, words []string) bool {
	return Hit(text, words) != ""
}
