package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/token"
)

func main() {
	root := &cobra.Command{
		Use:   "token [text | file path]",
		Short: "Count LLM tokens in text or files",
		Long:  helpRoot,
		Args:  cobra.ExactArgs(1),
		RunE:  run,
	}
	root.Flags().BoolP("file", "f", false, "Force arg to be treated as a file path")
	root.Flags().BoolP("verbose", "v", false, "Show individual token names")
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
func run(cmd *cobra.Command, args []string) error {
	arg := args[0]
	forceFile, _ := cmd.Flags().GetBool("file")
	verbose, _ := cmd.Flags().GetBool("verbose")
	var text string
	if forceFile {
		data, err := os.ReadFile(arg)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		text = string(data)
	} else {
		if info, err := os.Stat(arg); err == nil && !info.IsDir() {
			data, err := os.ReadFile(arg)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			text = string(data)
		} else {
			text = arg
		}
	}
	if verbose {
		tokens := token.Encode(text)
		fmt.Printf("tokenizer: cl100k_base\n")
		fmt.Printf("count: %d\n", len(tokens))
		fmt.Printf("tokens: %s\n", strings.Join(tokens, " "))
	} else {
		count := token.Count(text)
		fmt.Printf("tokenizer: cl100k_base\n")
		fmt.Printf("count: %d\n", count)
	}
	return nil
}
