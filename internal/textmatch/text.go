package textmatch

import (
	"strings"
	"unicode"
)

// Normalize lowercases Unicode letters without changing separators or spacing.
func Normalize(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, r := range value {
		normalized.WriteRune(unicode.ToLower(r))
	}
	return normalized.String()
}

// Tokens splits text on non-alphanumeric separators and camel-case boundaries.
// Tokens are normalized to lowercase Unicode text.
func Tokens(value string) []string {
	runes := []rune(value)
	tokens := make([]string, 0, 8)
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		tokens = append(tokens, token.String())
		token.Reset()
	}
	for i, current := range runes {
		if !unicode.IsLetter(current) && !unicode.IsNumber(current) {
			flush()
			continue
		}
		startsWord := token.Len() > 0 && unicode.IsUpper(current) &&
			(i > 0 && unicode.IsLower(runes[i-1]) || i+1 < len(runes) && unicode.IsLower(runes[i+1]))
		if startsWord {
			flush()
		}
		token.WriteRune(unicode.ToLower(current))
	}
	flush()
	return tokens
}

// ContainsTokenSequence reports whether sequence occurs contiguously in tokens.
func ContainsTokenSequence(tokens, sequence []string) bool {
	if len(sequence) == 0 || len(sequence) > len(tokens) {
		return false
	}
	for start := 0; start <= len(tokens)-len(sequence); start++ {
		matches := true
		for offset := range sequence {
			if tokens[start+offset] != sequence[offset] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}
