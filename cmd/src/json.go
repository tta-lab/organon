package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/aymanbagabas/go-udiff"
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

// readJSON is the machine-readable result of `src read --json`. Line
// positions (StartLine, TotalLines, NextOffset) are relative to the selected
// content: the whole file, or the exact symbol/section when symbol_id is
// present. TotalLines uses pagination's addressable-line model;
// TruncationTotalLines uses Pi truncateHead's counted-line model. Offset and
// limit are 1-indexed line positions in the same frame.
type readJSON struct {
	Path                  string     `json:"path"`
	SymbolID              string     `json:"symbol_id,omitempty"`
	Content               string     `json:"content"`
	StartLine             int        `json:"start_line"`
	TotalLines            int        `json:"total_lines"`
	TruncationTotalLines  int        `json:"truncation_total_lines,omitempty"`
	TotalBytes            int        `json:"total_bytes"`
	Truncated             bool       `json:"truncated"`
	TruncatedBy           string     `json:"truncated_by,omitempty"`
	OutputLines           int        `json:"output_lines,omitempty"`
	OutputBytes           int        `json:"output_bytes,omitempty"`
	NextOffset            int        `json:"next_offset,omitempty"`
	FirstLineExceedsLimit bool       `json:"first_line_exceeds_limit,omitempty"`
	Media                 *mediaJSON `json:"media,omitempty"`
}

// mediaJSON carries a recognized image so the Pi adapter can attach media
// without ever decoding it as UTF-8 text.
type mediaJSON struct {
	Kind       string `json:"kind"`
	Mime       string `json:"mime"`
	DataBase64 string `json:"data_base64"`
}

func printJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

// mutationJSON is the machine-readable result of a symbol mutation.
type mutationJSON struct {
	Path             string `json:"path"`
	Action           string `json:"action"`
	SymbolID         string `json:"symbol_id,omitempty"`
	Diff             string `json:"diff"`
	FirstChangedLine int    `json:"first_changed_line,omitempty"`
}

// commentJSON is the machine-readable result of a comment read.
type commentJSON struct {
	Path     string `json:"path"`
	SymbolID string `json:"symbol_id"`
	Comment  string `json:"comment"`
}

// editBatchJSON is the machine-readable result of `src edit --edits-json --json`.
type editBatchJSON struct {
	Path             string `json:"path"`
	Diff             string `json:"diff"`
	Patch            string `json:"patch"`
	FirstChangedLine int    `json:"first_changed_line,omitempty"`
	EditsApplied     int    `json:"edits_applied"`
}

// commonBinarySignatures identify non-text formats whose headers may otherwise
// be valid UTF-8 and contain no NUL byte. Recognized images are handled before
// this classifier so supported image files still become media attachments.
var commonBinarySignatures = [][]byte{
	[]byte("%PDF-"),
	[]byte("PK\x03\x04"), // ZIP local file header
	[]byte("PK\x05\x06"), // ZIP empty archive header
	[]byte("PK\x07\x08"), // ZIP spanned archive header
	{0x7F, 'E', 'L', 'F'},
	{0xFE, 0xED, 0xFA, 0xCE}, // Mach-O 32-bit big-endian
	{0xCE, 0xFA, 0xED, 0xFE}, // Mach-O 32-bit little-endian
	{0xFE, 0xED, 0xFA, 0xCF}, // Mach-O 64-bit big-endian
	{0xCF, 0xFA, 0xED, 0xFE}, // Mach-O 64-bit little-endian
	{0xCA, 0xFE, 0xBA, 0xBE}, // Mach-O universal binary
	{0xBE, 0xBA, 0xFE, 0xCA}, // Mach-O universal binary, byte-swapped
	{0xCA, 0xFE, 0xBA, 0xBF}, // Mach-O 64-bit universal binary
	{0xBF, 0xBA, 0xFE, 0xCA}, // Mach-O 64-bit universal binary, byte-swapped
	{0x00, 'a', 's', 'm'},
}

// isBinaryBytes reports binary content via a NUL byte in the first 8 KiB or a
// common binary signature at the start of the file.
func isBinaryBytes(data []byte) bool {
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	return bytes.IndexByte(check, 0) >= 0 || hasBinarySignature(data)
}

func hasBinarySignature(data []byte) bool {
	for _, signature := range commonBinarySignatures {
		if bytes.HasPrefix(data, signature) {
			return true
		}
	}
	return false
}

// validateTextSource rejects unsupported image variants and binary input before
// a symbol parser can expose a text-looking prefix from an otherwise binary file.
func validateTextSource(filename string, source []byte) error {
	if looksLikeImageButUnsupported(source) || isBinaryBytes(source) || !utf8.Valid(source) {
		return mediaErrorFor(source, filename)
	}
	return nil
}

// wholeFileMediaResult returns a supported-image result or validates a text
// whole-file read. Symbol reads call validateTextSource before extraction.
func wholeFileMediaResult(filename string, source []byte, result readJSON) (*readJSON, error) {
	if media, ok := detectMedia(source); ok {
		result.Media = &media
		result.Content = ""
		return &result, nil
	}
	if err := validateTextSource(filename, source); err != nil {
		return nil, err
	}
	return nil, nil
}

