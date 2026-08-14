package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/diff"
	"github.com/tta-lab/organon/internal/indent"
	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/srcop"
	"github.com/tta-lab/organon/internal/srcview"
	"github.com/tta-lab/organon/internal/tree"
	"github.com/tta-lab/organon/internal/treesitter"
)

func isMarkdown(filename string) bool {
	return srcview.IsMarkdown(filename)
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "src <file> [flags]",
		Short: "Structure-aware source file reading and editing",
		Long:  helpRoot,
		Args:  cobra.ExactArgs(1),
		RunE:  runTreeOrRead,
	}
	root.SilenceUsage = true

	// Persistent flag — inherited by all subcommands
	root.PersistentFlags().Int("depth", 2, "Symbol tree depth (default 2)")

	// Root-only flags
	root.Flags().Bool("tree", false, "Force tree view")
	root.Flags().StringP("symbol-id", "s", "", "Symbol ID to read")

	replaceCmd := &cobra.Command{
		Use:   "replace <file> --symbol-id <id>",
		Short: "Replace a symbol (new content via stdin)",
		Long:  helpReplace,
		Args:  cobra.ExactArgs(1),
		RunE:  runReplace,
	}
	replaceCmd.SilenceUsage = true
	replaceCmd.Flags().StringP("symbol-id", "s", "", "Symbol ID to replace")
	replaceCmd.Flags().Bool("json", false, "Output the machine-readable result as JSON")
	_ = replaceCmd.MarkFlagRequired("symbol-id")

	insertCmd := &cobra.Command{
		Use:   "insert <file> --after <id>|--before <id>",
		Short: "Insert content before/after a symbol (stdin)",
		Long:  helpInsert,
		Args:  cobra.ExactArgs(1),
		RunE:  runInsert,
	}
	insertCmd.SilenceUsage = true
	insertCmd.Flags().String("after", "", "Insert after symbol ID")
	insertCmd.Flags().String("before", "", "Insert before symbol ID")
	insertCmd.Flags().Bool("json", false, "Output the machine-readable result as JSON")

	deleteCmd := &cobra.Command{
		Use:   "delete <file> --symbol-id <id>",
		Short: "Delete a symbol",
		Long:  helpDelete,
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}
	deleteCmd.SilenceUsage = true
	deleteCmd.Flags().StringP("symbol-id", "s", "", "Symbol ID to delete")
	deleteCmd.Flags().Bool("json", false, "Output the machine-readable result as JSON")
	_ = deleteCmd.MarkFlagRequired("symbol-id")

	commentCmd := &cobra.Command{
		Use:   "comment <file> --symbol-id <id> [--read]",
		Short: "Read or write a doc comment on a symbol (stdin for write)",
		Long:  helpComment,
		Args:  cobra.ExactArgs(1),
		RunE:  runComment,
	}
	commentCmd.SilenceUsage = true
	commentCmd.Flags().StringP("symbol-id", "s", "", "Symbol ID")
	commentCmd.Flags().Bool("read", false, "Read existing doc comment instead of writing")
	commentCmd.Flags().Bool("json", false, "Output the machine-readable result as JSON")
	_ = commentCmd.MarkFlagRequired("symbol-id")

	editCmd := &cobra.Command{
		Use:   "edit <file> [--symbol-id <id>] [--before-file <f> --after-file <f>]",
		Short: "Replace text using exact match (stdin: ===BEFORE===/===AFTER===)",
		Long:  helpEdit,
		Args:  cobra.ExactArgs(1),
		RunE:  runEdit,
	}
	editCmd.SilenceUsage = true
	editCmd.Flags().StringP("symbol-id", "s", "",
		"Scope edit to a symbol/section ID (use src <file> to find IDs)")
	editCmd.Flags().String("before-file", "",
		"Read BEFORE content from a file instead of stdin (use with --after-file)")
	editCmd.Flags().String("after-file", "",
		"Read AFTER content from a file instead of stdin (use with --before-file)")
	editCmd.Flags().Bool("edits-json", false,
		"Read a JSON edits envelope from stdin: {\"edits\":[{\"oldText\":...,\"newText\":...}]}")
	editCmd.Flags().Bool("json", false, "Output the machine-readable result as JSON")

	symbolsCmd := &cobra.Command{
		Use:   "symbols <file>",
		Short: "Show the typed symbol outline (JSON with --json)",
		Long:  helpSymbols,
		Args:  cobra.ExactArgs(1),
		RunE:  runSymbols,
	}
	symbolsCmd.SilenceUsage = true
	symbolsCmd.Flags().Bool("json", false, "Output the typed outline as JSON")

	readCmd := &cobra.Command{
		Use:   "read <file>",
		Short: "Read a file, symbol, or line range (JSON with --json)",
		Long:  helpRead,
		Args:  cobra.ExactArgs(1),
		RunE:  runRead,
	}
	readCmd.SilenceUsage = true
	readCmd.Flags().StringP("symbol-id", "s", "", "Symbol or Markdown section ID to read")
	readCmd.Flags().Int("offset", 0, "1-indexed line offset within the selected content")
	readCmd.Flags().Int("limit", 0, "Maximum number of lines to read (0 = all)")
	readCmd.Flags().Bool("json", false, "Output the machine-readable read result as JSON")

	root.AddCommand(replaceCmd, insertCmd, deleteCmd, commentCmd, editCmd, symbolsCmd, readCmd)
	return root
}

