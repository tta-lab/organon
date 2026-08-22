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
	Title    string `json:"title"`
	Link     string `json:"link"`
	Snippet  string `json:"snippet"`
	Position int    `json:"position"`
}

// Response is a structured web-search result with the selected provider.
type Response struct {
	Provider string         `json:"provider"`
	Results  []SearchResult `json:"results"`
}

const (
	providerExa   = "Exa"
	providerBrave = "Brave"
)

// Search performs a web search using the best available backend.
// Backend selection: EXA_API_KEY → Exa, then BRAVE_API_KEY → Brave.
func Search(ctx context.Context, query string) (string, error) {
	return SearchWithConfig(ctx, query, Config{})
}

// SearchWithConfig performs a search using the configured provider. An empty
// provider preserves automatic selection based on available API keys.
func SearchWithConfig(ctx context.Context, query string, cfg Config) (string, error) {
	response, err := SearchResultsWithConfig(ctx, query, cfg)
	if err != nil {
		return "", err
	}
	return FormatResults(response.Results), nil
}

// SearchResultsWithConfig performs a search and returns the selected provider
// together with structured results.
func SearchResultsWithConfig(ctx context.Context, query string, cfg Config) (Response, error) {
	if query == "" {
		return Response{}, fmt.Errorf("query is required")
	}

	provider, searcher, err := resolveConfiguredSearchProvider(cfg.Provider)
	if err != nil {
		return Response{}, err
	}
	return searchResultsWithProvider(ctx, query, provider, searcher)
}

func searchWithProvider(ctx context.Context, query, provider string, searcher WebSearcher) (string, error) {
	response, err := searchResultsWithProvider(ctx, query, provider, searcher)
	if err != nil {
		return "", err
	}
	return FormatResults(response.Results), nil
}

func searchResultsWithProvider(
	ctx context.Context,
	query string,
	provider string,
	searcher WebSearcher,
) (Response, error) {
	if query == "" {
		return Response{}, fmt.Errorf("query is required")
	}

	results, err := searcher.Search(ctx, query)
	if err != nil {
		return Response{}, fmt.Errorf("search failed with %s provider: %w", provider, err)
	}
	return Response{Provider: provider, Results: results}, nil
}

// resolveSearcher returns the best available search backend.
// Priority: EXA_API_KEY → Exa, then BRAVE_API_KEY → Brave.
// Returns an error if a key is set but empty or neither provider is configured.
func resolveSearcher() (WebSearcher, error) {
	_, searcher, err := resolveSearchProvider()
	return searcher, err
}

func resolveSearchProvider() (string, WebSearcher, error) {
	exaKey, exaSet := os.LookupEnv("EXA_API_KEY")
	if exaSet && exaKey == "" {
		return "", nil, fmt.Errorf("EXA_API_KEY is set but empty; provide a valid key or unset it to use Brave")
	}
	if exaKey != "" {
		return providerExa, NewExaSearcher(exaKey), nil
	}

	braveKey, braveSet := os.LookupEnv("BRAVE_API_KEY")
	if braveSet && braveKey == "" {
		return "", nil, fmt.Errorf("BRAVE_API_KEY is set but empty; provide a valid key or unset it")
	}
	if braveKey != "" {
		return providerBrave, NewBraveSearcher(braveKey), nil
	}

	return "", nil, fmt.Errorf("web search requires EXA_API_KEY or BRAVE_API_KEY")
}

func resolveConfiguredSearchProvider(configured string) (string, WebSearcher, error) {
	if err := ValidateProvider(configured); err != nil {
		return "", nil, err
	}
	switch configured {
	case "":
		return resolveSearchProvider()
	case "exa":
		key := os.Getenv("EXA_API_KEY")
		if key == "" {
			return "", nil, fmt.Errorf("EXA_API_KEY is required when --provider exa is selected")
		}
		return providerExa, NewExaSearcher(key), nil
	case "brave":
		key := os.Getenv("BRAVE_API_KEY")
		if key == "" {
			return "", nil, fmt.Errorf("BRAVE_API_KEY is required when --provider brave is selected")
		}
		return providerBrave, NewBraveSearcher(key), nil
	default:
		return "", nil, fmt.Errorf("unsupported search provider %q", configured)
	}
}

// FormatResults renders structured results in the established CLI format.
func FormatResults(results []SearchResult) string {
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
