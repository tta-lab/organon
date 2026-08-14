package srcop

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/aymanbagabas/go-udiff"
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

	// Preserve BOM and normalize line endings for matching, mirroring the Pi
	// built-in edit: matching always runs on LF, and the final file is restored
	// to the line-ending style of the FIRST line ending in the original file.
	var bom []byte
	content := source
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		bom, content = content[:3], content[3:]
	}
	crlfEnding := firstLineEndingIsCRLF(content)
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))

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
	firstChangedLine := bytes.Count(normalized[:matches[0].start], []byte("\n")) + 1

	final := newContent
	if crlfEnding {
		final = bytes.ReplaceAll(final, []byte("\n"), []byte("\r\n"))
	}
	final = append(bom, final...)

	oldText, newText := string(normalized), string(newContent)
	editsForDiff := udiff.Lines(oldText, newText)
	unified, err := udiff.ToUnifiedDiff("a/"+filename, "b/"+filename, oldText, editsForDiff, 4)
	if err != nil {
		return nil, fmt.Errorf("generate edit diff: %w", err)
	}
	return &BatchEditResult{
		Content:          final,
		FirstChangedLine: firstChangedLine,
		Diff:             formatDisplayDiff(oldText, newText),
		Patch:            unified.String(),
	}, nil
}

// formatDisplayDiff mirrors Pi's renderer-oriented edit diff: changed and
// context lines carry their old/new line number, while file and hunk headers
// remain exclusive to the machine-consumable unified patch.
func formatDisplayDiff(oldText, newText string) string {
	const contextLines = 4
	parts := displayDiffParts(oldText, newText)
	lineNumberWidth := len(fmt.Sprint(max(strings.Count(oldText, "\n")+1, strings.Count(newText, "\n")+1)))
	lines := make([]string, 0)
	oldLine, newLine := 1, 1
	lastWasChange := false

	for i, part := range parts {
		raw := displayDiffLines(part.Content)
		if part.Added || part.Removed {
			lines, oldLine, newLine = appendDisplayChange(lines, part, raw, lineNumberWidth, oldLine, newLine)
			lastWasChange = true
			continue
		}
		nextPartIsChange := i < len(parts)-1 && (parts[i+1].Added || parts[i+1].Removed)
		lines, oldLine, newLine = appendDisplayContext(
			lines, raw, lineNumberWidth, oldLine, newLine, lastWasChange, nextPartIsChange, contextLines,
		)
		lastWasChange = false
	}
	return strings.Join(lines, "\n")
}

func appendDisplayChange(
	lines []string,
	part displayDiffPart,
	raw []string,
	lineNumberWidth, oldLine, newLine int,
) ([]string, int, int) {
	for _, line := range raw {
		if part.Added {
			lines = append(lines, fmt.Sprintf("+%*d %s", lineNumberWidth, newLine, line))
			newLine++
		} else {
			lines = append(lines, fmt.Sprintf("-%*d %s", lineNumberWidth, oldLine, line))
			oldLine++
		}
	}
	return lines, oldLine, newLine
}

func appendDisplayContext(
	lines, raw []string,
	lineNumberWidth, oldLine, newLine int,
	afterChange, beforeChange bool,
	contextLines int,
) ([]string, int, int) {
	if afterChange && beforeChange {
		return appendDisplayMiddleContext(lines, raw, lineNumberWidth, oldLine, newLine, contextLines)
	}
	if afterChange {
		return appendDisplayTrailingContext(lines, raw, lineNumberWidth, oldLine, newLine, contextLines)
	}
	if beforeChange {
		return appendDisplayLeadingContext(lines, raw, lineNumberWidth, oldLine, newLine, contextLines)
	}
	return lines, oldLine + len(raw), newLine + len(raw)
}

func appendDisplayMiddleContext(
	lines, raw []string,
	lineNumberWidth, oldLine, newLine, contextLines int,
) ([]string, int, int) {
	if len(raw) <= contextLines*2 {
		return appendDisplayEqualLines(lines, raw, lineNumberWidth, oldLine, newLine)
	}
	lines, oldLine, newLine = appendDisplayEqualLines(
		lines, raw[:contextLines], lineNumberWidth, oldLine, newLine,
	)
	skippedLines := len(raw) - contextLines*2
	lines = append(lines, fmt.Sprintf(" %*s ...", lineNumberWidth, ""))
	oldLine += skippedLines
	newLine += skippedLines
	return appendDisplayEqualLines(lines, raw[len(raw)-contextLines:], lineNumberWidth, oldLine, newLine)
}

