package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/search"
)

func main() {
	root := &cobra.Command{
		Use:   "web <query>",
		Short: "Search the web",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			maxResults, _ := cmd.Flags().GetInt("max")
			result, err := search.Search(context.Background(), args[0], maxResults)
			if err != nil {
				return err
			}
			fmt.Print(result)
			return nil
		},
	}

	root.Flags().IntP("max", "n", 10, "Maximum results (max 20)")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
