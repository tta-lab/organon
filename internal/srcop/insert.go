package srcop

import (
	"bytes"
	"fmt"

	"github.com/tta-lab/organon/internal/treesitter"
)

// InsertAfter inserts new content after the symbol identified by symbolID.
// Returns the modified file content.
func InsertAfter(filename string, source []byte, symbolID string, newContent []byte, depth int) ([]byte, error) {
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
	insertPos := sym.EndByte

	// Ensure there's a newline before the new content
	prefix := []byte("\n\n")
	if insertPos < uint(len(source)) && source[insertPos] == '\n' {
		prefix = []byte("\n")
	}

	result := make([]byte, 0, uint(len(source))+uint(len(newContent))+2)
	result = append(result, source[:insertPos]...)
	result = append(result, prefix...)
	result = append(result, bytes.TrimLeft(newContent, "\n")...)
	result = append(result, source[insertPos:]...)

	return result, nil
}

// InsertBefore inserts new content before the symbol identified by symbolID.
// Returns the modified file content.
func InsertBefore(filename string, source []byte, symbolID string, newContent []byte, depth int) ([]byte, error) {
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
	insertPos := sym.StartByte
	// If there's a doc comment, insert before it
	if sym.DocStart >= 0 {
		insertPos = uint(sym.DocStart)
	}

	content := bytes.TrimRight(newContent, "\n")

	result := make([]byte, 0, uint(len(source))+uint(len(content))+2)
	result = append(result, source[:insertPos]...)
	result = append(result, content...)
	result = append(result, '\n', '\n')
	result = append(result, source[insertPos:]...)

	return result, nil
}