func appendDisplayTrailingContext(
	lines, raw []string,
	lineNumberWidth, oldLine, newLine, contextLines int,
) ([]string, int, int) {
	shownLines := min(len(raw), contextLines)
	lines, oldLine, newLine = appendDisplayEqualLines(
		lines, raw[:shownLines], lineNumberWidth, oldLine, newLine,
	)
	if skippedLines := len(raw) - shownLines; skippedLines > 0 {
		lines = append(lines, fmt.Sprintf(" %*s ...", lineNumberWidth, ""))
		oldLine += skippedLines
		newLine += skippedLines
	}
	return lines, oldLine, newLine
}

func appendDisplayLeadingContext(
	lines, raw []string,
	lineNumberWidth, oldLine, newLine, contextLines int,
) ([]string, int, int) {
	skippedLines := max(0, len(raw)-contextLines)
	if skippedLines > 0 {
		lines = append(lines, fmt.Sprintf(" %*s ...", lineNumberWidth, ""))
		oldLine += skippedLines
		newLine += skippedLines
	}
	return appendDisplayEqualLines(lines, raw[skippedLines:], lineNumberWidth, oldLine, newLine)
}

func appendDisplayEqualLines(
	lines, raw []string,
	lineNumberWidth, oldLine, newLine int,
) ([]string, int, int) {
	for _, line := range raw {
		lines = append(lines, fmt.Sprintf(" %*d %s", lineNumberWidth, oldLine, line))
		oldLine++
		newLine++
	}
	return lines, oldLine, newLine
}

type displayDiffPart struct {
	Content string
	Added   bool
	Removed bool
}

// displayDiffParts ports jsdiff's Myers traversal, which Pi uses through
// Diff.diffLines. A generic LCS can select different equal-line alignments,
// changing the numbered display diff for repeated lines.
func displayDiffParts(oldText, newText string) []displayDiffPart {
	oldTokens, newTokens := displayDiffTokens(oldText), displayDiffTokens(newText)
	lastComponent := displayDiffComponents(oldTokens, newTokens)
	return displayDiffValues(lastComponent, newTokens, oldTokens)
}

func displayDiffComponents(oldTokens, newTokens []string) *displayDiffComponent {
	oldLen, newLen := len(oldTokens), len(newTokens)
	bestPath := map[int]*displayDiffPath{0: {oldPos: -1}}
	newPos := displayDiffExtractCommon(bestPath[0], newTokens, oldTokens, 0)
	if bestPath[0].oldPos+1 >= oldLen && newPos+1 >= newLen {
		return bestPath[0].lastComponent
	}

	minDiagonal, maxDiagonal := -oldLen-newLen, oldLen+newLen
	for editLength := 1; editLength <= oldLen+newLen; editLength++ {
		for diagonal := max(minDiagonal, -editLength); diagonal <= min(maxDiagonal, editLength); diagonal += 2 {
			path, nextNewPos, done := displayDiffAdvance(
				bestPath, diagonal, oldTokens, newTokens,
			)
			if path == nil {
				continue
			}
			if done {
				return path.lastComponent
			}
			bestPath[diagonal] = path
			minDiagonal, maxDiagonal = displayDiffBounds(
				minDiagonal, maxDiagonal, diagonal, path, nextNewPos, oldLen, newLen,
			)
		}
	}
	return nil
}

func displayDiffAdvance(
	bestPath map[int]*displayDiffPath,
	diagonal int,
	oldTokens, newTokens []string,
) (*displayDiffPath, int, bool) {
	removePath, addPath := bestPath[diagonal-1], bestPath[diagonal+1]
	delete(bestPath, diagonal-1)
	canAdd := displayDiffCanAdd(addPath, diagonal, len(newTokens))
	canRemove := removePath != nil && removePath.oldPos+1 < len(oldTokens)
	if !canAdd && !canRemove {
		delete(bestPath, diagonal)
		return nil, 0, false
	}
	path := displayDiffSelectPath(removePath, addPath, canRemove, canAdd)
	newPos := displayDiffExtractCommon(path, newTokens, oldTokens, diagonal)
	done := path.oldPos+1 >= len(oldTokens) && newPos+1 >= len(newTokens)
	return path, newPos, done
}

