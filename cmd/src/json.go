package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/srcop"
	"github.com/tta-lab/organon/internal/srcview"
)

// symbolOutlineJSON is the typed outline returned by `src symbols --json`.
type symbolOutlineJSON = srcview.SymbolResult

// readJSON adds CLI media handling to the shared machine-readable text result.
type readJSON struct {
	srcview.ReadResult
	Media *mediaJSON `json:"media,omitempty"`
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

// mutationJSON is the machine-readable result of a mutation. Symbol
// mutations include the resulting outline; exact-text mutations leave it
// omitted so their existing result contract is unchanged. Change fields
// intentionally match the batch edit result so both mutation families have the
// same Pi-facing contract.
type mutationJSON struct {
	Path             string             `json:"path"`
	Action           string             `json:"action"`
	SymbolID         string             `json:"symbol_id,omitempty"`
	Diff             string             `json:"diff"`
	Patch            string             `json:"patch"`
	FirstChangedLine int                `json:"first_changed_line,omitempty"`
	Outline          *symbolOutlineJSON `json:"outline,omitempty"`
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

// validateTextSource rejects unsupported image variants and binary input before
// a symbol parser can expose a text-looking prefix from an otherwise binary file.
func validateTextSource(filename string, source []byte) error {
	if looksLikeImageButUnsupported(source) || srcview.ValidateText(source) != nil {
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

// writeMutationJSON writes a symbol mutation result to disk and prints the
// same display diff, unified patch, and first-changed-line description used by
// batch edits, plus the typed outline of the resulting content.
func writeMutationJSON(filename, action, symbolID string, source, result []byte) error {
	return writeMutationJSONWithOutline(filename, action, symbolID, source, result, true)
}

// writeExactMutationJSON preserves the existing JSON result for exact-text
// edits, which intentionally do not report a symbol outline.
func writeExactMutationJSON(filename, action, symbolID string, source, result []byte) error {
	return writeMutationJSONWithOutline(filename, action, symbolID, source, result, false)
}

func writeMutationJSONWithOutline(
	filename, action, symbolID string, source, result []byte, includeOutline bool,
) error {
	description, err := srcop.DescribeChange(filename, source, result)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}

	output := mutationJSON{
		Path: filename, Action: action, SymbolID: symbolID,
		Diff: description.Diff, Patch: description.Patch,
		FirstChangedLine: description.FirstChangedLine,
	}
	if includeOutline {
		outline, err := srcview.BuildSymbolResult(filename, result, true)
		if err != nil {
			return fmt.Errorf("edit applied to %s, but post-edit outline reporting failed: %w", filename, err)
		}
		output.Outline = &outline
	}
	if err := printJSON(output); err != nil {
		return fmt.Errorf("edit applied to %s, but mutation result reporting failed: %w", filename, err)
	}
	return nil
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
	outline, err := srcview.BuildSymbolResult(filename, source, false)
	if err != nil {
		return err
	}
	return printJSON(outline)
}

// runRead dispatches between the human read and the JSON read; human output
// remains the default, --json opts into the machine-readable read result.
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
	limitSet := cmd.Flags().Changed("limit")
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	result, err := buildReadJSON(filename, source, symbolID, offset, limit, limitSet)
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
	limitSet := cmd.Flags().Changed("limit")

	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	result, err := buildReadJSON(filename, source, symbolID, offset, limit, limitSet)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func buildReadJSON(
	filename string, source []byte, symbolID string,
	offset, limit int, limitSet bool,
) (readJSON, error) {
	if symbolID != "" {
		if err := validateTextSource(filename, source); err != nil {
			return readJSON{}, err
		}
	} else {
		mediaResult, err := wholeFileMediaResult(filename, source, readJSON{
			ReadResult: srcview.ReadResult{Path: filename, TotalBytes: len(source)},
		})
		if err != nil {
			return readJSON{}, err
		}
		if mediaResult != nil {
			return *mediaResult, nil
		}
	}
	result, err := srcview.BuildReadResult(filename, source, symbolID, offset, limit, limitSet)
	if err != nil {
		return readJSON{}, err
	}
	return readJSON{ReadResult: result}, nil
}