// getDepth reads the --depth persistent flag from the root command.
// IMPORTANT: Must use cmd.Root().PersistentFlags() — NOT cmd.Flags() —
// because cmd.Flags() on subcommands does NOT include inherited persistent flags.
func getDepth(cmd *cobra.Command) int {
	depth, err := cmd.Root().PersistentFlags().GetInt("depth")
	if err != nil {
		panic("BUG: --depth flag not registered")
	}
	return depth
}

func printDisclosure(w io.Writer, r *srcop.EditResult) {
	if r.Pass != "exact" {
		fmt.Fprintf(w, "matched via: %s pass\n", r.Pass)
	}
	if r.Reindented {
		fmt.Fprintf(w, "AFTER re-indented: %s → %s\n",
			styleLabel(r.IndentFrom), styleLabel(r.IndentTo))
	}
	for _, msg := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", msg)
	}
}

func styleLabel(s indent.Style) string {
	switch s.Kind {
	case indent.Tab:
		return "tab"
	case indent.Space:
		return fmt.Sprintf("%d-space", s.Width)
	default:
		return "unknown"
	}
}

func runTreeOrRead(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	depth := getDepth(cmd)
	symbolID, _ := cmd.Flags().GetString("symbol-id")
	treeOnly, _ := cmd.Flags().GetBool("tree")
	inspector := srcview.NewInspector(filename, source, depth)
	outline, err := inspector.Outline()
	if errors.Is(err, srcview.ErrNoStructure) {
		if symbolID != "" {
			return noStructureError(filename, "reading by --symbol-id")
		}
		if treeOnly {
			return noSymbolTreeError(filename)
		}
		fmt.Print(string(source))
		return nil
	}
	if err != nil {
		return err
	}

	if symbolID != "" {
		content, err := inspector.ReadContent(symbolID)
		if err != nil {
			if isMarkdown(filename) {
				return err
			}
			return fmt.Errorf("symbol %q not found; run --tree to see current IDs", symbolID)
		}
		fmt.Print(content)
		return nil
	}
	if len(outline.Symbols) == 0 && !isMarkdown(filename) {
		if treeOnly {
			return noSymbolTreeError(filename)
		}
		fmt.Print(string(source))
		return nil
	}
	rendered, err := inspector.RenderTree()
	if err != nil {
		return err
	}
	fmt.Print(rendered)
	return nil
}

func runReplace(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	symbolID, _ := cmd.Flags().GetString("symbol-id")
	depth := getDepth(cmd)
	jsonOut, _ := cmd.Flags().GetBool("json")

	newContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	if isMarkdown(filename) {
		result, err := markdown.ReplaceSection(source, symbolID, newContent)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeMutationJSON(filename, "replace", symbolID, source, result)
		}
		return writeAndShow(filename, source, result, depth)
	}
	if !hasTreeSitterSupport(filename) {
		return noStructureError(filename, "replace")
	}
	hasSymbols, err := hasSymbolTree(filename, source, depth)
	if err != nil {
		return err
	}
	if !hasSymbols {
		return noStructureError(filename, "replace")
	}

	result, err := srcop.Replace(filename, source, symbolID, newContent, depth)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeMutationJSON(filename, "replace", symbolID, source, result)
	}

	return writeAndShow(filename, source, result, depth)
}

