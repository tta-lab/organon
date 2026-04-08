package srcop

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/indent"
)

const (
	beforeDelim     = "===BEFORE==="
	afterDelim      = "===AFTER==="
	maxFileSize     = 100 * 1024 // 100KB
	binaryCheckSize = 8192       // 8KB
)

// EditResult holds the result of an edit operation including match metadata.
type EditResult struct {
	Content    []byte       // the new file bytes
	Pass       string       // "exact"|"trim-trailing"|"trim-both"|"unicode-fold"
	IndentFrom indent.Style // zero value if reindent not attempted
	IndentTo   indent.Style
	Reindented bool
	Warnings   []string // file-level + reindent line-level warnings
}

// Edit applies a text replacement to source using a BEFORE/AFTER block from input.
// input must contain ===BEFORE=== and ===AFTER=== delimiters.
// It does NOT call writeAndShow — that is the caller's responsibility.
func Edit(filename string, source []byte, input []byte) (*EditResult, error) {
	if isBinary(source) {
		return nil, fmt.Errorf("binary file detected; src edit only works on text files")
	}
	if len(source) > maxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", len(source), maxFileSize)
	}

	oldText, newText, err := parseEditInput(input)
	if err != nil {
		return nil, err
	}

	// Detect and normalize CRLF for matching.
	normalized, normalizedOld, hasCRLF := normalizeForMatch(source, oldText)

	start, end, pass, err := findMatch(normalized, normalizedOld, filename)
	if err != nil {
		return nil, err
	}

	// If source had CRLF, map matched positions back to original byte offsets.
	origStart, origEnd := crlfAdjust(source, start, end)

	// Apply reindent for trim-both pass when target style is known.
	// This runs BEFORE CRLF adjustment so that indent.Reindent operates on LF content.
	// Note: reindent is intentionally scoped to trim-both only — exact, trim-trailing,
	// and unicode-fold passes don't involve indent normalization.
	var warnings []string
	var indentFrom, indentTo indent.Style
	var reindented bool
	replacement := newText

	switch pass {
	case "trim-both":
		target := indent.Detect(filename, source)
		if target.Kind != indent.Unknown {
			var reindentWarnings []string
			replacement, reindented, indentFrom, indentTo, reindentWarnings =
				applyReindent(newText, normalized[origStart:origEnd], target)
			warnings = append(warnings, reindentWarnings...)
		}
	case "exact":
		// Reindent is not applied for exact pass; warn if AFTER style mismatches target.
		if target := indent.Detect(filename, source); target.Kind != indent.Unknown {
			if after := indent.DetectByContent(newText); after.Kind != indent.Unknown && after.Kind != target.Kind {
				warnings = append(warnings, fmt.Sprintf("AFTER uses %s indent but file uses %s; mismatch will be inserted verbatim",
					kindLabel(after.Kind), kindLabel(target.Kind)))
			}
		}
	}

	// Prepare replacement text — match source line endings.
	if hasCRLF {
		replacement = bytes.ReplaceAll(replacement, []byte("\n"), []byte("\r\n"))
		replacement = bytes.ReplaceAll(replacement, []byte("\r\r\n"), []byte("\r\n"))
	}

	result := make([]byte, 0, len(source)-(origEnd-origStart)+len(replacement))
	result = append(result, source[:origStart]...)
	result = append(result, replacement...)
	result = append(result, source[origEnd:]...)
	return &EditResult{
		Content:    result,
		Pass:       pass,
		IndentFrom: indentFrom,
		IndentTo:   indentTo,
		Reindented: reindented,
		Warnings:   warnings,
	}, nil
}

// normalizeForMatch normalizes source and oldText to LF line endings for matching,
// returning the normalized forms and whether the source had CRLF.
func normalizeForMatch(source, oldText []byte) (normalized, normalizedOld []byte, hasCRLF bool) {
	hasCRLF = bytes.Contains(source, []byte("\r\n"))
	if hasCRLF {
		return bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n")),
			bytes.ReplaceAll(oldText, []byte("\r\n"), []byte("\n")),
			true
	}
	return source, oldText, false
}

// crlfAdjust maps byte offsets from LF-normalized source back to the original
// (potentially CRLF) source if needed.
func crlfAdjust(source []byte, start, end int) (origStart, origEnd int) {
	if !bytes.Contains(source, []byte("\r\n")) {
		return start, end
	}
	return crlfOffset(source, start), crlfOffset(source, end)
}

