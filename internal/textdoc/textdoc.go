package textdoc

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tta-lab/organon/internal/id"
	"github.com/tta-lab/organon/internal/tree"
)

type node struct {
	tree.Node
	start int
	end   int
}

var (
	texSectionRE = regexp.MustCompile(
		`^\\(part|chapter|section|subsection|subsubsection|paragraph|subparagraph)\*?\{([^}]*)\}`,
	)
	texBeginRE = regexp.MustCompile(`^\\begin\{([^}]*)\}`)
	texEndRE   = regexp.MustCompile(`^\\end\{([^}]*)\}`)
	jsonKeyRE  = regexp.MustCompile(`^\s*"((?:\\.|[^"\\])*)"\s*:`)
)

// Supported reports whether filename can be handled as a text document.
func Supported(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".json", ".jsonc", ".csv", ".tsv", ".tex", ".ltx", ".latex", ".txt", ".text":
		return true
	case "":
		return true
	default:
		return false
	}
}

// Nodes returns editable text-document nodes for filename.
func Nodes(filename string, source []byte) ([]tree.Node, error) {
	nodes, err := parse(filename, source)
	if err != nil {
		return nil, err
	}
	out := make([]tree.Node, len(nodes))
	for i := range nodes {
		out[i] = nodes[i].Node
	}
	return out, nil
}

// Bounds returns the [start, end) byte range for id.
func Bounds(filename string, source []byte, targetID string) (int, int, error) {
	nodes, err := parse(filename, source)
	if err != nil {
		return 0, 0, err
	}
	for _, n := range nodes {
		if n.ID == targetID {
			return n.start, n.end, nil
		}
	}
	return 0, 0, fmt.Errorf("text node %q not found; run --tree to see current IDs", targetID)
}

// Read returns the text covered by targetID.
func Read(filename string, source []byte, targetID string) (string, error) {
	start, end, err := Bounds(filename, source, targetID)
	if err != nil {
		return "", err
	}
	return string(source[start:end]), nil
}

// Replace replaces the text covered by targetID.
func Replace(filename string, source []byte, targetID string, content []byte) ([]byte, error) {
	start, end, err := Bounds(filename, source, targetID)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(source)-(end-start)+len(content))
	result = append(result, source[:start]...)
	result = append(result, content...)
	result = append(result, source[end:]...)
	return result, nil
}

// InsertBefore inserts content before targetID.
func InsertBefore(filename string, source []byte, targetID string, content []byte) ([]byte, error) {
	start, _, err := Bounds(filename, source, targetID)
	if err != nil {
		return nil, err
	}
	return insertAt(source, start, content), nil
}

// InsertAfter inserts content after targetID.
func InsertAfter(filename string, source []byte, targetID string, content []byte) ([]byte, error) {
	_, end, err := Bounds(filename, source, targetID)
	if err != nil {
		return nil, err
	}
	return insertAt(source, end, content), nil
}

// Delete removes the text covered by targetID.
func Delete(filename string, source []byte, targetID string) ([]byte, error) {
	start, end, err := Bounds(filename, source, targetID)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(source)-(end-start))
	result = append(result, source[:start]...)
	result = append(result, source[end:]...)
	return result, nil
}

func insertAt(source []byte, pos int, content []byte) []byte {
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(append([]byte{}, content...), '\n')
	}
	if pos > 0 && source[pos-1] != '\n' && len(content) > 0 && content[0] != '\n' {
		content = append([]byte{'\n'}, content...)
	}
	result := make([]byte, 0, len(source)+len(content))
	result = append(result, source[:pos]...)
	result = append(result, content...)
	result = append(result, source[pos:]...)
	return result
}

func parse(filename string, source []byte) ([]node, error) {
	if !Supported(filename) {
		return nil, fmt.Errorf("unsupported text document type: %s", filename)
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".json", ".jsonc":
		return parseJSON(source), nil
	case ".csv", ".tsv":
		return parseDelimited(source), nil
	case ".tex", ".ltx", ".latex":
		return parseTeX(source), nil
	default:
		return parsePlain(source), nil
	}
}