// readOffset accepts zero only as Cobra's internal sentinel for an omitted
// offset. An explicit --offset=0 violates the public one-indexed contract.
func readOffset(cmd *cobra.Command) (int, error) {
	offset, _ := cmd.Flags().GetInt("offset")
	if offset < 0 || (offset == 0 && cmd.Flags().Changed("offset")) {
		return 0, fmt.Errorf("offset must be 1 or greater")
	}
	return offset, nil
}

func targetID(afterID, beforeID string) string {
	if afterID != "" {
		return afterID
	}
	return beforeID
}

// writeMutationJSON writes the mutation result to disk and prints the
// machine-readable result, keeping diagnostics off stdout.
func writeMutationJSON(filename, action, symbolID string, source, result []byte) error {
	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}
	diffText, first := diffSummary(filename, source, result)
	return printJSON(mutationJSON{
		Path: filename, Action: action, SymbolID: symbolID,
		Diff: diffText, FirstChangedLine: first,
	})
}

// diffSummary renders a unified diff between old and new content and the
// 1-indexed line of the first change in the new file.
func diffSummary(filename string, old, new []byte) (string, int) {
	diffText := udiff.Unified("a/"+filename, "b/"+filename, string(old), string(new))
	return diffText, firstChangedLineBytes(old, new)
}

// firstChangedLineBytes returns the 1-indexed line of the first differing byte
// in the new content, mirroring the Pi built-in edit's reporting of the first
// changed line in the new file.
func firstChangedLineBytes(old, new []byte) int {
	limit := min(len(old), len(new))
	pos := 0
	for pos < limit && old[pos] == new[pos] {
		pos++
	}
	return bytes.Count(new[:pos], []byte("\n")) + 1
}

// runSymbols dispatches between the human outline and the JSON outline; human
// output remains the default, --json opts into the machine-readable form.
func runSymbols(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return runSymbolsJSON(cmd, args)
	}
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := validateTextSource(filename, source); err != nil {
		return err
	}
	rendered, err := srcview.NewInspector(filename, source, 2).RenderTree()
	if err != nil {
		return err
	}
	fmt.Print(rendered)
	return nil
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
	if err := validateTextSource(filename, source); err != nil {
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

// runRead dispatches between the human read and the JSON read; human output
// remains the default, --json opts into the machine-readable form.
func runRead(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return runReadJSON(cmd, args)
	}
	filename := args[0]
	symbolID, _ := cmd.Flags().GetString("symbol-id")
	offset, err := readOffset(cmd)
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
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
	if result.Media != nil {
		fmt.Printf("Read image file [%s]\n", result.Media.Mime)
		return nil
	}
	if result.FirstLineExceedsLimit {
		return fmt.Errorf("the first line of %s exceeds the 50KB read limit", filename)
	}
	fmt.Print(result.Content)
	return nil
}

// runReadJSON implements `src read <file> --json`. Offset is a 1-indexed line
// offset and limit a maximum line count, both relative to the selected content
// (the whole file, or the exact symbol/section when symbol_id is present).
func runReadJSON(cmd *cobra.Command, args []string) error {
	filename := args[0]
	symbolID, _ := cmd.Flags().GetString("symbol-id")
	offset, err := readOffset(cmd)
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")

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
	var content string
	if symbolID != "" {
		if err := validateTextSource(filename, source); err != nil {
			return readJSON{}, err
		}
		symbolContent, err := srcview.NewInspector(filename, source, 2).ReadContent(symbolID)
		if err != nil {
			return readJSON{}, err
		}
		content = symbolContent
	} else {
		content = string(source)
	}

	// Pi's built-in read treats strings.Split(text, "\n") as its pagination
	// model: an empty file is one empty line, and a trailing newline creates an
	// addressable final empty line. truncate.Head uses its own counted-line
	// model, which excludes that terminal empty segment.
	lines := strings.Split(content, "\n")
	result := readJSON{
		Path: filename, SymbolID: symbolID, TotalLines: len(lines), TotalBytes: len(content),
	}

	if symbolID == "" {
		mediaResult, err := wholeFileMediaResult(filename, source, result)
		if err != nil {
			return readJSON{}, err
		}
		if mediaResult != nil {
			return *mediaResult, nil
		}
	}

	startIdx := 0
	if offset > 0 {
		startIdx = offset - 1
	}
	if startIdx >= len(lines) {
		return readJSON{}, fmt.Errorf("offset %d is beyond end of %s (%d lines)", offset, filename, len(lines))
	}

	selected := lines[startIdx:]
	userLimited := false
	if limit > 0 && limit < len(selected) {
		selected = selected[:limit]
		userLimited = true
	}
	content = strings.Join(selected, "\n")
	result.StartLine = 1 + startIdx

	tr := truncate.Head(content, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	result.Content = tr.Content
	result.Truncated = tr.Truncated
	result.TruncatedBy = tr.TruncatedBy
	result.OutputLines = tr.OutputLines
	result.OutputBytes = tr.OutputBytes
	result.FirstLineExceedsLimit = tr.FirstLineExceedsLimit
	if tr.Truncated {
		result.TruncationTotalLines = tr.TotalLines
	}
	// A continuation applies only when Pi's output truncation stopped early or
	// an explicit limit selected fewer addressable lines. A terminal empty
	// segment is addressable for pagination but does not itself cause truncation.
	if !tr.FirstLineExceedsLimit {
		switch {
		case tr.Truncated:
			result.NextOffset = result.StartLine + tr.OutputLines
		case userLimited:
			result.NextOffset = result.StartLine + len(selected)
		}
	}
	return result, nil
}
