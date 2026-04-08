package indent

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Kind int

const (
	Unknown Kind = iota
	Tab
	Space
)

// Style describes an indentation style.
// Invariants: if Kind is Tab, Width is ignored (tabs are undefined-width).
// If Kind is Space, Width must be 2, 4, or 8. These constraints are enforced
// by the Reindent function but not by the constructor; invalid combinations
// (e.g. Tab+Width=4) are silently accepted to allow legacy files to load.
type Style struct {
	Kind   Kind
	Width  int
	Source string
}

// Detect uses Layer 1 (filename lookup) first, then Layer 2 (content scan),
// then Layer 3 (fallback). Use this when you trust filename — e.g. for the
// target file being edited.
func Detect(filename string, source []byte) Style {
	if style := detectLayer1(filename); style.Kind != Unknown {
		return style
	}
	return detectLayer2(source)
}

// DetectByContent uses ONLY Layer 2 and Layer 3 — no filename lookup.
// Use this when filename cannot be trusted as a signal — e.g. for AFTER
// block content that the agent submitted. Even if the file is `.go`, the
// agent may have submitted AFTER with 4-space indent, and we need to see
// that rather than have Layer 1 short-circuit to "tab".
//
// This function is the reason Reindent works at all: without it,
// Reindent(afterText, targetGoFile) would always see from==target==tab
// and no-op, silently defeating the entire feature.
func DetectByContent(source []byte) Style {
	return detectLayer2(source)
}

// Layer 1: hardcoded table for opinionated languages.
func detectLayer1(filename string) Style {
	ext := strings.ToLower(filepath.Ext(filename))
	base := filepath.Base(filename)

	switch ext {
	case ".go":
		return Style{Tab, 0, "layer1:go"}
	case ".py":
		return Style{Space, 4, "layer1:python"}
	case ".rs":
		return Style{Space, 4, "layer1:rust"}
	case ".rb":
		return Style{Space, 2, "layer1:ruby"}
	case ".ex", ".exs":
		return Style{Space, 2, "layer1:elixir"}
	case ".lua":
		return Style{Space, 2, "layer1:lua"}
	case ".yaml", ".yml":
		return Style{Space, 2, "layer1:yaml"}
	}

	if base == "Makefile" || base == "makefile" || strings.HasPrefix(base, "Makefile.") {
		return Style{Tab, 0, "layer1:makefile"}
	}

	return Style{}
}

// Layer 2: per-file detection via 80% majority on first 200 non-blank, non-JSDoc lines.
func detectLayer2(source []byte) Style {
	var tabLines, spaceLines int
	var spaceWidths []int

	lines := strings.Split(string(source), "\n")
	checked := 0

	for _, line := range lines {
		if checked >= 200 {
			break
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue // blank line — skip
		}
		if strings.HasPrefix(trimmed, "*") {
			continue // JSDoc continuation line — skip
		}

		checked++
		prefix := line[:len(line)-len(trimmed)]

		if prefix == "" {
			continue
		}

		allTabs := strings.TrimLeft(prefix, "\t") == ""
		allSpaces := strings.TrimLeft(prefix, " ") == ""

		if allTabs && !allSpaces {
			tabLines++
		} else if allSpaces && !allTabs {
			spaceLines++
			spaceWidths = append(spaceWidths, len(prefix))
		}
		// Mixed prefix (tabs+spaces) — skip; don't count either way.
	}

	total := tabLines + spaceLines
	if total == 0 {
		return Style{Unknown, 0, "layer3:fallback"}
	}

	tabRatio := float64(tabLines) / float64(total)
	if tabRatio > 0.8 {
		return Style{Tab, 0, "layer2:majority-tab"}
	}

	spaceRatio := float64(spaceLines) / float64(total)
	if spaceRatio > 0.8 {
		width := inferSpaceWidth(spaceWidths)
		return Style{Space, width, fmt.Sprintf("layer2:majority-space-%d", width)}
	}

	return Style{Unknown, 0, "layer3:fallback"}
}

// inferSpaceWidth returns the most common space width (mode) from recorded widths.
// Defaults to 2 if no widths recorded.
func inferSpaceWidth(widths []int) int {
	if len(widths) == 0 {
		return 2
	}
	// Count frequency of each width.
	freq := make(map[int]int)
	for _, w := range widths {
		freq[w]++
	}
	// Find mode.
	mode := widths[0]
	maxCount := freq[mode]
	for w, c := range freq {
		if c > maxCount || (c == maxCount && w > mode) {
			maxCount = c
			mode = w
		}
	}
	// Round to nearest standard width.
	if mode >= 4 {
		return 4
	}
	return 2
}