func parseDelimited(source []byte) []node {
	lines := splitLines(source)
	nodes := make([]node, 0, len(lines))
	for i, line := range lines {
		text := strings.TrimRight(string(source[line.start:line.end]), "\r\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		label := fmt.Sprintf("row %d: %s", i+1, preview(text))
		if i == 0 {
			label = "header: " + preview(text)
		}
		nodes = append(nodes, node{
			Node:  tree.Node{Label: label, Level: 1, Meta: lineMeta(line.line, line.line)},
			start: line.start,
			end:   line.end,
		})
	}
	return assign(nodes)
}

func parseJSON(source []byte) []node {
	lines := splitLines(source)
	nodes := make([]node, 0, len(lines))
	for _, line := range lines {
		text := string(source[line.start:line.end])
		trimmed := strings.TrimSpace(strings.TrimRight(text, "\r\n"))
		if trimmed == "" || trimmed == "{" || trimmed == "}" || trimmed == "[" || trimmed == "]" {
			continue
		}
		label := "item: " + preview(strings.TrimRight(trimmed, ","))
		if match := jsonKeyRE.FindStringSubmatch(text); len(match) == 2 {
			if unquoted, err := strconv.Unquote(`"` + match[1] + `"`); err == nil {
				label = "key " + unquoted
			} else {
				label = "key " + match[1]
			}
		}
		nodes = append(nodes, node{
			Node:  tree.Node{Label: label, Level: 1, Meta: lineMeta(line.line, line.line)},
			start: line.start,
			end:   line.end,
		})
	}
	return assign(nodes)
}

func parseTeX(source []byte) []node {
	lines := splitLines(source)
	var nodes []node
	for i := 0; i < len(lines); i++ {
		text := string(source[lines[i].start:lines[i].contentEnd])
		trimmed := strings.TrimSpace(text)
		if match := texSectionRE.FindStringSubmatch(trimmed); len(match) == 3 {
			level := map[string]int{
				"part": 1, "chapter": 1, "section": 1, "subsection": 2,
				"subsubsection": 3, "paragraph": 4, "subparagraph": 5,
			}[match[1]]
			nodes = append(nodes, node{
				Node:  tree.Node{Label: trimmed, Level: level, Meta: lineMeta(lines[i].line, lines[i].line)},
				start: lines[i].start,
				end:   lines[i].end,
			})
			continue
		}
		if match := texBeginRE.FindStringSubmatch(trimmed); len(match) == 2 {
			env := match[1]
			endLine := i
			for j := i + 1; j < len(lines); j++ {
				endText := strings.TrimSpace(string(source[lines[j].start:lines[j].contentEnd]))
				if endMatch := texEndRE.FindStringSubmatch(endText); len(endMatch) == 2 && endMatch[1] == env {
					endLine = j
					break
				}
			}
			nodes = append(nodes, node{
				Node:  tree.Node{Label: "environment " + env, Level: 2, Meta: lineMeta(lines[i].line, lines[endLine].line)},
				start: lines[i].start,
				end:   lines[endLine].end,
			})
			i = endLine
		}
	}
	if len(nodes) == 0 {
		return parsePlain(source)
	}
	return assign(nodes)
}

func parsePlain(source []byte) []node {
	lines := splitLines(source)
	var nodes []node
	paraStart := -1
	paraEnd := -1
	paraFirstLine := 0
	paraLastLine := 0
	flush := func() {
		if paraStart < 0 {
			return
		}
		first := strings.TrimSpace(string(source[paraStart:firstLineEnd(source, paraStart, paraEnd)]))
		nodes = append(nodes, node{
			Node: tree.Node{
				Label: fmt.Sprintf("paragraph %d: %s", len(nodes)+1, preview(first)),
				Level: 1,
				Meta:  lineMeta(paraFirstLine, paraLastLine),
			},
			start: paraStart,
			end:   paraEnd,
		})
		paraStart = -1
	}
	for _, line := range lines {
		content := source[line.start:line.contentEnd]
		if len(bytes.TrimSpace(content)) == 0 {
			flush()
			continue
		}
		if paraStart < 0 {
			paraStart = line.start
			paraFirstLine = line.line
		}
		paraEnd = line.end
		paraLastLine = line.line
	}
	flush()
	return assign(nodes)
}

type lineRange struct {
	start      int
	contentEnd int
	end        int
	line       int
}

func splitLines(source []byte) []lineRange {
	if len(source) == 0 {
		return nil
	}
	var lines []lineRange
	start := 0
	line := 1
	for i, b := range source {
		if b != '\n' {
			continue
		}
		contentEnd := i
		if contentEnd > start && source[contentEnd-1] == '\r' {
			contentEnd--
		}
		lines = append(lines, lineRange{start: start, contentEnd: contentEnd, end: i + 1, line: line})
		start = i + 1
		line++
	}
	if start < len(source) {
		lines = append(lines, lineRange{start: start, contentEnd: len(source), end: len(source), line: line})
	}
	return lines
}

func lineContentEnd(source []byte, start, end int) int {
	for end > start && (source[end-1] == '\n' || source[end-1] == '\r') {
		end--
	}
	return end
}

func firstLineEnd(source []byte, start, end int) int {
	for i := start; i < end; i++ {
		if source[i] == '\n' || source[i] == '\r' {
			return i
		}
	}
	return lineContentEnd(source, start, end)
}

func assign(nodes []node) []node {
	labels := make([]string, len(nodes))
	for i, n := range nodes {
		labels[i] = fmt.Sprintf("%s\x00%d\x00%d", n.Label, n.start, n.end)
	}
	ids := id.AssignIDs(labels)
	for i := range nodes {
		nodes[i].ID = ids[i]
	}
	return nodes
}

func lineMeta(start, end int) string {
	if start == end {
		return fmt.Sprintf("[L%d]", start)
	}
	return fmt.Sprintf("[L%d-L%d]", start, end)
}

func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 60
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