// crlfOffset converts a byte offset in normalized (LF-only) source to the corresponding
// offset in the original CRLF source.
func crlfOffset(original []byte, pos int) int {
	offset := 0
	lf := 0
	for i := 0; i < len(original) && lf < pos; i++ {
		if original[i] == '\r' && i+1 < len(original) && original[i+1] == '\n' {
			offset++
			// i++ moves from \r position to \n position; the loop's automatic i++
			// then moves past \n entirely. The \r\n pair counts as one line terminator.
			i++
			lf++
		} else {
			lf++
		}
	}
	return pos + offset
}

// parseEditInput parses the BEFORE/AFTER block from input bytes.
func parseEditInput(input []byte) (old, new []byte, err error) {
	text := string(input)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	beforeIdx := -1
	afterIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == beforeDelim && beforeIdx == -1 {
			beforeIdx = i
		} else if trimmed == afterDelim && afterIdx == -1 && beforeIdx >= 0 {
			afterIdx = i
		}
	}

	if beforeIdx < 0 {
		return nil, nil, fmt.Errorf("missing %s delimiter", beforeDelim)
	}
	if afterIdx < 0 {
		return nil, nil, fmt.Errorf("missing %s delimiter", afterDelim)
	}

	oldLines := lines[beforeIdx+1 : afterIdx]
	newLines := lines[afterIdx+1:]

	oldLines = trimBlankBorderLines(oldLines)
	newLines = trimBlankBorderLines(newLines)

	if len(oldLines) == 0 {
		return nil, nil, fmt.Errorf("BEFORE section is empty")
	}

	oldText := strings.Join(oldLines, "\n") + "\n"
	newText := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		newText += "\n"
	}

	if oldText == newText {
		return nil, nil, fmt.Errorf("old and new text are identical (no-op)")
	}

	return []byte(oldText), []byte(newText), nil
}

// trimBlankBorderLines removes leading and trailing blank lines from a slice.
func trimBlankBorderLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// findMatch runs a 4-pass search for old in source:
//  1. exact: exact byte match
//  2. trim-trailing: match after trimming trailing whitespace from each line
//  3. trim-both: match after trimming all leading/trailing whitespace per line
//  4. unicode-fold: match after folding unicode punctuation to ASCII equivalents
//
// If a pass finds exactly one match, it returns immediately (mapping byte positions
// back to the original source if normalization was applied). If a pass finds multiple
// matches, it errors with line numbers — it does NOT fall through to the next pass.
// If all passes find zero matches, it returns an error with closestRegion output.
//
// Returns byte offsets [start, end) of the match in the original source.
func findMatch(source, old []byte, filename string) (start, end int, pass string, err error) {
	passes := []struct {
		name      string
		normalize func([]byte) []byte
	}{
		{"exact", func(b []byte) []byte { return b }},
		{"trim-trailing", normalizeTrailingWS},
		{"trim-both", normalizeBothWS},
		{"unicode-fold", normalizeUnicode},
	}

	for _, p := range passes {
		normSource := p.normalize(source)
		normOld := p.normalize(old)

		first := bytes.Index(normSource, normOld)
		if first < 0 {
			continue
		}

		// Check for duplicates.
		last := bytes.LastIndex(normSource, normOld)
		if first != last {
			sites := matchSites(normSource, source, normOld)
			var sb strings.Builder
			fmt.Fprintf(&sb, "found %d matches:\n", len(sites))
			for _, site := range sites {
				fmt.Fprintf(&sb, "  line %d: %s\n", site.lineNum, site.snippet)
			}
			sb.WriteString("\nadd surrounding context to disambiguate")
			return 0, 0, "", errors.New(sb.String())
		}

		// For exact pass, positions are already correct.
		if p.name == "exact" {
			return first, first + len(normOld), p.name, nil
		}

		// For normalized passes, map back to original byte range using pre-computed
		// normSource/normOld to avoid re-normalizing inside mapNormToOrig.
		s, e, mapErr := mapNormToOrig(source, normSource, normOld)
		if mapErr != nil {
			return 0, 0, "", fmt.Errorf("internal: %s pass matched but remapping failed: %w", p.name, mapErr)
		}
		return s, e, p.name, nil
	}

	// No match found — show closest region.
	region := closestRegion(source, old)
	return 0, 0, "", fmt.Errorf("text not found in %s\n\nClosest region:\n%s", filename, region)
}

// normalizeTrailingWS trims trailing whitespace from each line.
func normalizeTrailingWS(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return []byte(strings.Join(lines, "\n"))
}

// normalizeBothWS trims all whitespace from each line.
func normalizeBothWS(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return []byte(strings.Join(lines, "\n"))
}

// normalizeUnicode applies unicode folding: fancy punctuation → ASCII equivalents.
func normalizeUnicode(b []byte) []byte {
	return []byte(foldUnicode(string(b)))
}

