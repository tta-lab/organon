package srcop

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// BatchEdit is one exact text replacement in a batch. Both fields may contain
// multiline text; matching happens against the original file content.
type BatchEdit struct {
	OldText string
	NewText string
}

// BatchEditResult is the atomic result of a batch edit.
type BatchEditResult struct {
	Content          []byte // the new file bytes (BOM and line endings preserved)
	FirstChangedLine int    // 1-indexed line of the first change in the new file
	Diff             string // display-oriented numbered diff for Pi renderers
	Patch            string // standard unified patch for machine consumers
}

// batchMatch is one validated replacement located against the original content.
type batchMatch struct {
	start, end int
	newText    string
	index      int // 1-based user-facing edit number
}

// ApplyBatch applies several disjoint exact replacements to source in one
// atomic operation. Every OldText must identify exactly one unique region in
// the original content; all replacements are located against that original
// content, never against content produced by preceding entries. Overlapping,
// nested, empty, duplicate, missing, and no-op edits are rejected before any
// write. BOM and CRLF line endings are preserved. Unlike Edit, ApplyBatch does
// not impose the single-edit 100-KB file cap: it must not reject ordinary text
// files earlier than the replaced Pi built-in edit.
func ApplyBatch(filename string, source []byte, edits []BatchEdit) (*BatchEditResult, error) {
	if len(edits) == 0 {
		return nil, fmt.Errorf("at least one edit is required")
	}
	if isBinary(source) {
		return nil, fmt.Errorf("binary file detected; src edit only works on text files")
	}

	// Preserve BOM and normalize line endings for matching and application,
	// mirroring the Pi built-in edit: matching always runs on LF, and the final
	// file is restored to the line-ending style of the FIRST line ending in the
	// original file.
	var bom []byte
	content := source
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		bom, content = content[:3], content[3:]
	}
	crlfEnding := firstLineEndingIsCRLF(content)
	normalized := []byte(normalizeLineEndings(string(content)))

	matches := make([]batchMatch, 0, len(edits))
	for i, edit := range edits {
		// Both sides are normalized to LF for matching and application; the
		// final restore must never see a literal \r\n from newText, or it
		// would become \r\r\n.
		old := normalizeLineEndings(edit.OldText)
		newText := normalizeLineEndings(edit.NewText)
		if old == "" {
			return nil, fmt.Errorf("edit %d: oldText is empty", i+1)
		}
		if old == newText {
			return nil, fmt.Errorf("edit %d: oldText and newText are identical (no-op)", i+1)
		}
		occurrences := allIndexes(normalized, []byte(old))
		switch len(occurrences) {
		case 0:
			return nil, fmt.Errorf("edit %d: text not found in %s", i+1, filename)
		case 1:
		default:
			return nil, fmt.Errorf(
				"edit %d: found %d matches in %s — add surrounding context so each oldText is unique",
				i+1, len(occurrences), filename)
		}
		matches = append(matches, batchMatch{
			start: occurrences[0], end: occurrences[0] + len([]byte(old)),
			newText: newText, index: i + 1,
		})
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	for i := 1; i < len(matches); i++ {
		if matches[i].start < matches[i-1].end {
			return nil, fmt.Errorf(
				"edits %d and %d overlap in %s; merge them into one edit or target disjoint regions",
				matches[i-1].index, matches[i].index, filename)
		}
	}

	newContent := applyBatch(normalized, matches)
	final := newContent
	if crlfEnding {
		final = bytes.ReplaceAll(final, []byte("\n"), []byte("\r\n"))
	}
	final = append(bom, final...)

	description, err := DescribeChange(filename, normalized, newContent)
	if err != nil {
		return nil, err
	}
	return &BatchEditResult{
		Content:          final,
		FirstChangedLine: description.FirstChangedLine,
		Diff:             description.Diff,
		Patch:            description.Patch,
	}, nil
}

// firstLineEndingIsCRLF reports whether the first line ending in content is
// CRLF, mirroring the Pi built-in edit's detectLineEnding: the earlier of the
// first CRLF and the first LF decides the restored style, and a file with no
// line ending at all is treated as LF.
func firstLineEndingIsCRLF(content []byte) bool {
	crlf := bytes.Index(content, []byte("\r\n"))
	lf := bytes.IndexByte(content, '\n')
	if crlf < 0 || lf >= 0 && lf < crlf {
		return false
	}
	return true
}

// allIndexes finds every occurrence of needle in haystack, including
// overlapping ones, so an oldText that matches more than one region — for
// example "aa" inside "aaa" — is rejected as ambiguous instead of silently
// replacing only the first region.
func allIndexes(haystack, needle []byte) []int {
	var indexes []int
	pos := 0
	for {
		idx := bytes.Index(haystack[pos:], needle)
		if idx < 0 {
			break
		}
		indexes = append(indexes, pos+idx)
		pos += idx + 1
		if pos >= len(haystack) {
			break
		}
	}
	return indexes
}

func applyBatch(source []byte, matches []batchMatch) []byte {
	var b strings.Builder
	b.Grow(len(source))
	last := 0
	for _, m := range matches {
		b.Write(source[last:m.start])
		b.WriteString(m.newText)
		last = m.end
	}
	b.Write(source[last:])
	return []byte(b.String())
}
