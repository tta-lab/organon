package srcop

import (
	"fmt"

	"github.com/tta-lab/organon/internal/tree"
	"github.com/tta-lab/organon/internal/treesitter"
)

// Delete removes a symbol and its doc comment from the file.
// Returns the modified file content.
func Delete(filename string, source []byte, symbolID string, depth int) ([]byte, error) {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return nil, err
	}

	nodes := treesitter.SymbolTree(symbols)
	targetIdx := findSymbolIndex(nodes, symbolID)
	if targetIdx < 0 {
		return nil, fmt.Errorf("symbol %q not found; run --tree to see current IDs", symbolID)
	}

	sym := symbols[targetIdx]

	// Start from doc comment if present, otherwise from symbol start
	start := sym.StartByte
	if sym.DocStart >= 0 {
		start = uint(sym.DocStart)
	}
	end := sym.EndByte

	// Extend end to consume trailing newline(s)
	for end < uint(len(source)) && source[end] == '\n' {
		end++
	}
	// But don't consume more than 1 extra blank line
	if end > sym.EndByte+1 {
		end = sym.EndByte + 1
	}

	result := make([]byte, 0, len(source))
	result = append(result, source[:start]...)
	result = append(result, source[end:]...)

	// Clean up consecutive blank lines at deletion site
	result = cleanBlankLines(result)

	return result, nil
}

// findSymbolIndex returns the index of a symbol by its tree node ID, or -1.
func findSymbolIndex(nodes []tree.Node, symbolID string) int {
	for i, n := range nodes {
		if n.ID == symbolID {
			return i
		}
	}
	return -1
}

// cleanBlankLines reduces sequences of 3+ consecutive newlines to 2.
func cleanBlankLines(src []byte) []byte {
	result := make([]byte, 0, len(src))
	blankCount := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			blankCount++
			if blankCount <= 2 {
				result = append(result, src[i])
			}
		} else {
			blankCount = 0
			result = append(result, src[i])
		}
	}
	return result
}