// foldUnicode maps unicode punctuation variants to their ASCII equivalents.
func foldUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		// Curly/typographic quotes → straight quotes
		case '\u2018', '\u2019', '\u201A', '\u201B': // ' ' ‚ ‛
			return '\''
		case '\u201C', '\u201D', '\u201E', '\u201F': // " " „ ‟
			return '"'
		// Dashes → hyphen
		case '\u2013', '\u2014', '\u2015', '\u2212': // – — ― −
			return '-'
		// Spaces → ASCII space
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005',
			'\u2006', '\u2007', '\u2008', '\u2009', '\u200A',
			'\u202F', '\u205F', '\u3000':
			return ' '
		// Ellipsis → three dots
		case '\u2026': // …
			return '.'
		}
		return r
	}, s)
}

// mapNormToOrig finds the byte range in original source corresponding to a match found
// in normalized source. It accepts pre-computed normSource and normOld (already normalized
// by the caller) to avoid double-normalization. It slides a window through normSource lines
// looking for a contiguous run matching normOld lines, then maps the matched line indices
// back to byte offsets in the original (un-normalized) source via lineOffset.
func mapNormToOrig(source, normSource, normOld []byte) (start, end int, err error) {
	sourceLines := strings.Split(string(source), "\n")
	normSourceLines := strings.Split(string(normSource), "\n")

	normOldLines := strings.Split(string(normOld), "\n")
	// Remove trailing empty element from splitting a trailing newline.
	if len(normOldLines) > 0 && normOldLines[len(normOldLines)-1] == "" {
		normOldLines = normOldLines[:len(normOldLines)-1]
	}

	nOld := len(normOldLines)
	if nOld == 0 {
		return 0, 0, fmt.Errorf(
			"internal: line-level remapping failed (old_lines=0, source_lines=%d)",
			len(sourceLines),
		)
	}

	// Slide a window through normalized source lines to find the matching run.
	matchLine := -1
outer:
	for i := 0; i <= len(normSourceLines)-nOld; i++ {
		for j := 0; j < nOld; j++ {
			if normSourceLines[i+j] != normOldLines[j] {
				continue outer
			}
		}
		matchLine = i
		break
	}

	if matchLine < 0 {
		return 0, 0, fmt.Errorf(
			"internal: line-level remapping failed (old_lines=%d, source_lines=%d)",
			nOld, len(sourceLines),
		)
	}

	// Compute byte offsets in original source for the matched line range.
	// Use nOld (matched normalized line count) for the end offset — this is the
	// number of lines we matched, and normOldLines and oldLines have the same count
	// after trimBlankBorderLines strips trailing blanks in parseEditInput.
	lineStart := lineOffset(sourceLines, matchLine)
	lineEnd := lineOffset(sourceLines, matchLine+nOld)

	return lineStart, lineEnd, nil
}

// lineOffset computes the byte offset of line index lineIdx in a slice of lines.
func lineOffset(lines []string, lineIdx int) int {
	offset := 0
	for i := 0; i < lineIdx && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // +1 for \n
	}
	return offset
}

// matchSite describes one occurrence of old in source.
type matchSite struct {
	lineNum int    // 1-based line number
	snippet string // first line of the match, trimmed to 60 chars
}

// matchSites returns all match sites for old in source.
// source is the normalized source (where bytes.Index was run) used for line number
// computation. origSource is the original file bytes used for snippet extraction.
func matchSites(normSource, origSource, old []byte) []matchSite {
	var sites []matchSite
	pos := 0
	for {
		idx := bytes.Index(normSource[pos:], old)
		if idx < 0 {
			break
		}
		abs := pos + idx
		lineNum := bytes.Count(normSource[:abs], []byte("\n")) + 1
		// Extract snippet from original source (as the user wrote it).
		origLines := strings.Split(string(origSource), "\n")
		snippet := ""
		if lineNum <= len(origLines) {
			snippet = origLines[lineNum-1]
			if len(snippet) > 60 {
				snippet = snippet[:60] + "…"
			}
		}
		sites = append(sites, matchSite{lineNum: lineNum, snippet: snippet})
		pos = abs + len(old)
		if pos >= len(normSource) {
			break
		}
	}
	return sites
}

// isBinary checks the first 8KB for null bytes.
func isBinary(data []byte) bool {
	check := data
	if len(check) > binaryCheckSize {
		check = check[:binaryCheckSize]
	}
	return bytes.IndexByte(check, 0) >= 0
}

