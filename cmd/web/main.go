package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/search"
	webcore "github.com/tta-lab/organon/internal/web"
)

type webService interface {
	Search(context.Context, string) (webcore.SearchResult, error)
	Fetch(context.Context, webcore.FetchInput) (webcore.FetchResult, error)
	DocsResolve(context.Context, string) (webcore.DocsResolveResult, error)
	DocsFetch(context.Context, webcore.DocsFetchInput) (webcore.DocsFetchResult, error)
	SGraphSearch(context.Context, webcore.SGraphInput) (webcore.SGraphResult, error)
}

type serviceFactory func(searchProvider string) (webService, error)

func defaultServiceFactory(searchProvider string) (webService, error) {
	return webcore.NewService(webcore.Options{SearchProvider: searchProvider})
}

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
		newWebMCPCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newFetchCmd() *cobra.Command {
	return newFetchCmdWithFactory(defaultServiceFactory)
}

func newFetchCmdWithFactory(factory serviceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <url> [flags]",
		Short: "Fetch and read a web page as markdown",
		Long:  helpFetch,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := factory("")
			if err != nil {
				return err
			}
			return runFetch(cmd, args, service)
		},
	}
	cmd.Flags().StringP("section-id", "s", "", "Section ID to read")
	cmd.Flags().Bool("tree", false, "Force heading tree view")
	cmd.Flags().Bool("full", false, "Full content, skip auto-tree")
	cmd.Flags().Int("tree-threshold", 5000, "Auto-tree threshold in characters")
	cmd.Flags().Bool("json", false, "Output the structured result as JSON")
	return cmd
}

func newSearchCmd() *cobra.Command {
	return newSearchCmdWithFactory(defaultServiceFactory)
}

func newSearchCmdWithFactory(factory serviceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the web",
		Long:  helpSearch,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := cmd.Flags().GetString("provider")
			if err != nil {
				return fmt.Errorf("read --provider: %w", err)
			}
			service, err := factory(provider)
			if err != nil {
				return err
			}
			result, err := service.Search(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), search.FormatResults(result.Results))
			return err
		},
	}
	cmd.Flags().Bool("json", false, "Output the structured result as JSON")
	cmd.Flags().String("provider", "", "Search provider: exa, brave, or duckduckgo")
	return cmd
}

func runFetch(cmd *cobra.Command, args []string, service webService) error {
	targetURL := args[0]
	showTree, _ := cmd.Flags().GetBool("tree")
	section, _ := cmd.Flags().GetString("section-id")
	full, _ := cmd.Flags().GetBool("full")
	treeThreshold, _ := cmd.Flags().GetInt("tree-threshold")

	result, err := service.Fetch(cmd.Context(), webcore.FetchInput{
		URL:           targetURL,
		ShowTree:      showTree,
		SectionID:     section,
		Full:          full,
		TreeThreshold: treeThreshold,
	})
	if err != nil {
		return err
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), result.Content)
	return err
}

func newDocsResolveCmd() *cobra.Command {
	return newDocsResolveCmdWithFactory(defaultServiceFactory)
}

func newDocsResolveCmdWithFactory(factory serviceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <query>",
		Short: "Resolve a library name to Context7 IDs",
		Long:  helpDocsResolve,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := factory("")
			if err != nil {
				return err
			}
			return runDocsResolve(cmd, args, service)
		},
	}
	cmd.Flags().Bool("json", false, "Output the structured result as JSON")
	return cmd
}

func runDocsResolve(cmd *cobra.Command, args []string, service webService) error {
	result, err := service.DocsResolve(cmd.Context(), args[0])
	if err != nil {
		return err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), formatLibraries(result.Libraries))
	return err
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
	return newDocsFetchCmdWithFactory(defaultServiceFactory)
}

func newDocsFetchCmdWithFactory(factory serviceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch <library-id> [topic]",
		Short: "Fetch documentation for a resolved Context7 library ID",
		Long:  helpDocsFetch,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := factory("")
			if err != nil {
				return err
			}
			return runDocsFetch(cmd, args, service)
		},
	}
	cmd.Flags().Int("tokens", 0, "Token budget (0 = backend default)")
	cmd.Flags().Bool("json", false, "Output the structured result as JSON")
	return cmd
}

func runDocsFetch(cmd *cobra.Command, args []string, service webService) error {
	topic := ""
	if len(args) == 2 {
		topic = args[1]
	}
	tokens, err := cmd.Flags().GetInt("tokens")
	if err != nil {
		return fmt.Errorf("invalid --tokens value: %w", err)
	}

	result, err := service.DocsFetch(cmd.Context(), webcore.DocsFetchInput{
		LibraryID: args[0],
		Topic:     topic,
		Tokens:    tokens,
	})
	if err != nil {
		return err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), result.Content)
	return err
}

func normalizeLibraryID(id string) string {
	return webcore.NormalizeLibraryID(id)
}

func newSgraphCmd() *cobra.Command {
	return newSgraphCmdWithFactory(defaultServiceFactory)
}

func newSgraphCmdWithFactory(factory serviceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sgraph <query>",
		Short: "Search code across public repositories via Sourcegraph",
		Long:  helpSgraph,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := factory("")
			if err != nil {
				return err
			}
			return runSgraph(cmd, args, service)
		},
	}
	cmd.Flags().IntP("count", "c", 10, "Max results to return (10-20, default 10)")
	cmd.Flags().IntP("context", "C", 10, "Lines of context around each match")
	cmd.Flags().IntP("timeout", "t", 0, "Request timeout in seconds (max 120, 0 = no timeout)")
	cmd.Flags().Bool("json", false, "Output the structured result as JSON")
	return cmd
}

func runSgraph(cmd *cobra.Command, args []string, service webService) error {
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
	result, err := service.SGraphSearch(cmd.Context(), webcore.SGraphInput{
		Query:         args[0],
		Count:         count,
		ContextWindow: contextWindow,
		Timeout:       timeout,
	})
	if err != nil {
		return err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), result.Content)
	return err
}
