package tree

import (
	"fmt"
	"strings"
)

// Node represents a tree item to render.
type Node struct {
	ID    string // 2-char base62 ID (empty for untargetable nodes)
	Label string // display text (e.g., "func main()", "## Install")
	Level int    // nesting depth (1 = top-level)
	Meta  string // optional suffix (e.g., "[L1-L15]", "(2,340 chars)")
}

// Render produces a box-drawing tree string from a flat list of nodes.
// Nodes are expected in document order with Level indicating nesting.
// Correctly handles indent continuation: uses "│   " under ├── parents
// and "    " (spaces) under └── parents.
func Render(nodes []Node) string {
	if len(nodes) == 0 {
		return "(empty)\n"
	}

	var sb strings.Builder

	minLevel := 99
	for _, n := range nodes {
		if n.Level < minLevel {
			minLevel = n.Level
		}
	}

	// isLastSibling[i] = true when node i is the last child at its depth
	isLastSibling := make([]bool, len(nodes))
	for i, n := range nodes {
		isLastSibling[i] = true
		for _, future := range nodes[i+1:] {
			if future.Level <= n.Level {
				isLastSibling[i] = future.Level < n.Level
				break
			}
		}
	}

	// levelHasMore[depth] = true means we should draw │ at that depth when indenting children
	levelHasMore := map[int]bool{}

	for i, n := range nodes {
		depth := n.Level - minLevel

		var indent strings.Builder
		for d := 0; d < depth; d++ {
			if levelHasMore[d] {
				indent.WriteString("│   ")
			} else {
				indent.WriteString("    ")
			}
		}

		connector := "├── "
		if isLastSibling[i] {
			connector = "└── "
			levelHasMore[depth] = false
		} else {
			levelHasMore[depth] = true
		}

		// Clean up deeper levels when we go back up
		for d := depth + 1; d < depth+10; d++ {
			delete(levelHasMore, d)
		}

		idStr := ""
		if n.ID != "" {
			idStr = fmt.Sprintf("[%s] ", n.ID)
		}

		metaStr := ""
		if n.Meta != "" {
			metaStr = "  " + n.Meta
		}

		fmt.Fprintf(&sb, "%s%s%s%s%s\n", indent.String(), connector, idStr, n.Label, metaStr)
	}

	return sb.String()
}