// closestRegion finds the window of source lines most similar to old for error reporting.
// It builds a set of trimmed old lines, then slides a window of the same size through source,
// scoring each window by how many of its lines appear in the old set. The highest-scoring
// window is returned with 1-based line numbers and content so agents can self-correct.
func closestRegion(source, old []byte) string {
	if len(source) == 0 {
		return "(source file is empty)"
	}

	sourceLines := strings.Split(string(source), "\n")
	oldLines := strings.Split(strings.TrimRight(string(old), "\n"), "\n")
	nOld := len(oldLines)
	if nOld == 0 {
		return "(empty search text)"
	}

	if len(sourceLines) < nOld {
		return fmt.Sprintf("(source has %d lines, search text has %d — no region to show)", len(sourceLines), nOld)
	}

	// Build normalized set of old lines for comparison.
	oldSet := make(map[string]bool, nOld)
	for _, l := range oldLines {
		oldSet[strings.TrimSpace(l)] = true
	}

	bestScore := 0
	bestStart := -1

	for i := 0; i <= len(sourceLines)-nOld; i++ {
		score := 0
		for j := 0; j < nOld; j++ {
			if oldSet[strings.TrimSpace(sourceLines[i+j])] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestStart = i
		}
	}

	if bestStart < 0 {
		return "(no similar region found — BEFORE shares no lines with file)"
	}

	bestEnd := bestStart + nOld
	if bestEnd > len(sourceLines) {
		bestEnd = len(sourceLines)
	}

	var sb strings.Builder
	for i := bestStart; i < bestEnd; i++ {
		fmt.Fprintf(&sb, "%4d: %s\n", i+1, sourceLines[i])
	}
	return sb.String()
}

// kindLabel returns a human-readable label for a Kind value.
func kindLabel(k indent.Kind) string {
	switch k {
	case indent.Tab:
		return "tab"
	case indent.Space:
		return "space"
	default:
		return "unknown"
	}
}

// indentDepthFromLines computes the indent level from the leading whitespace of
// the first non-blank line in matchedLines (which is the normalized matched region).
// Returns 0 if no non-blank line is found.
func indentDepthFromLines(matchedLines []byte, target indent.Style) int {
	for _, line := range strings.Split(string(matchedLines), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return indentLevel([]byte(line), target)
	}
	return 0
}

// applyReindent handles the trim-both reindent logic: first tries indent.Reindent,
// and if AFTER has no indent (from.Kind==Unknown) but target is known, infers
// indent depth from the matched region and applies it.
// Returns (replacement, reindented, indentFrom, indentTo, warnings).
func applyReindent(newText []byte, matchedRegion []byte, target indent.Style) (
	replacement []byte, reindented bool, indentFrom, indentTo indent.Style, warnings []string,
) {
	reindentBytes, from, ok, reindentWarnings := indent.Reindent(newText, target)
	warnings = append(warnings, reindentWarnings...)
	replacement = reindentBytes
	indentFrom = from
	indentTo = target

	if ok && from.Kind != indent.Unknown && from != target {
		// Normal reindent: AFTER had indent and it was transformed.
		reindented = true
		return
	}
	if !ok {
		warnings = append(warnings, "could not detect AFTER indent style; inserted as-is")
		return
	}
	// from.Kind == Unknown: AFTER has no indent. Infer depth from matched region.
	if depth := indentDepthFromLines(matchedRegion, target); depth > 0 {
		replacement = applyIndentDepth(string(newText), depth, target)
		reindented = true
		indentFrom = indent.Style{Kind: indent.Unknown}
	}
	return
}

// indentLevel returns the indent level of a line given the target style.
// A line with no leading whitespace returns 0.
func indentLevel(line []byte, target indent.Style) int {
	tabs := 0
	spaces := 0
	for _, b := range line {
		switch b {
		case '\t':
			tabs++
		case ' ':
			spaces++
		default:
			goto done
		}
	}
done:
	if tabs > 0 {
		return tabs
	}
	if spaces > 0 && target.Kind == indent.Space && target.Width > 0 {
		return spaces / target.Width
	}
	return 0
}

// applyIndentDepth prepends target-style indentation to each non-empty line in text,
// using the computed indent level (number of levels, not raw width).
func applyIndentDepth(text string, level int, target indent.Style) []byte {
	prefix := indentPrefix(level, target)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = prefix + line
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// indentPrefix returns a string of level indentation units in the target style.
func indentPrefix(level int, target indent.Style) string {
	if target.Kind == indent.Tab {
		return strings.Repeat("\t", level)
	}
	width := target.Width
	if width <= 0 {
		width = 2
	}
	return strings.Repeat(" ", level*width)
}
