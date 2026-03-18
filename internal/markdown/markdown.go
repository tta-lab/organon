package markdown

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"

	"github.com/tta-lab/organon/internal/fetch"
	"github.com/tta-lab/organon/internal/id"
	"github.com/tta-lab/organon/internal/tree"
)

// mdHeading holds metadata for one parsed markdown heading.
type mdHeading struct {
	level  int
	text   string
	offset int    // byte offset of the heading line in source
	id     string // 2-char base62 ID (empty for H1)
}

// DefaultTreeThreshold is the character count above which content returns a heading tree by default.
const DefaultTreeThreshold = 5000

// parseHeadings parses markdown source and returns all headings with levels and byte offsets.
func parseHeadings(source []byte) ([]mdHeading, error) { //nolint:gocyclo
	md := goldmark.New()
	reader := goldmarktext.NewReader(source)
	doc := md.Parser().Parse(reader)

	var headings []mdHeading
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		var textBuf strings.Builder
		for c := h.FirstChild(); c != nil; c = c.NextSibling() {
			if seg, ok := c.(*ast.Text); ok {
				textBuf.Write(seg.Segment.Value(source))
			} else if code, ok := c.(*ast.CodeSpan); ok {
				for cc := code.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if seg, ok := cc.(*ast.Text); ok {
						textBuf.Write(seg.Segment.Value(source))
					}
				}
			}
		}

		offset := 0
		if h.Lines() != nil && h.Lines().Len() > 0 {
			offset = h.Lines().At(0).Start
		} else if seg := n.Lines(); seg != nil && seg.Len() > 0 {
			offset = seg.At(0).Start
		}

		headings = append(headings, mdHeading{
			level:  h.Level,
			text:   textBuf.String(),
			offset: offset,
		})
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, fmt.Errorf("parseHeadings: ast.Walk: %w", err)
	}

	// Fix offsets: walk back to the '#' characters at start of line
	for i := range headings {
		off := headings[i].offset
		for off > 0 && source[off-1] != '\n' {
			off--
		}
		headings[i].offset = off
	}

	return headings, nil
}

// assignIDs generates stable 2-char base62 IDs for each heading.
// H1 headings get no ID (they can't be targeted with --section).
// On collision, extends to 3 chars with positional disambiguator.
func assignIDs(headings []mdHeading) {
	// Collect labels for non-H1 headings
	type entry struct {
		idx   int
		label string
	}
	var entries []entry
	for i, h := range headings {
		if h.level == 1 {
			continue
		}
		label := fmt.Sprintf("%s %s", strings.Repeat("#", h.level), h.text)
		entries = append(entries, entry{idx: i, label: label})
	}

	if len(entries) == 0 {
		return
	}

	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = e.label
	}

	ids := id.AssignIDs(labels)
	for i, e := range entries {
		headings[e.idx].id = ids[i]
	}
}

// extractSection returns the byte slice from the target heading to the next
// heading at the same or higher level (lower number), or end of document.
func extractSection(source []byte, headings []mdHeading, sectionID string) (string, error) {
	targetIdx := -1
	for i, h := range headings {
		if h.id == sectionID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		var ids []string
		for _, h := range headings {
			if h.id != "" {
				ids = append(ids, fmt.Sprintf("%q (%s)", h.id, h.text))
			}
		}
		return "", fmt.Errorf("section %q not found; available: %s", sectionID, strings.Join(ids, ", "))
	}

	target := headings[targetIdx]
	start := target.offset

	end := len(source)
	for _, h := range headings[targetIdx+1:] {
		if h.level <= target.level {
			end = h.offset
			break
		}
	}

	return strings.TrimRight(string(source[start:end]), "\n") + "\n", nil
}

// sectionCharCount returns the character count of a section's content.
func sectionCharCount(source []byte, headings []mdHeading, idx int) int {
	start := headings[idx].offset
	end := len(source)
	for _, h := range headings[idx+1:] {
		if h.level <= headings[idx].level {
			end = h.offset
			break
		}
	}
	return utf8.RuneCount(source[start:end])
}

// renderTree builds an indented tree of headings with IDs and char counts.
func renderTree(headings []mdHeading, source []byte) string {
	if len(headings) == 0 {
		return "(no headings)\n"
	}

	var nodes []tree.Node

	// Print H1 title as header line if present
	var header string
	if headings[0].level == 1 {
		charCount := sectionCharCount(source, headings, 0)
		header = fmt.Sprintf("# %s\n\nTotal: %s characters\n\n", headings[0].text, formatNum(charCount))
	}

	minLevel := 99
	for _, h := range headings {
		if h.level > 1 && h.level < minLevel {
			minLevel = h.level
		}
	}
	if minLevel == 99 {
		minLevel = 2
	}

	for i, h := range headings {
		if h.level == 1 {
			continue
		}
		charCount := sectionCharCount(source, headings, i)
		meta := fmt.Sprintf("(%s chars)", formatNum(charCount))
		label := fmt.Sprintf("%s %s", strings.Repeat("#", h.level), h.text)
		nodes = append(nodes, tree.Node{
			ID:    h.id,
			Label: label,
			Level: h.level - minLevel + 1,
			Meta:  meta,
		})
	}

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString(tree.Render(nodes))
	sb.WriteString("\nUse -s <id> to read a section, or --full to read everything.\n")
	return sb.String()
}

// MarkdownResult holds the output of a markdown render operation.
type MarkdownResult struct {
	Content string // rendered content (full text, tree, or section)
	Mode    string // "full", "tree", or "section"
}

// RenderContent renders markdown source bytes with tree/section/full modes.
// treeThreshold: auto-switch to tree above this char count (default 5000 if ≤ 0).
func RenderContent(
	source []byte, showTree bool, section string, full bool, treeThreshold int,
) (*MarkdownResult, error) {
	if treeThreshold <= 0 {
		treeThreshold = DefaultTreeThreshold
	}

	headings, err := parseHeadings(source)
	if err != nil {
		return nil, err
	}
	assignIDs(headings)

	if section != "" {
		content, err := extractSection(source, headings, section)
		if err != nil {
			return nil, err
		}
		return &MarkdownResult{Content: content, Mode: "section"}, nil
	}

	charCount := utf8.RuneCountInString(string(source))

	if showTree || (!full && charCount > treeThreshold) {
		if len(headings) == 0 {
			return &MarkdownResult{Content: fetch.TruncateContent(string(source)), Mode: "full"}, nil
		}
		return &MarkdownResult{Content: renderTree(headings, source), Mode: "tree"}, nil
	}

	return &MarkdownResult{Content: fetch.TruncateContent(string(source)), Mode: "full"}, nil
}

// formatNum formats an integer with thousands separators.
func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
