package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/fetch"
	"github.com/tta-lab/organon/internal/markdown"
)

func main() {
	root := &cobra.Command{
		Use:   "url <url>",
		Short: "Fetch and read a web page as markdown",
		Args:  cobra.ExactArgs(1),
		RunE:  runURL,
	}

	root.Flags().Bool("tree", false, "Show heading tree")
	root.Flags().StringP("section", "s", "", "Section ID to read")
	root.Flags().Bool("full", false, "Show full content without truncation check")
	root.Flags().Int("tree-threshold", markdown.DefaultTreeThreshold, "Auto-tree above this char count")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runURL(cmd *cobra.Command, args []string) error {
	targetURL := args[0]
	showTree, _ := cmd.Flags().GetBool("tree")
	section, _ := cmd.Flags().GetString("section")
	full, _ := cmd.Flags().GetBool("full")
	treeThreshold, _ := cmd.Flags().GetInt("tree-threshold")

	backend := fetch.Resolve()
	content, err := backend.Fetch(context.Background(), targetURL)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", targetURL, err)
	}

	result, err := markdown.RenderContent([]byte(content), showTree, section, full, treeThreshold)
	if err != nil {
		return err
	}

	fmt.Print(result.Content)
	return nil
}