func runInsert(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	afterID, _ := cmd.Flags().GetString("after")
	beforeID, _ := cmd.Flags().GetString("before")
	depth := getDepth(cmd)
	jsonOut, _ := cmd.Flags().GetBool("json")

	if afterID == "" && beforeID == "" {
		return fmt.Errorf("either --after or --before is required")
	}

	newContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	if isMarkdown(filename) {
		var result []byte
		if afterID != "" {
			result, err = markdown.InsertAfterSection(source, afterID, newContent)
		} else {
			result, err = markdown.InsertBeforeSection(source, beforeID, newContent)
		}
		if err != nil {
			return err
		}
		if jsonOut {
			return writeMutationJSON(filename, "insert", targetID(afterID, beforeID), source, result)
		}
		return writeAndShow(filename, source, result, depth)
	}
	if !hasTreeSitterSupport(filename) {
		return noStructureError(filename, "insert")
	}
	hasSymbols, err := hasSymbolTree(filename, source, depth)
	if err != nil {
		return err
	}
	if !hasSymbols {
		return noStructureError(filename, "insert")
	}

	var result []byte
	if afterID != "" {
		result, err = srcop.InsertAfter(filename, source, afterID, newContent, depth)
	} else {
		result, err = srcop.InsertBefore(filename, source, beforeID, newContent, depth)
	}
	if err != nil {
		return err
	}
	if jsonOut {
		return writeMutationJSON(filename, "insert", targetID(afterID, beforeID), source, result)
	}

	return writeAndShow(filename, source, result, depth)
}

func runDelete(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	symbolID, _ := cmd.Flags().GetString("symbol-id")
	depth := getDepth(cmd)
	jsonOut, _ := cmd.Flags().GetBool("json")

	if isMarkdown(filename) {
		result, err := markdown.DeleteSection(source, symbolID)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeMutationJSON(filename, "delete", symbolID, source, result)
		}
		return writeAndShow(filename, source, result, depth)
	}
	if !hasTreeSitterSupport(filename) {
		return noStructureError(filename, "delete")
	}
	hasSymbols, err := hasSymbolTree(filename, source, depth)
	if err != nil {
		return err
	}
	if !hasSymbols {
		return noStructureError(filename, "delete")
	}

	result, err := srcop.Delete(filename, source, symbolID, depth)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeMutationJSON(filename, "delete", symbolID, source, result)
	}

	return writeAndShow(filename, source, result, depth)
}

func runComment(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	if isMarkdown(filename) {
		return fmt.Errorf("comment command not supported for markdown files; use replace --symbol-id <id> instead")
	}
	if !hasTreeSitterSupport(filename) {
		return fmt.Errorf("comment requires code symbols in %s; use src edit %s for text edits",
			filename, shellQuote(filename))
	}

	symbolID, _ := cmd.Flags().GetString("symbol-id")
	readOnly, _ := cmd.Flags().GetBool("read")
	depth := getDepth(cmd)
	hasSymbols, err := hasSymbolTree(filename, source, depth)
	if err != nil {
		return err
	}
	if !hasSymbols {
		return fmt.Errorf("comment requires code symbols in %s; use src edit %s for text edits",
			filename, shellQuote(filename))
	}

	jsonOut, _ := cmd.Flags().GetBool("json")

	if readOnly {
		comment, err := srcop.ReadComment(filename, source, symbolID, depth)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(commentJSON{Path: filename, SymbolID: symbolID, Comment: comment})
		}
		fmt.Print(comment)
		return nil
	}

	newComment, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	result, err := srcop.WriteComment(filename, source, symbolID, newComment, depth)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeMutationJSON(filename, "comment", symbolID, source, result)
	}

	return writeAndShow(filename, source, result, depth)
}

// writeAndShow writes the result to disk, prints a colored diff of old→new,
// then prints the updated symbol tree if the file type is supported by tree-sitter.
func writeAndShow(filename string, source, result []byte, depth int) error {
	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}
	if err := diff.Show(os.Stdout, source, result, filename); err != nil {
		return fmt.Errorf("edit applied to %s, but diff display failed: %w", filename, err)
	}
	if isMarkdown(filename) {
		return printMarkdownTree(filename, result)
	}
	// Skip tree display for file types tree-sitter doesn't support.
	// The file was already written successfully — tree display is optional.
	if _, err := treesitter.LangNameFromExt(filename); err != nil {
		return nil
	}
	return printTree(filename, result, depth)
}

