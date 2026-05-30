package main
import (
	"fmt"
	"os"
	"github.com/spf13/cobra"
	"github.com/tta-lab/organon/internal/token"
)
func main() {
	root := &cobra.Command{
		Use:   "token [text | file path]",
		Short: "Count LLM tokens in text or a file",
		Long: `Count the number of LLM tokens in text (positional arg) or the
contents of a file (if the arg is a valid file path).
Uses tiktoken-go with the cl100k_base tokenizer (Claude / GPT-4).
Reports the tokenizer used and the token count.`,
		Args: cobra.ExactArgs(1),
		RunE: run,
	}
	root.Flags().BoolP("file", "f", false, "Force arg to be treated as a file path")
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
func run(cmd *cobra.Command, args []string) error {
	arg := args[0]
	forceFile, _ := cmd.Flags().GetBool("file")
	var text string
	if forceFile {
		data, err := os.ReadFile(arg)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		text = string(data)
	} else {
		// If arg is a readable file, treat it as a file. Otherwise, it's literal text.
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
	count := token.Count(text)
	fmt.Printf("tokenizer: cl100k_base\n")
	fmt.Printf("count: %d\n", count)
	return nil
}
