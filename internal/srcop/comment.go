package srcop

import (
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/treesitter"
)

// ReadComment extracts the doc comment attached to a symbol.
// Returns empty string if no doc comment exists.
func ReadComment(filename string, source []byte, symbolID string, depth int) (string, error) {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return "", err
	}

	nodes := treesitter.SymbolTree(symbols)
	for i, n := range nodes {
		if n.ID == symbolID {
			sym := symbols[i]
			if sym.DocStart < 0 {
				return "", nil
			}
			return string(source[sym.DocStart:sym.StartByte]), nil
		}
	}
	return "", fmt.Errorf("symbol %q not found", symbolID)
}

// WriteComment replaces or adds a doc comment on a symbol.
// Detects comment style from file extension via treesitter.LangNameFromExt.
// Returns the new file content.
func WriteComment(filename string, source []byte, symbolID string, comment []byte, depth int) ([]byte, error) {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return nil, err
	}

	nodes := treesitter.SymbolTree(symbols)
	targetIdx := findSymbolIndex(nodes, symbolID)
	if targetIdx < 0 {
		return nil, fmt.Errorf("symbol %q not found", symbolID)
	}

	sym := symbols[targetIdx]
	lang, _ := treesitter.LangNameFromExt(filename)
	formatted := formatComment(string(comment), lang)

	var result []byte
	if sym.DocStart >= 0 {
		// Replace existing comment
		result = append(result, source[:sym.DocStart]...)
		result = append(result, []byte(formatted)...)
		result = append(result, source[sym.StartByte:]...)
	} else {
		// Insert new comment before symbol
		result = append(result, source[:sym.StartByte]...)
		result = append(result, []byte(formatted)...)
		result = append(result, source[sym.StartByte:]...)
	}

	return result, nil
}

// formatComment wraps raw text in language-appropriate comment syntax.
// Always ends with a newline so it separates cleanly from the symbol below.
func formatComment(text, lang string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var sb strings.Builder
	prefix := "// "
	switch lang {
	case "python", "ruby":
		prefix = "# "
	}
	for _, line := range lines {
		sb.WriteString(prefix)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}
