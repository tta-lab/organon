package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/fetch"
	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/search"
)

func main() {
	root := &cobra.Command{
		Use:   "web",
		Short: "Web search and page fetching for AI agents",
	}

	root.AddCommand(newSearchCmd())
	root.AddCommand(newFetchCmd())

	docsCmd := &cobra.Command{Use: "docs", Short: "Library documentation via Context7"}
	docsCmd.AddCommand(newDocsResolveCmd())
	docsCmd.AddCommand(newDocsFetchCmd())
	root.AddCommand(docsCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the web",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := search.Search(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Print(result)
			return nil
		},
	}
}

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <url>",
		Short: "Fetch and read a web page as markdown",
		Args:  cobra.ExactArgs(1),
		RunE:  runFetch,
	}

	cmd.Flags().Bool("tree", false, "Show heading tree")
	cmd.Flags().StringP("section", "s", "", "Section ID to read")
	cmd.Flags().Bool("full", false, "Show full content without truncation check")
	cmd.Flags().Int("tree-threshold", markdown.DefaultTreeThreshold, "Auto-tree above this char count")

	return cmd
}

func runFetch(cmd *cobra.Command, args []string) error {
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

// newDocsClient builds a Context7 client. CONTEXT7_API_KEY may be unset
// (anonymous, lower limits) or set to a non-empty value. Set-but-empty
// is rejected to surface misconfiguration early.
func newDocsClient() (*docs.Client, error) {
	key, set := os.LookupEnv("CONTEXT7_API_KEY")
	if set && strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("CONTEXT7_API_KEY is set but empty; provide a key or unset it")
	}
	return docs.NewClient(key), nil
}

func newDocsResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <query>",
		Short: "Resolve a library name to Context7 IDs",
		Long:  "Lists Context7 library candidates for <query>. Pick an ID and pass it to 'web docs fetch'.",
		Args:  cobra.ExactArgs(1),
		RunE:  runDocsResolve,
	}
}

func runDocsResolve(cmd *cobra.Command, args []string) error {
	client, err := newDocsClient()
	if err != nil {
		return err
	}
	libs, err := client.Resolve(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	if len(libs) == 0 {
		return fmt.Errorf("no libraries found for %q", args[0])
	}
	fmt.Print(formatLibraries(libs))
	return nil
}

func formatLibraries(libs []docs.Library) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d libraries:\n\n", len(libs))
	for i, lib := range libs {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, lib.Title)
		fmt.Fprintf(&sb, "   ID: %s\n", lib.ID)
		fmt.Fprintf(&sb, "   Trust: %.1f   Snippets: %d\n", lib.TrustScore, lib.TotalSnippets)
		if len(lib.Versions) > 0 {
			fmt.Fprintf(&sb, "   Versions: %s\n", strings.Join(lib.Versions, ", "))
		}
		fmt.Fprintf(&sb, "   %s\n\n", lib.Description)
	}
	return sb.String()
}

func newDocsFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <library-id> [topic]",
		Short: "Fetch documentation for a resolved Context7 library ID",
		Long: `Fetches Context7 docs for <library-id> (from 'web docs resolve').
<library-id> may be passed with or without the leading slash.
[topic] is freeform natural language ("hooks", "how to handle errors").

To pin a version, pass the version-suffixed ID returned by 'web docs resolve'
(e.g. /reactjs/react.dev/18.2.0).`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runDocsFetch,
	}
	cmd.Flags().Int("tokens", 0, "Token budget (0 = backend default)")
	return cmd
}

func runDocsFetch(cmd *cobra.Command, args []string) error {
	id := normalizeLibraryID(args[0])
	topic := ""
	if len(args) == 2 {
		topic = args[1]
	}
	tokens, err := cmd.Flags().GetInt("tokens")
	if err != nil {
		return fmt.Errorf("invalid --tokens value: %w", err)
	}

	client, err := newDocsClient()
	if err != nil {
		return err
	}
	out, err := client.Docs(cmd.Context(), id, topic, tokens)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func normalizeLibraryID(id string) string {
	if id == "" || strings.HasPrefix(id, "/") {
		return id
	}
	return "/" + id
}
