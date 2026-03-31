package srcop

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	beforeDelim     = "===BEFORE==="
	afterDelim      = "===AFTER==="
	maxFileSize     = 100 * 1024 // 100KB
	binaryCheckSize = 8192       // 8KB
)

// Edit applies a text replacement to source using a BEFORE/AFTER block from input.
// input must contain ===BEFORE=== and ===AFTER=== delimiters.
// It does NOT call writeAndShow — that is the caller's responsibility.
func Edit(filename string, source []byte, input []byte) ([]byte, error) {
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
	hasCRLF := bytes.Contains(source, []byte("\r\n"))
	normalized := source
	if hasCRLF {
		normalized = bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	}

	normalizedOld := oldText
	if hasCRLF {
		normalizedOld = bytes.ReplaceAll(oldText, []byte("\r\n"), []byte("\n"))
	}

	start, end, _, err := findMatch(normalized, normalizedOld, filename)
	if err != nil {
		return nil, err
	}

	// If source had CRLF, map matched positions back to original byte offsets.
	origStart, origEnd := start, end
	if hasCRLF {
		origStart = crlfOffset(source, start)
		origEnd = crlfOffset(source, end)
	}

	// Prepare replacement text — match source line endings.
	replacement := newText
	if hasCRLF {
		replacement = bytes.ReplaceAll(newText, []byte("\n"), []byte("\r\n"))
		// Avoid double \r\n if newText already had \r\n.
		replacement = bytes.ReplaceAll(replacement, []byte("\r\r\n"), []byte("\r\n"))
	}

	result := make([]byte, 0, len(source)-(origEnd-origStart)+len(replacement))
	result = append(result, source[:origStart]...)
	result = append(result, replacement...)
	result = append(result, source[origEnd:]...)
	return result, nil
}

// crlfOffset converts a byte offset in normalized (LF-only) source to the corresponding
// offset in the original CRLF source. It walks the original counting LF-equivalent positions:
// each \r\n pair is treated as one line terminator (like \n in normalized form).
// The returned offset accounts for the additional \r bytes.
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
			lines := matchLines(normSource, normOld)
			return 0, 0, "", fmt.Errorf("found %d matches at lines %s — add surrounding context to disambiguate",
				len(lines), formatLineNumbers(lines))
		}

		// For exact pass, positions are already correct.
		if p.name == "exact" {
			return first, first + len(normOld), p.name, nil
		}

		// For normalized passes, map back to original byte range.
		s, e, mapErr := mapNormToOrig(source, old, p.normalize)
		if mapErr != nil {
			// Fallback: use normalized positions. For non-CRLF files, normalized positions
			// are already correct. For CRLF files, Edit will correct them via crlfOffset.
			// mapNormToOrig can fail if the normalization function changes content in ways
			// that break line-by-line matching even though bytes.Index found a match.
			return first, first + len(normOld), p.name, nil
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
// in normalized source. It normalizes both source and old line-by-line, then slides a
// window through the normalized source lines looking for a contiguous run that matches
// the normalized old lines. Once found, the matched line indices are mapped back to byte
// offsets in the original (un-normalized) source via lineOffset.
func mapNormToOrig(source, old []byte, normalize func([]byte) []byte) (start, end int, err error) {
	sourceLines := strings.Split(string(source), "\n")
	oldLines := strings.Split(string(old), "\n")
	normOldLines := strings.Split(string(normalize(old)), "\n")

	// Remove trailing empty element from splitting (trailing newline).
	if len(normOldLines) > 0 && normOldLines[len(normOldLines)-1] == "" {
		normOldLines = normOldLines[:len(normOldLines)-1]
	}
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}

	nOld := len(normOldLines)
	if nOld == 0 {
		return 0, 0, fmt.Errorf("empty old after normalization")
	}

	normSourceLines := strings.Split(string(normalize(source)), "\n")

	// Find the matching window in normalized source.
	matchLine := -1
	for i := 0; i <= len(normSourceLines)-nOld; i++ {
		match := true
		for j := 0; j < nOld; j++ {
			if normSourceLines[i+j] != normOldLines[j] {
				match = false
				break
			}
		}
		if match {
			matchLine = i
			break
		}
	}

	if matchLine < 0 {
		return 0, 0, fmt.Errorf("no line match found")
	}

	// Compute byte offsets in original source for the matched lines.
	lineStart := lineOffset(sourceLines, matchLine)
	lineEnd := lineOffset(sourceLines, matchLine+len(oldLines))

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

// matchLines returns the line numbers (1-based) of all matches of old in source.
func matchLines(source, old []byte) []int {
	var result []int
	pos := 0
	for {
		idx := bytes.Index(source[pos:], old)
		if idx < 0 {
			break
		}
		abs := pos + idx
		lineNum := bytes.Count(source[:abs], []byte("\n")) + 1
		result = append(result, lineNum)
		pos = abs + len(old)
		if pos >= len(source) {
			break
		}
	}
	return result
}

// formatLineNumbers formats a slice of line numbers as a comma-separated string.
func formatLineNumbers(lines []int) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = fmt.Sprintf("%d", l)
	}
	return strings.Join(parts, ", ")
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
	sourceLines := strings.Split(string(source), "\n")
	oldLines := strings.Split(strings.TrimRight(string(old), "\n"), "\n")
	nOld := len(oldLines)
	if nOld == 0 {
		return "(empty search text)"
	}

	// Build normalized set of old lines for comparison.
	oldSet := make(map[string]bool, nOld)
	for _, l := range oldLines {
		oldSet[strings.TrimSpace(l)] = true
	}

	bestScore := -1
	bestStart := 0

	windowSize := nOld
	if windowSize < 1 {
		windowSize = 1
	}

	for i := 0; i <= len(sourceLines)-windowSize; i++ {
		score := 0
		for j := 0; j < windowSize; j++ {
			if oldSet[strings.TrimSpace(sourceLines[i+j])] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestStart = i
		}
	}

	bestEnd := bestStart + windowSize
	if bestEnd > len(sourceLines) {
		bestEnd = len(sourceLines)
	}

	var sb strings.Builder
	for i := bestStart; i < bestEnd; i++ {
		fmt.Fprintf(&sb, "%4d: %s\n", i+1, sourceLines[i])
	}
	return sb.String()
}