func displayDiffCanAdd(path *displayDiffPath, diagonal, newLen int) bool {
	if path == nil {
		return false
	}
	newPos := path.oldPos - diagonal
	return newPos >= 0 && newPos < newLen
}

func displayDiffSelectPath(
	removePath, addPath *displayDiffPath,
	canRemove, canAdd bool,
) *displayDiffPath {
	if !canRemove || (canAdd && removePath.oldPos < addPath.oldPos) {
		return displayDiffAddToPath(addPath, true, false, 0)
	}
	return displayDiffAddToPath(removePath, false, true, 1)
}

func displayDiffBounds(
	minDiagonal, maxDiagonal, diagonal int,
	path *displayDiffPath,
	newPos, oldLen, newLen int,
) (int, int) {
	if path.oldPos+1 >= oldLen {
		maxDiagonal = min(maxDiagonal, diagonal-1)
	}
	if newPos+1 >= newLen {
		minDiagonal = max(minDiagonal, diagonal+1)
	}
	return minDiagonal, maxDiagonal
}

type displayDiffPath struct {
	oldPos        int
	lastComponent *displayDiffComponent
}

type displayDiffComponent struct {
	count             int
	added, removed    bool
	previousComponent *displayDiffComponent
}

func displayDiffTokens(content string) []string {
	tokens := strings.SplitAfter(content, "\n")
	if tokens[len(tokens)-1] == "" {
		return tokens[:len(tokens)-1]
	}
	return tokens
}

func displayDiffAddToPath(
	path *displayDiffPath,
	added, removed bool,
	oldPosIncrement int,
) *displayDiffPath {
	last := path.lastComponent
	if last != nil && last.added == added && last.removed == removed {
		return &displayDiffPath{
			oldPos: path.oldPos + oldPosIncrement,
			lastComponent: &displayDiffComponent{
				count: last.count + 1, added: added, removed: removed, previousComponent: last.previousComponent,
			},
		}
	}
	return &displayDiffPath{
		oldPos: path.oldPos + oldPosIncrement,
		lastComponent: &displayDiffComponent{
			count: 1, added: added, removed: removed, previousComponent: last,
		},
	}
}

func displayDiffExtractCommon(
	path *displayDiffPath,
	newTokens, oldTokens []string,
	diagonal int,
) int {
	oldPos := path.oldPos
	newPos := oldPos - diagonal
	commonCount := 0
	for newPos+1 < len(newTokens) && oldPos+1 < len(oldTokens) && oldTokens[oldPos+1] == newTokens[newPos+1] {
		newPos++
		oldPos++
		commonCount++
	}
	if commonCount > 0 {
		path.lastComponent = &displayDiffComponent{
			count: commonCount, previousComponent: path.lastComponent,
		}
	}
	path.oldPos = oldPos
	return newPos
}

func displayDiffValues(
	lastComponent *displayDiffComponent,
	newTokens, oldTokens []string,
) []displayDiffPart {
	components := make([]*displayDiffComponent, 0)
	for component := lastComponent; component != nil; component = component.previousComponent {
		components = append(components, component)
	}
	parts := make([]displayDiffPart, 0, len(components))
	newPos, oldPos := 0, 0
	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]
		if component.removed {
			parts = append(parts, displayDiffPart{
				Content: strings.Join(oldTokens[oldPos:oldPos+component.count], ""), Removed: true,
			})
			oldPos += component.count
			continue
		}
		parts = append(parts, displayDiffPart{
			Content: strings.Join(newTokens[newPos:newPos+component.count], ""), Added: component.added,
		})
		newPos += component.count
		if !component.added {
			oldPos += component.count
		}
	}
	return parts
}

func displayDiffLines(content string) []string {
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	return lines
}

// normalizeLineEndings converts CRLF and lone CR line endings to LF so edit
// matching and replacement have one canonical representation.
func normalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
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
