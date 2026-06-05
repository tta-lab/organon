package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tta-lab/organon/internal/codexusage"
)

func main() {
	if err := run(os.Stdout, &http.Client{Timeout: 30 * time.Second}, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stdout io.Writer, client *http.Client, args []string) error {
	var configPath string
	var baseURL string

	root := &cobra.Command{
		Use:          "codex-usage",
		Short:        "Show Codex weekly usage from Lenos config",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := codexusage.LoadConfig(configPath)
			if err != nil {
				return err
			}
			usage, err := codexusage.FetchWeeklyUsage(context.Background(), client, baseURL, config)
			if err != nil {
				return err
			}
			_, err = io.WriteString(stdout, codexusage.FormatWeeklyUsage(usage))
			return err
		},
	}

	root.Flags().StringVar(&configPath, "config", codexusage.DefaultConfigPath, "Lenos config path")
	root.Flags().StringVar(&baseURL, "base-url", "https://chatgpt.com/backend-api", "Codex usage API base URL")
	root.SetArgs(args)
	return root.Execute()
}
