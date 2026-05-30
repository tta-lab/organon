package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tiktoken-go/tokenizer"

	"github.com/tta-lab/organon/internal/token"
)

var validEncodings = map[string]tokenizer.Encoding{
	"gpt2":        token.GPT2,
	"r50k_base":   token.R50kBase,
	"p50k_base":   token.P50kBase,
	"p50k_edit":   token.P50kEdit,
	"cl100k_base": token.Cl100kBase,
	"o200k_base":  token.O200kBase,
}

func main() {
	root := &cobra.Command{
		Use:   "token [text | file path]",
		Short: "Count LLM tokens in text or a file",
		Long: `Count the number of LLM tokens in text (positional arg) or the
contents of a file (if the arg is a valid file path).
Uses tiktoken-go. Default tokenizer is cl100k_base (Claude / GPT-4).
Use -t to select another tokenizer:
  gpt2, r50k_base, p50k_base, p50k_edit, cl100k_base, o200k_base
Use -v to show individual token names.`,
		Args: cobra.ExactArgs(1),
		RunE: run,
	}
	root.Flags().BoolP("file", "f", false, "Force arg to be treated as a file path")
	root.Flags().BoolP("verbose", "v", false, "Show individual token names")
	root.Flags().StringP("tokenizer", "t", "cl100k_base", "Tokenizer encoding")
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
func run(cmd *cobra.Command, args []string) error {
	arg := args[0]
	forceFile, _ := cmd.Flags().GetBool("file")
	verbose, _ := cmd.Flags().GetBool("verbose")
	encName, _ := cmd.Flags().GetString("tokenizer")
	enc, ok := validEncodings[encName]
	if !ok {
		return fmt.Errorf("unknown tokenizer: %s (valid: gpt2, r50k_base, p50k_base, p50k_edit, cl100k_base, o200k_base)", encName)
	}
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
		tokens := token.Encode(enc, text)
		fmt.Printf("tokenizer: %s\n", encName)
		fmt.Printf("count: %d\n", len(tokens))
		fmt.Printf("tokens: %s\n", strings.Join(tokens, " | "))
	} else {
		count := token.Count(enc, text)
		fmt.Printf("tokenizer: %s\n", encName)
		fmt.Printf("count: %d\n", count)
	}
	return nil
}
