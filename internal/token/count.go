package token

import (
	"regexp"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

const (
	GPT2       = tokenizer.GPT2Enc
	R50kBase   = tokenizer.R50kBase
	P50kBase   = tokenizer.P50kBase
	P50kEdit   = tokenizer.P50kEdit
	Cl100kBase = tokenizer.Cl100kBase
	O200kBase  = tokenizer.O200kBase
)

var (
	fallbackRE = regexp.MustCompile(`[A-Za-z0-9]+|[\x{4e00}-\x{9fff}]|[^\s]`)
	codecCache sync.Map
)

func getCodec(enc tokenizer.Encoding) (tokenizer.Codec, error) {
	v, ok := codecCache.Load(enc)
	if ok {
		return v.(tokenizer.Codec), nil
	}
	codec, err := tokenizer.Get(enc)
	if err != nil {
		return nil, err
	}
	codecCache.Store(enc, codec)
	return codec, nil
}

// Encode returns the individual token strings for text using Cl100kBase.
// Falls back to regex split on error.
func Encode(enc tokenizer.Encoding, text string) []string {
	codec, err := getCodec(enc)
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
func Count(enc tokenizer.Encoding, text string) int {
	codec, err := getCodec(enc)
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
