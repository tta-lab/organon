package srcop

import (
	"fmt"

	"github.com/tta-lab/organon/internal/treesitter"
)

// Replace replaces a symbol's byte range with new content.
// Returns the modified file content.
// Note: Replace does not remove the existing doc comment (DocStart..StartByte).
// The caller is responsible for including or omitting comment text in newContent.
// Use WriteComment to update the doc comment independently.
func Replace(filename string, source []byte, symbolID string, newContent []byte, depth int) ([]byte, error) {
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
	result := make([]byte, 0, len(source))
	result = append(result, source[:sym.StartByte]...)
	result = append(result, newContent...)
	result = append(result, source[sym.EndByte:]...)

	return result, nil
}
