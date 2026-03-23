package search

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// WebSearcher performs web searches and returns structured results.
type WebSearcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title    string
	Link     string
	Snippet  string
	Position int
}

// Search performs a web search using the best available backend.
// Backend selection: TAVILY_API_KEY → Tavily, BRAVE_API_KEY → Brave, otherwise → DuckDuckGo Lite.
func Search(ctx context.Context, query string, maxResults int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	searcher, err := resolveSearcher()
	if err != nil {
		return "", err
	}
	results, err := searcher.Search(ctx, query, normalizeMaxResults(maxResults))
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	return formatSearchResults(results), nil
}

// resolveSearcher returns the best available search backend.
// Priority: TAVILY_API_KEY → BRAVE_API_KEY → DuckDuckGo Lite.
// Returns an error if a key env var is set but empty — this prevents silently
// falling back when a user has misconfigured their key.
func resolveSearcher() (WebSearcher, error) {
	tavilyKey, tavilySet := os.LookupEnv("TAVILY_API_KEY")
	if tavilySet && tavilyKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY is set but empty; provide a valid key or unset it to fall back to other backends")
	}
	if tavilyKey != "" {
		return NewTavilySearcher(tavilyKey), nil
	}

	key, set := os.LookupEnv("BRAVE_API_KEY")
	if set && key == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY is set but empty; provide a valid key or unset it to use DuckDuckGo")
	}
	if key != "" {
		return NewBraveSearcher(key), nil
	}
	return NewDDGSearcher(), nil
}

// normalizeMaxResults clamps maxResults to [1, 20].
func normalizeMaxResults(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 20 {
		return 20
	}
	return n
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
