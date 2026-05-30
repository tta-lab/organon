package token

import (
	"regexp"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)
const (
	Cl100kBase = tokenizer.Cl100kBase
)

var (
	fallbackRE = regexp.MustCompile(`[A-Za-z0-9]+|[\x{4e00}-\x{9fff}]|[^\s]`)
	cl100k     = sync.OnceValues(func() (tokenizer.Codec, error) {
		return tokenizer.Get(Cl100kBase)
	})
)

// Encode returns the individual token strings for text using Cl100kBase.
// Falls back to regex split on error.
func Encode(text string) []string {
	codec, err := cl100k()
	if err != nil {
		return fallbackRE.FindAllString(text, -1)
	}
	_, names, err := codec.Encode(text)
	if err != nil {
		return fallbackRE.FindAllString(text, -1)
	}
	return names
}

// Count returns the token count of text using the default Cl100kBase tokenizer.
// Falls back to a simple regex split on error.
func Count(text string) int {
	codec, err := cl100k()
	if err != nil {
		return fallbackCount(text)
	}
	count, err := codec.Count(text)
	if err != nil {
		return fallbackCount(text)
	}
	return count
}
func fallbackCount(text string) int {
	return len(fallbackRE.FindAllString(text, -1))
}
