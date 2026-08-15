package srcop

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/aymanbagabas/go-udiff"
)

// ChangeDescription is the Pi-compatible description of a file mutation.
// Diff is the compact display form, Patch is a standard unified patch, and
// FirstChangedLine is the 1-indexed line of the first change in the new file.
type ChangeDescription struct {
	Diff             string
	Patch            string
	FirstChangedLine int
}

// DescribeChange produces the one change description shared by exact batches
// and symbol mutations. BOM and line endings are omitted from the comparison,
// matching Pi's edit diff contract rather than treating a preserved file
// encoding as a content change.
func DescribeChange(filename string, old, new []byte) (ChangeDescription, error) {
	oldText := normalizeChangeContent(old)
	newText := normalizeChangeContent(new)
	edits := udiff.Lines(oldText, newText)
	unified, err := udiff.ToUnifiedDiff("a/"+filename, "b/"+filename, oldText, edits, 4)
	if err != nil {
		return ChangeDescription{}, fmt.Errorf("generate edit diff: %w", err)
	}
	return ChangeDescription{
		Diff:             formatDisplayDiff(oldText, edits),
		Patch:            unified.String(),
		FirstChangedLine: firstChangedLineBytes([]byte(oldText), []byte(newText)),
	}, nil
}

func normalizeChangeContent(content []byte) string {
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		content = content[3:]
	}
	return normalizeLineEndings(string(content))
}

// formatDisplayDiff renders the same line edits used by the unified patch in
// Pi's compact numbered form. It intentionally leaves file and hunk headers
// to the machine-consumable unified patch.
func formatDisplayDiff(oldText string, edits []udiff.Edit) string {
	operations := displayDiffOperations(oldText, edits)
	if len(operations) == 0 {
		return ""
	}

	lineNumberWidth := 1
	for _, operation := range operations {
		lineNumberWidth = max(lineNumberWidth, len(fmt.Sprint(max(operation.oldLine, operation.newLine))))
	}
	return strings.Join(renderDisplayHunks(operations, lineNumberWidth), "\n")
}

type displayDiffOperation struct {
	content          string
	oldLine, newLine int
	added, removed   bool
}

func displayDiffOperations(oldText string, edits []udiff.Edit) []displayDiffOperation {
	operations := make([]displayDiffOperation, 0)
	oldOffset := 0
	oldLine, newLine := 1, 1
	for _, edit := range edits {
		operations, oldLine, newLine = appendDisplayOperations(
			operations, oldText[oldOffset:edit.Start], oldLine, newLine, false, false,
		)
		operations, oldLine, newLine = appendDisplayOperations(
			operations, oldText[edit.Start:edit.End], oldLine, newLine, false, true,
		)
		operations, oldLine, newLine = appendDisplayOperations(
			operations, edit.New, oldLine, newLine, true, false,
		)
		oldOffset = edit.End
	}
	operations, _, _ = appendDisplayOperations(operations, oldText[oldOffset:], oldLine, newLine, false, false)
	return operations
}

func appendDisplayOperations(
	operations []displayDiffOperation,
	content string,
	oldLine, newLine int,
	added, removed bool,
) ([]displayDiffOperation, int, int) {
	for _, line := range displayDiffLines(content) {
		operations = append(operations, displayDiffOperation{
			content: line, oldLine: oldLine, newLine: newLine, added: added, removed: removed,
		})
		switch {
		case added:
			newLine++
		case removed:
			oldLine++
		default:
			oldLine++
			newLine++
		}
	}
	return operations, oldLine, newLine
}

func renderDisplayHunks(operations []displayDiffOperation, lineNumberWidth int) []string {
	const contextLines = 4
	groups := displayChangeGroups(operations, contextLines)
	lines := make([]string, 0)
	previousEnd := 0
	for _, group := range groups {
		start := max(0, group.start-contextLines)
		end := min(len(operations), group.end+contextLines+1)
		if start > previousEnd {
			lines = append(lines, displayEllipsis(lineNumberWidth))
		}
		for _, operation := range operations[start:end] {
			lines = append(lines, formatDisplayOperation(operation, lineNumberWidth))
		}
		previousEnd = end
	}
	if previousEnd < len(operations) {
		lines = append(lines, displayEllipsis(lineNumberWidth))
	}
	return lines
}

type displayChangeGroup struct{ start, end int }

func displayChangeGroups(operations []displayDiffOperation, contextLines int) []displayChangeGroup {
	groups := make([]displayChangeGroup, 0)
	for index, operation := range operations {
		if !operation.added && !operation.removed {
			continue
		}
		if len(groups) == 0 || index-groups[len(groups)-1].end-1 > contextLines*2 {
			groups = append(groups, displayChangeGroup{start: index, end: index})
			continue
		}
		groups[len(groups)-1].end = index
	}
	return groups
}

func displayEllipsis(lineNumberWidth int) string {
	return fmt.Sprintf(" %*s ...", lineNumberWidth, "")
}

func formatDisplayOperation(operation displayDiffOperation, lineNumberWidth int) string {
	switch {
	case operation.added:
		return fmt.Sprintf("+%*d %s", lineNumberWidth, operation.newLine, operation.content)
	case operation.removed:
		return fmt.Sprintf("-%*d %s", lineNumberWidth, operation.oldLine, operation.content)
	default:
		return fmt.Sprintf(" %*d %s", lineNumberWidth, operation.oldLine, operation.content)
	}
}

func displayDiffLines(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for index, line := range lines {
		lines[index] = strings.TrimSuffix(line, "\n")
	}
	return lines
}

func normalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func firstChangedLineBytes(old, new []byte) int {
	limit := min(len(old), len(new))
	pos := 0
	for pos < limit && old[pos] == new[pos] {
		pos++
	}
	return bytes.Count(new[:pos], []byte("\n")) + 1
}
