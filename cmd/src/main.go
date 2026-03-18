package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/srcop"
	"github.com/tta-lab/organon/internal/tree"
	"github.com/tta-lab/organon/internal/treesitter"
)

func main() {
	root := &cobra.Command{
		Use:   "src <file> [flags]",
		Short: "Structure-aware source file reading and editing",
		Args:  cobra.ExactArgs(1),
		RunE:  runTreeOrRead,
	}

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
	replaceCmd.Flags().StringP("symbol", "s", "", "Symbol ID to replace")
	_ = replaceCmd.MarkFlagRequired("symbol")

	insertCmd := &cobra.Command{
		Use:   "insert <file>",
		Short: "Insert content before/after a symbol (stdin)",
		Args:  cobra.ExactArgs(1),
		RunE:  runInsert,
	}
	insertCmd.Flags().String("after", "", "Insert after symbol ID")
	insertCmd.Flags().String("before", "", "Insert before symbol ID")

	deleteCmd := &cobra.Command{
		Use:   "delete <file> -s <id>",
		Short: "Delete a symbol",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}
	deleteCmd.Flags().StringP("symbol", "s", "", "Symbol ID to delete")
	_ = deleteCmd.MarkFlagRequired("symbol")

	commentCmd := &cobra.Command{
		Use:   "comment <file> -s <id>",
		Short: "Add/replace doc comment on a symbol (stdin)",
		Args:  cobra.ExactArgs(1),
		RunE:  runComment,
	}
	commentCmd.Flags().StringP("symbol", "s", "", "Symbol ID")
	commentCmd.Flags().Bool("read", false, "Read existing doc comment instead of writing")
	_ = commentCmd.MarkFlagRequired("symbol")

	root.AddCommand(replaceCmd, insertCmd, deleteCmd, commentCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// getDepth reads the --depth persistent flag from the root command.
// IMPORTANT: Must use cmd.Root().PersistentFlags() — NOT cmd.Flags() —
// because cmd.Flags() on subcommands does NOT include inherited persistent flags.
func getDepth(cmd *cobra.Command) int {
	depth, _ := cmd.Root().PersistentFlags().GetInt("depth")
	return depth
}

func runTreeOrRead(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
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

	result, err := srcop.Replace(filename, source, symbolID, newContent, depth)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}

	return printTree(filename, result, depth)
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

	var result []byte
	if afterID != "" {
		result, err = srcop.InsertAfter(filename, source, afterID, newContent, depth)
	} else {
		result, err = srcop.InsertBefore(filename, source, beforeID, newContent, depth)
	}
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}

	return printTree(filename, result, depth)
}

func runDelete(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	symbolID, _ := cmd.Flags().GetString("symbol")
	depth := getDepth(cmd)

	result, err := srcop.Delete(filename, source, symbolID, depth)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}

	return printTree(filename, result, depth)
}

func runComment(cmd *cobra.Command, args []string) error {
	filename := args[0]
	source, err := os.ReadFile(filename)
	if err != nil {
		return err
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

	if err := os.WriteFile(filename, result, 0o644); err != nil {
		return err
	}

	return printTree(filename, result, depth)
}

func printTree(filename string, source []byte, depth int) error {
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	if err != nil {
		return err
	}
	fmt.Print(tree.Render(treesitter.SymbolTree(symbols)))
	return nil
}
