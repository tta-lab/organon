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
	"github.com/tta-lab/organon/internal/sgraph"
)

func main() {
	if err := loadTTALEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   "web [command]",
		Short: "Search the web and fetch web pages",
		Long:  helpRoot,
	}
	root.SilenceUsage = true

	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Library documentation via Context7",
		Long:  helpDocs,
	}
	docsCmd.AddCommand(newDocsResolveCmd(), newDocsFetchCmd())

	root.AddCommand(
		newSearchCmd(),
		newFetchCmd(),
		docsCmd,
		newSgraphCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <url> [flags]",
		Short: "Fetch and read a web page as markdown",
		Long:  helpFetch,
		Args:  cobra.ExactArgs(1),
		RunE:  runFetch,
	}
	cmd.Flags().String("section-id", "", "Section ID to read")
	cmd.Flags().Bool("tree", false, "Force heading tree view")
	cmd.Flags().Bool("full", false, "Full content, skip auto-tree")
	cmd.Flags().Int("tree-threshold", 5000, "Auto-tree threshold in characters")
	return cmd
}

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the web",
		Long:  helpSearch,
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

func runFetch(cmd *cobra.Command, args []string) error {
	targetURL := args[0]
	showTree, _ := cmd.Flags().GetBool("tree")
	section, _ := cmd.Flags().GetString("section-id")
	full, _ := cmd.Flags().GetBool("full")
	treeThreshold, _ := cmd.Flags().GetInt("tree-threshold")

	backend := fetch.Resolve()
	content, err := backend.Fetch(context.Background(), targetURL)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", targetURL, err)
	}

	// Backend-agnostic binary check: catches binary returned by any backend
	// (defuddle checks Content-Type too, but gateway may not).
	if fetch.IsBinaryBody([]byte(content)) {
		return fetch.BinaryFetchError(targetURL, "")
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
		Long:  helpDocsResolve,
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
		Long:  helpDocsFetch,
		Args:  cobra.RangeArgs(1, 2),
		RunE:  runDocsFetch,
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

func newSgraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sgraph <query>",
		Short: "Search code across public repositories via Sourcegraph",
		Long:  helpSgraph,
		Args:  cobra.ExactArgs(1),
		RunE:  runSgraph,
	}
	cmd.Flags().IntP("count", "c", 10, "Max results to return (10-20, default 10)")
	cmd.Flags().IntP("context", "C", 10, "Lines of context around each match")
	cmd.Flags().IntP("timeout", "t", 0, "Request timeout in seconds (max 120, 0 = no timeout)")
	return cmd
}

func runSgraph(cmd *cobra.Command, args []string) error {
	count, err := cmd.Flags().GetInt("count")
	if err != nil {
		return fmt.Errorf("invalid --count value: %w", err)
	}
	contextWindow, err := cmd.Flags().GetInt("context")
	if err != nil {
		return fmt.Errorf("invalid --context value: %w", err)
	}
	timeout, err := cmd.Flags().GetInt("timeout")
	if err != nil {
		return fmt.Errorf("invalid --timeout value: %w", err)
	}
	out, err := sgraph.Search(context.Background(), args[0], count, contextWindow, timeout)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
