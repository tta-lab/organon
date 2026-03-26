package search

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// WebSearcher performs web searches and returns structured results.
type WebSearcher interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title    string
	Link     string
	Snippet  string
	Position int
}

// Search performs a web search using the best available backend.
// Backend selection: EXA_API_KEY → Exa, BRAVE_API_KEY → Brave, otherwise → DuckDuckGo Lite.
func Search(ctx context.Context, query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	searcher, err := resolveSearcher()
	if err != nil {
		return "", err
	}
	results, err := searcher.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	return formatSearchResults(results), nil
}

// resolveSearcher returns the best available search backend.
// Priority: EXA_API_KEY → Exa, BRAVE_API_KEY → Brave, fallback → DDG.
// Returns an error if a key is set but empty — this prevents silently
// falling back when a user has misconfigured their API key.
func resolveSearcher() (WebSearcher, error) {
	exaKey, exaSet := os.LookupEnv("EXA_API_KEY")
	if exaSet && exaKey == "" {
		return nil, fmt.Errorf("EXA_API_KEY is set but empty; provide a valid key or unset it to use Brave/DuckDuckGo")
	}
	if exaKey != "" {
		return NewExaSearcher(exaKey), nil
	}

	braveKey, braveSet := os.LookupEnv("BRAVE_API_KEY")
	if braveSet && braveKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY is set but empty; provide a valid key or unset it to use DuckDuckGo")
	}
	if braveKey != "" {
		return NewBraveSearcher(braveKey), nil
	}

	return NewDDGSearcher(), nil
}

func formatSearchResults(results []SearchResult) string {
	if len(results) == 0 {
		return "No results found. Try rephrasing your search.\n"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d search results:\n\n", len(results))
	for _, result := range results {
		fmt.Fprintf(&sb, "%d. %s\n", result.Position, result.Title)
		fmt.Fprintf(&sb, "   URL: %s\n", result.Link)
		fmt.Fprintf(&sb, "   Summary: %s\n\n", result.Snippet)
	}
	return sb.String()
}
