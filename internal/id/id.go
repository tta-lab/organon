package id

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Hash returns a stable 2-char base62 ID for the given text.
func Hash(text string) string {
	return toBase62(hash64(text), 2)
}

// AssignIDs generates stable 2-char base62 IDs for a list of labels.
// On collision, extends to 3 chars with positional disambiguator.
// Mirrors flicknote's algorithm: hash heading text, detect duplicates,
// rehash colliders with null-separated position index.
func AssignIDs(labels []string) []string {
	ids := make([]string, len(labels))
	for i, label := range labels {
		ids[i] = Hash(label)
	}

	counts := map[string]int{}
	for _, id := range ids {
		counts[id]++
	}

	for i, id := range ids {
		if counts[id] > 1 {
			disambiguated := fmt.Sprintf("%s\x00%d", labels[i], i)
			ids[i] = toBase62(hash64(disambiguated), 3)
		}
	}
	return ids
}

func hash64(s string) uint64 {
	h := sha256.Sum256([]byte(s))
	var n uint64
	for _, b := range h[:8] {
		n = n<<8 | uint64(b)
	}
	return n
}

// toBase62 encodes n as a base62 string of exactly exactLen characters.
// The output is padded with '0' on the left if shorter, truncated on the left if longer.
func toBase62(n uint64, exactLen int) string {
	if n == 0 {
		return strings.Repeat("0", exactLen)
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base62Chars[n%62]
		n /= 62
	}
	s := string(buf[i:])
	// Pad left if shorter than exactLen
	for len(s) < exactLen {
		s = "0" + s
	}
	// Truncate to exactLen (take rightmost chars, like low-order digits)
	if len(s) > exactLen {
		s = s[len(s)-exactLen:]
	}
	return s
}
