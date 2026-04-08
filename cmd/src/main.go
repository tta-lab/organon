package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/diff"
	"github.com/tta-lab/organon/internal/indent"
	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/srcop"
	"github.com/tta-lab/organon/internal/tree"
	"github.com/tta-lab/organon/internal/treesitter"
)

func isMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".md" || ext == ".markdown" || ext == ".mdx"
}

func main() {
	root := &cobra.Command{
		Use:   "src <file> [flags]",
		Short: "Structure-aware source file reading and editing",
		Args:  cobra.ExactArgs(1),
		RunE:  runTreeOrRead,
	}
	root.SilenceUsage = true

	// Persistent flag — inherited by all subcommands
	root.PersistentFlags().Int("depth", 2, "Symbol tree depth (default 2)")

	// Root-only flags
	root.Flags().Bool("tree", false, "Force tree view")
	root.Flags().StringP("symbol", "s", "", "Symbol ID to read")

	replaceCmd := &cobra.Command{
		Use:   "replace <file> -s <id>",
		Short: "Replace a symbol (new content via stdin)",
		Args:  cobra.ExactArgs(1),
		RunE:  runReplace,
	}
	replaceCmd.SilenceUsage = true
	replaceCmd.Flags().StringP("symbol", "s", "", "Symbol ID to replace")
	_ = replaceCmd.MarkFlagRequired("symbol")

	insertCmd := &cobra.Command{
		Use:   "insert <file>",
		Short: "Insert content before/after a symbol (stdin)",
		Args:  cobra.ExactArgs(1),
		RunE:  runInsert,
	}
	insertCmd.SilenceUsage = true
	insertCmd.Flags().String("after", "", "Insert after symbol ID")
	insertCmd.Flags().String("before", "", "Insert before symbol ID")

	deleteCmd := &cobra.Command{
		Use:   "delete <file> -s <id>",
		Short: "Delete a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}
	deleteCmd.SilenceUsage = true
	deleteCmd.Flags().StringP("symbol", "s", "", "Symbol ID to delete")
	_ = deleteCmd.MarkFlagRequired("symbol")

	commentCmd := &cobra.Command{
		Use:   "comment <file> -s <id>",
		Short: "Add/replace doc comment on a symbol (stdin)",
		Args:  cobra.ExactArgs(1),
		RunE:  runComment,
	}
	commentCmd.SilenceUsage = true
	commentCmd.Flags().StringP("symbol", "s", "", "Symbol ID")
	commentCmd.Flags().Bool("read", false, "Read existing doc comment instead of writing")
	_ = commentCmd.MarkFlagRequired("symbol")

	editCmd := &cobra.Command{
		Use:   "edit <file>",
		Short: "Replace text using exact match (stdin: ===BEFORE===/===AFTER===)",
		Args:  cobra.ExactArgs(1),
		RunE:  runEdit,
	}
	editCmd.SilenceUsage = true
	editCmd.Flags().StringP("section", "s", "",
		"Scope edit to a symbol/section ID (use `src <file> --tree` to find IDs)")

	root.AddCommand(replaceCmd, insertCmd, deleteCmd, commentCmd, editCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
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

	if isMarkdown(filename) {
		return runMarkdownTreeOrRead(cmd, filename, source)
	}

	depth := getDepth(cmd)
	symbolID, _ := cmd.Flags().GetString("symbol")

	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return err
	}
	nodes := treesitter.SymbolTree(symbols)

	if symbolID != "" {
		for i, n := range nodes {
			if n.ID == symbolID {
				sym := symbols[i]
				start := sym.StartByte
				if sym.DocStart >= 0 {
					start = uint(sym.DocStart)
				}
				fmt.Print(string(source[start:sym.EndByte]))
				return nil
			}
		}
		return fmt.Errorf("symbol %q not found; run --tree to see current IDs", symbolID)
	}

	fmt.Print(tree.Render(nodes))
	return nil
}

func runReplace(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	symbolID, _ := cmd.Flags().GetString("symbol")
	depth := getDepth(cmd)

	newContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	if isMarkdown(filename) {
		result, err := markdown.ReplaceSection(source, symbolID, newContent)
		if err != nil {
			return err
		}
		return writeAndShow(filename, source, result, depth)
	}

	result, err := srcop.Replace(filename, source, symbolID, newContent, depth)
	if err != nil {
		return err
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
		return writeAndShow(filename, source, result, depth)
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

	return writeAndShow(filename, source, result, depth)
}

func runDelete(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	symbolID, _ := cmd.Flags().GetString("symbol")
	depth := getDepth(cmd)

	if isMarkdown(filename) {
		result, err := markdown.DeleteSection(source, symbolID)
		if err != nil {
			return err
		}
		return writeAndShow(filename, source, result, depth)
	}

	result, err := srcop.Delete(filename, source, symbolID, depth)
	if err != nil {
		return err
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
		return fmt.Errorf("comment command not supported for markdown files; use replace -s <id> instead")
	}

	symbolID, _ := cmd.Flags().GetString("symbol")
	readOnly, _ := cmd.Flags().GetBool("read")
	depth := getDepth(cmd)

	if readOnly {
		comment, err := srcop.ReadComment(filename, source, symbolID, depth)
		if err != nil {
			return err
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

	return writeAndShow(filename, source, result, depth)
}

// writeAndShow writes the result to disk, prints a colored diff of old→new,
// then prints the updated symbol tree.
func writeAndShow(filename string, source, result []byte, depth int) error {
	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}
	if err := diff.Show(os.Stdout, source, result, filename); err != nil {
		return err
	}
	if isMarkdown(filename) {
		return printMarkdownTree(filename, result)
	}
	return printTree(filename, result, depth)
}

// runMarkdownTreeOrRead handles the root command for .md files.
// --tree and --depth flags are no-ops for markdown: the heading tree is always shown
// (unless -s is given), since markdown structure is heading-based, not depth-bounded.
func runMarkdownTreeOrRead(cmd *cobra.Command, filename string, source []byte) error {
	symbolID, _ := cmd.Flags().GetString("symbol")
	if symbolID != "" {
		content, err := markdown.ReadSection(source, symbolID)
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	}
	return printMarkdownTree(filename, source)
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

	stdinContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	sectionID, _ := cmd.Flags().GetString("section")
	if sectionID != "" {
		return runEditScoped(cmd, filename, source, stdinContent, sectionID)
	}

	depth := getDepth(cmd)

	result, err := srcop.Edit(filename, source, stdinContent)
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
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return 0, 0, err
	}
	nodes := treesitter.SymbolTree(symbols)
	for i, n := range nodes {
		if n.ID == sectionID {
			return int(symbols[i].StartByte), int(symbols[i].EndByte), nil
		}
	}
	// When tree is empty, error message should still suggest --tree.
	return 0, 0, fmt.Errorf("symbol %q not found; run --tree to see current IDs", sectionID)
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

// lineEndAfter returns the byte offset just past the newline that ends the line
// containing pos-1. If no newline, returns len(source).
func lineEndAfter(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos < len(source) && source[pos-1] != '\n' {
		pos++
	}
	return pos
}
