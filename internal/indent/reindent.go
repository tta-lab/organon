package indent

import (
	"strings"
)

// Reindent transforms text so leading whitespace matches target. It detects text's
// own style via DetectByContent (Layer 2 only — filename is untrustworthy for AFTER
// content). If source style is Unknown, text is returned unchanged with ok=false
// so the caller can emit a warning. If source style equals target, text is returned
// unchanged with ok=true.
//
// warnings contains per-line messages when a line's prefix could not be transformed
// (mixed tab+space, or non-divisible space count). nil when the transform is clean.
func Reindent(text []byte, target Style) (result []byte, from Style, ok bool, warnings []string) {
	from = DetectByContent(text)
	if from.Kind == Unknown {
		return text, from, true, nil // no indent to transform, treat as success
	}
	if from == target {
		return text, from, true, nil
	}

	lines := strings.Split(string(text), "\n")
	var out []string
	for _, line := range lines {
		newLine, warning := reindentLine(line, from, target)
		out = append(out, newLine)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return []byte(strings.Join(out, "\n")), from, true, warnings
}

// reindentLine transforms a single line's leading whitespace to match target style.
// Returns a warning string if the line's prefix could not be cleanly transformed.
func reindentLine(line string, from, target Style) (string, string) {
	// Find leading whitespace prefix.
	prefixLen := leadingPrefixLen(line)
	prefix := line[:prefixLen]
	rest := line[prefixLen:]

	// Empty prefix — nothing to do.
	if prefix == "" {
		return line, ""
	}

	// Detect mixed prefix (both tabs and spaces).
	hasTab := strings.Contains(prefix, "\t")
	hasSpace := strings.Contains(prefix, " ")
	if hasTab && hasSpace {
		return line, "mixed tab+space prefix preserved as-is"
	}

	var level int

	if from.Kind == Tab {
		// Tab-based: level = count of leading tabs.
		if hasSpace {
			return line, "mixed tab+space prefix preserved as-is"
		}
		level = strings.Count(prefix, "\t")
	} else {
		// Space-based: level = total spaces / width.
		width := from.Width
		if width == 0 {
			width = 2 // avoid div-by-zero
		}
		spaceCount := len(prefix)
		if spaceCount%width != 0 {
			return line, "space prefix not divisible by indent width; preserved as-is"
		}
		level = spaceCount / width
	}

	// Rewrite prefix in target style.
	var newPrefix string
	if target.Kind == Tab {
		newPrefix = strings.Repeat("\t", level)
	} else {
		width := target.Width
		if width == 0 {
			width = 2
		}
		newPrefix = strings.Repeat(" ", level*width)
	}

	return newPrefix + rest, ""
}

// leadingPrefixLen returns the byte length of the leading whitespace prefix of line.
func leadingPrefixLen(line string) int {
	prefixLen := 0
	for prefixLen < len(line) && (line[prefixLen] == '\t' || line[prefixLen] == ' ') {
		prefixLen++
	}
	return prefixLen
}
