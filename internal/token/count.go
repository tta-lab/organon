package token

import (
	"github.com/tiktoken-go/tokenizer"
	"regexp"
	"sync"
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