func printMarkdownTree(_ string, source []byte) error {
	treeStr, err := markdown.HeadingTree(source)
	if err != nil {
		return err
	}
	fmt.Print(treeStr)
	return nil
}

func printTree(filename string, source []byte, depth int) error {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return err
	}
	fmt.Print(tree.Render(treesitter.SymbolTree(symbols)))
	return nil
}

func runEdit(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	editsJSON, _ := cmd.Flags().GetBool("edits-json")
	if editsJSON {
		return runEditBatch(filename, source)
	}

	beforeFile, err := cmd.Flags().GetString("before-file")
	if err != nil {
		return fmt.Errorf("internal: --before-file flag error: %w", err)
	}
	afterFile, err := cmd.Flags().GetString("after-file")
	if err != nil {
		return fmt.Errorf("internal: --after-file flag error: %w", err)
	}

	if beforeFile != "" || afterFile != "" {
		return runEditWithFiles(cmd, filename, source, beforeFile, afterFile)
	}

	// Default: read stdin with ===BEFORE===/===AFTER=== delimiters.
	stdinContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")

	sectionID, _ := cmd.Flags().GetString("symbol-id")
	if sectionID != "" {
		if jsonOut {
			return runEditScopedJSON(cmd, filename, source, stdinContent, sectionID)
		}
		return runEditScoped(cmd, filename, source, stdinContent, sectionID)
	}

	depth := getDepth(cmd)
	result, err := srcop.Edit(filename, source, stdinContent)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeMutationJSON(filename, "edit", "", source, result.Content)
	}

	printDisclosure(os.Stderr, result)
	return writeAndShow(filename, source, result.Content, depth)
}

