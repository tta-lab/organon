package srcview

import (
	"errors"
	"fmt"
)

// machineSymbolDepth is the fixed depth used by src symbols/read JSON and og MCP.
const machineSymbolDepth = 2

// SymbolResult is the machine-readable outline for one source file.
type SymbolResult struct {
	Path       string   `json:"path"`
	Language   string   `json:"language"`
	Title      string   `json:"title,omitempty"`
	TotalBytes int      `json:"total_bytes"`
	Symbols    []Symbol `json:"symbols"`
}

// BuildSymbolResult applies the current JSON outline contract to trusted source bytes.
// allowEmpty is used only to report a post-mutation outline.
func BuildSymbolResult(filename string, source []byte, allowEmpty bool) (SymbolResult, error) {
	if !allowEmpty {
		if err := ValidateText(source); err != nil {
			return SymbolResult{}, err
		}
	}
	inspector := NewInspector(filename, source, machineSymbolDepth)
	var outline Outline
	var err error
	if allowEmpty {
		outline, err = inspector.OutlineAllowEmpty()
	} else {
		outline, err = inspector.Outline()
	}
	if err != nil {
		return SymbolResult{}, err
	}
	if outline.Symbols == nil {
		outline.Symbols = []Symbol{}
	}
	return SymbolResult{Path: filename, Language: outline.Language, Title: outline.Title,
		TotalBytes: len(source), Symbols: outline.Symbols}, nil
}

// ReadResult is the machine-readable text result for one source file or symbol.
type ReadResult struct {
	Path                  string `json:"path"`
	SymbolID              string `json:"symbol_id,omitempty"`
	Content               string `json:"content"`
	StartLine             int    `json:"start_line"`
	TotalLines            int    `json:"total_lines"`
	TruncationTotalLines  int    `json:"truncation_total_lines"`
	TotalBytes            int    `json:"total_bytes"`
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncated_by,omitempty"`
	OutputLines           int    `json:"output_lines,omitempty"`
	OutputBytes           int    `json:"output_bytes,omitempty"`
	OutputEndLine         int    `json:"output_end_line,omitempty"`
	RemainingLines        int    `json:"remaining_lines,omitempty"`
	NextOffset            int    `json:"next_offset,omitempty"`
	FirstLineExceedsLimit bool   `json:"first_line_exceeds_limit,omitempty"`
}

// BuildReadResult validates text, selects a symbol/section if requested, then
// applies the shared one-indexed line window and continuation contract.
func BuildReadResult(filename string, source []byte, symbolID string,
	offset, limit int, limitSet bool) (ReadResult, error) {
	if err := ValidateText(source); err != nil {
		return ReadResult{}, err
	}
	content := string(source)
	if symbolID != "" {
		var err error
		content, err = NewInspector(filename, source, machineSymbolDepth).ReadContent(symbolID)
		if err != nil {
			return ReadResult{}, err
		}
	}
	window, err := NewReadWindow(content, offset, limit, limitSet)
	if err != nil {
		var offsetErr *OffsetOutOfRangeError
		if errors.As(err, &offsetErr) {
			return ReadResult{}, fmt.Errorf("offset %d is beyond end of %s (%d lines)",
				offsetErr.Offset, filename, offsetErr.TotalLines)
		}
		return ReadResult{}, err
	}
	return ReadResult{
		Path: filename, SymbolID: symbolID, Content: window.Content,
		StartLine: window.StartLine, TotalLines: window.TotalLines,
		TruncationTotalLines: window.TruncationTotalLines, TotalBytes: window.TotalBytes,
		Truncated: window.Truncated, TruncatedBy: window.TruncatedBy,
		OutputLines: window.OutputLines, OutputBytes: window.OutputBytes,
		OutputEndLine: window.OutputEndLine, RemainingLines: window.RemainingLines,
		NextOffset: window.NextOffset, FirstLineExceedsLimit: window.FirstLineExceedsLimit,
	}, nil
}
