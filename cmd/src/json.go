package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/srcview"
	"github.com/tta-lab/organon/internal/truncate"
)

// symbolOutlineJSON is the typed outline returned by `src symbols --json`.
type symbolOutlineJSON struct {
	Path       string           `json:"path"`
	Language   string           `json:"language"`
	Title      string           `json:"title,omitempty"`
	TotalBytes int              `json:"total_bytes"`
	Symbols    []srcview.Symbol `json:"symbols"`
}

// readJSON is the machine-readable result of `src read --json`.
type readJSON struct {
	Path                  string `json:"path"`
	SymbolID              string `json:"symbol_id,omitempty"`
	Content               string `json:"content"`
	StartLine             int    `json:"start_line"`
	TotalLines            int    `json:"total_lines"`
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncated_by,omitempty"`
	OutputLines           int    `json:"output_lines,omitempty"`
	OutputBytes           int    `json:"output_bytes,omitempty"`
	NextOffset            int    `json:"next_offset,omitempty"`
	FirstLineExceedsLimit bool   `json:"first_line_exceeds_limit,omitempty"`
	Media                 *mediaJSON `json:"media,omitempty"`
}

// mediaJSON carries a recognized image so the Pi adapter can attach media
// without ever decoding it as UTF-8 text.
type mediaJSON struct {
	Kind     string `json:"kind"`
	Mime     string `json:"mime"`
	DataBase64 string `json:"data_base64"`
}

func printJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

// runSymbolsJSON implements `src symbols <file> --json` with the extension's
// fixed depth of 2 so every later symbol operation resolves IDs from the same
// outline shape.
func runSymbolsJSON(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	outline, err := srcview.NewInspector(filename, source, 2).Outline()
	if err != nil {
		return err
	}
	return printJSON(symbolOutlineJSON{
		Path:       filename,
		Language:   outline.Language,
		Title:      outline.Title,
		TotalBytes: len(source),
		Symbols:    outline.Symbols,
	})
}

// runReadJSON implements `src read <file> --json`. Offset is a 1-indexed line
// offset and limit a maximum line count, both relative to the selected content
// (the whole file, or the exact symbol/section when symbol_id is present).
func runReadJSON(cmd *cobra.Command, args []string) error {
	filename := args[0]
	symbolID, _ := cmd.Flags().GetString("symbol-id")
	offset, _ := cmd.Flags().GetInt("offset")
	limit, _ := cmd.Flags().GetInt("limit")

	if offset < 0 {
		return fmt.Errorf("offset must be 1 or greater")
	}
	if limit < 0 {
		return fmt.Errorf("limit must be zero or greater")
	}

	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	result, err := buildReadJSON(filename, source, symbolID, offset, limit)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func buildReadJSON(filename string, source []byte, symbolID string, offset, limit int) (readJSON, error) {
	content := string(source)
	startLine := 1
	totalLines := strings.Count(content, "\n") + 1

	if symbolID != "" {
		read, err := srcview.NewInspector(filename, source, 2).ReadSymbolLines(symbolID)
		if err != nil {
			return readJSON{}, err
		}
		content, startLine, totalLines = read.Content, read.StartLine, read.TotalLines
	}

	result := readJSON{
		Path: filename, SymbolID: symbolID, TotalLines: totalLines,
	}

	if media, ok := detectMedia(source); ok && symbolID == "" {
		result.Media = &media
		result.Content = ""
		return result, nil
	}

	lines := strings.Split(content, "\n")
	startIdx := 0
	if offset > 1 {
		startIdx = offset - 1
	}
	if startIdx > len(lines) {
		return readJSON{}, fmt.Errorf("offset %d is beyond end of %s (%d lines)", offset, filename, totalLines)
	}
	if limit > 0 && startIdx+limit < len(lines) {
		lines = lines[startIdx : startIdx+limit]
	} else {
		lines = lines[startIdx:]
	}
	content = strings.Join(lines, "\n")
	result.StartLine = startLine + startIdx

	tr := truncate.Head(content, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	result.Content = tr.Content
	result.Truncated = tr.Truncated
	result.TruncatedBy = tr.TruncatedBy
	result.OutputLines = tr.OutputLines
	result.OutputBytes = tr.OutputBytes
	result.FirstLineExceedsLimit = tr.FirstLineExceedsLimit
	if tr.Truncated && !tr.FirstLineExceedsLimit {
		result.NextOffset = result.StartLine + tr.OutputLines
	}
	return result, nil
}