// runEditBatch applies the public Pi batch edit contract: stdin carries a JSON
// envelope {"edits":[{"oldText":...,"newText":...}]} and stdout carries one
// machine-readable result. All replacements are validated against the original
// file before any write.
func runEditBatch(filename string, source []byte) error {
	stdinContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	var envelope struct {
		Edits []struct {
			OldText string `json:"oldText"`
			NewText string `json:"newText"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(stdinContent, &envelope); err != nil {
		return fmt.Errorf("invalid --edits-json envelope: %w", err)
	}
	if len(envelope.Edits) == 0 {
		return fmt.Errorf("edits must not be empty")
	}
	edits := make([]srcop.BatchEdit, 0, len(envelope.Edits))
	for _, e := range envelope.Edits {
		edits = append(edits, srcop.BatchEdit{OldText: e.OldText, NewText: e.NewText})
	}
	result, err := srcop.ApplyBatch(filename, source, edits)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, result.Content, 0o644); err != nil {
		return err
	}
	return printJSON(editBatchJSON{
		Path: filename, Diff: result.Diff, Patch: result.Patch,
		FirstChangedLine: result.FirstChangedLine, EditsApplied: len(edits),
	})
}

// runEditScopedJSON applies a single text edit scoped to one symbol/section and
// prints the machine-readable result.
func runEditScopedJSON(cmd *cobra.Command, filename string, source, input []byte, sectionID string) error {
	depth := getDepth(cmd)
	start, end, err := resolveSectionBounds(filename, source, sectionID, depth)
	if err != nil {
		return err
	}
	start = lineStartAt(source, start)
	end = lineEndAfter(source, end)
	slice := source[start:end]
	result, err := srcop.Edit(filename, slice, input)
	if err != nil {
		return err
	}
	final := make([]byte, 0, len(source)-(end-start)+len(result.Content))
	final = append(final, source[:start]...)
	final = append(final, result.Content...)
	final = append(final, source[end:]...)
	return writeMutationJSON(filename, "edit", sectionID, source, final)
}

// runEditWithFiles handles the --before-file/--after-file edit path.
func runEditWithFiles(cmd *cobra.Command, filename string, source []byte, beforeFile, afterFile string) error {
	if beforeFile == "" || afterFile == "" {
		return fmt.Errorf("both --before-file and --after-file are required together")
	}

	beforeContent, err := os.ReadFile(beforeFile)
	if err != nil {
		return fmt.Errorf("read --before-file %q: %w", beforeFile, err)
	}
	afterContent, err := os.ReadFile(afterFile)
	if err != nil {
		return fmt.Errorf("read --after-file %q: %w", afterFile, err)
	}

	depth := getDepth(cmd)
	result, err := srcop.EditDirect(filename, source, beforeContent, afterContent)
	if err != nil {
		return err
	}
	printDisclosure(os.Stderr, result)
	return writeAndShow(filename, source, result.Content, depth)
}

// runEditScoped edits a specific symbol or section identified by sectionID.
// Known limitation: when the scoped slice is small (e.g. a 5-line function in a JS/TS file),
// indent.Detect inside srcop.Edit will run Layer 2 against the slice and likely hit Layer 3
// fallback (not enough lines for 80% majority). Reindent will no-op with a warning in that case.
// This is acceptable for v1 — scoped editing on opinionated-language files (Go, Py, Rust) works
// perfectly because Layer 1 hits on the extension. A follow-up can pre-compute target style
// on the full file before slicing and thread it through the Edit API. NOT in scope for this task.
func runEditScoped(cmd *cobra.Command, filename string, source, input []byte, sectionID string) error {
	depth := getDepth(cmd)

	start, end, err := resolveSectionBounds(filename, source, sectionID, depth)
	if err != nil {
		return err
	}

	// Extend to line boundaries — srcop.Edit's line-splitting assumes the slice
	// starts at a line boundary and ends after a newline. Tree-sitter byte offsets
	// for nested symbols may land mid-line.
	start = lineStartAt(source, start)
	end = lineEndAfter(source, end)

	slice := source[start:end]
	result, err := srcop.Edit(filename, slice, input)
	if err != nil {
		return err
	}

	final := make([]byte, 0, len(source)-(end-start)+len(result.Content))
	final = append(final, source[:start]...)
	final = append(final, result.Content...)
	final = append(final, source[end:]...)

	printDisclosure(os.Stderr, result)
	return writeAndShow(filename, source, final, depth)
}

func resolveSectionBounds(filename string, source []byte, sectionID string, depth int) (int, int, error) {
	if isMarkdown(filename) {
		return markdown.SectionBounds(source, sectionID)
	}
	if !hasTreeSitterSupport(filename) {
		return 0, 0, noStructureError(filename, "scoped edit")
	}
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return 0, 0, err
	}
	nodes := treesitter.SymbolTree(symbols)
	if len(nodes) == 0 {
		return 0, 0, noStructureError(filename, "scoped edit")
	}
	for i, n := range nodes {
		if n.ID == sectionID {
			return int(symbols[i].StartByte), int(symbols[i].EndByte), nil
		}
	}
	// When tree is empty, error message should still suggest --tree.
	return 0, 0, fmt.Errorf("symbol %q not found; run --tree to see current IDs", sectionID)
}

func hasTreeSitterSupport(filename string) bool {
	_, err := treesitter.LangNameFromExt(filename)
	return err == nil
}

func hasSymbolTree(filename string, source []byte, depth int) (bool, error) {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return false, err
	}
	return len(treesitter.SymbolTree(symbols)) > 0, nil
}

func noStructureError(filename, action string) error {
	return fmt.Errorf("%s requires a symbol or section, but %s does not have a symbol tree; use src edit %s",
		action, filename, shellQuote(filename))
}

func noSymbolTreeError(filename string) error {
	return fmt.Errorf("%s does not have a symbol tree; use src edit %s for text edits",
		filename, shellQuote(filename))
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t\n'\"\\$`") {
		return s
	}
	return strconv.Quote(s)
}

// lineStartAt returns the byte offset of the start of the line containing pos.
func lineStartAt(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos > 0 && source[pos-1] != '\n' {
		pos--
	}
	return pos
}

// lineEndAfter returns the byte offset of the newline that ends the line
// containing pos-1. If pos lands on a newline, returns pos+1. If no newline
// is found, returns len(source).
func lineEndAfter(source []byte, pos int) int {
	if pos > len(source) {
		return len(source)
	}
	// If pos lands on a newline, the "line containing pos-1" ends here.
	if pos < len(source) && source[pos] == '\n' {
		return pos + 1
	}
	// Scan forward from pos to find the newline ending this line.
	for pos < len(source) && source[pos] != '\n' {
		pos++
	}
	return pos
}
