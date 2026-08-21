package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBraveSearcher_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/web/search", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("X-Subscription-Token"))

		resp := braveSearchResponse{}
		resp.Web.Results = []braveWebResult{
			{Title: "Result 1", URL: "https://example.com/1", Description: "First result"},
			{Title: "Result 2", URL: "https://example.com/2", Description: "Second result"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	searcher := &BraveSearcher{
		apiKey:  "test-api-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	results, err := searcher.Search(context.Background(), "test query")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "Result 1", results[0].Title)
	assert.Equal(t, "https://example.com/1", results[0].Link)
	assert.Equal(t, 1, results[0].Position)
}

type fixedSearcher struct {
	results []SearchResult
	err     error
}

func (s fixedSearcher) Search(context.Context, string) ([]SearchResult, error) {
	return s.results, s.err
}

func TestSearchResultsWithProviderReturnsTypedProviderAndResults(t *testing.T) {
	results := []SearchResult{{Title: "Typed", Link: "https://example.com", Position: 1}}

	got, err := searchResultsWithProvider(context.Background(), "query", "Brave", fixedSearcher{results: results})
	require.NoError(t, err)
	assert.Equal(t, "Brave", got.Provider)
	assert.Equal(t, results, got.Results)
}

func TestSearchResultsWithProviderWrapsProviderError(t *testing.T) {
	_, err := searchResultsWithProvider(
		context.Background(),
		"query",
		"Brave",
		fixedSearcher{err: errors.New("offline")},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "search failed with Brave provider")
}

func TestBraveSearcher_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	searcher := &BraveSearcher{
		apiKey:  "bad-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	_, err := searcher.Search(context.Background(), "test")
	assert.ErrorContains(t, err, "HTTP 401")
}

func TestResolveSearcher_EmptyBraveKeyError(t *testing.T) {
	unsetEnv(t, "EXA_API_KEY")
	t.Setenv("BRAVE_API_KEY", "")
	searcher, err := resolveSearcher()
	require.Error(t, err)
	assert.Nil(t, searcher)
	assert.ErrorContains(t, err, "BRAVE_API_KEY")
}

func TestResolveSearcher_EmptyExaKeyError(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	searcher, err := resolveSearcher()
	if err != nil {
		assert.ErrorContains(t, err, "EXA_API_KEY")
	} else {
		assert.NotNil(t, searcher)
	}
}

func TestResolveSearcher_WithExaKey(t *testing.T) {
	t.Setenv("EXA_API_KEY", "exa-key-123")
	searcher, err := resolveSearcher()
	require.NoError(t, err)
	_, ok := searcher.(*ExaSearcher)
	assert.True(t, ok, "expected ExaSearcher when EXA_API_KEY is set")
}

func TestResolveSearcher_ExaPriorityOverBrave(t *testing.T) {
	t.Setenv("EXA_API_KEY", "exa-key-123")
	t.Setenv("BRAVE_API_KEY", "brave-key-456")
	searcher, err := resolveSearcher()
	require.NoError(t, err)
	_, ok := searcher.(*ExaSearcher)
	assert.True(t, ok, "expected ExaSearcher to take priority over BraveSearcher")
}

func TestResolveSearcher_WithBraveKey(t *testing.T) {
	unsetEnv(t, "EXA_API_KEY")
	t.Setenv("BRAVE_API_KEY", "my-key-123")
	searcher, err := resolveSearcher()
	require.NoError(t, err)
	_, ok := searcher.(*BraveSearcher)
	assert.True(t, ok, "expected BraveSearcher when only BRAVE_API_KEY is set")
}

func TestResolveSearcher_NoKey(t *testing.T) {
	unsetEnv(t, "EXA_API_KEY")
	unsetEnv(t, "BRAVE_API_KEY")
	searcher, err := resolveSearcher()
	require.Error(t, err)
	assert.Nil(t, searcher)
	assert.ErrorContains(t, err, "EXA_API_KEY or BRAVE_API_KEY")
}

func TestResolveConfiguredSearchProviderUsesExactProvider(t *testing.T) {
	t.Setenv("EXA_API_KEY", "exa-key-123")
	t.Setenv("BRAVE_API_KEY", "brave-key-456")

	provider, searcher, err := resolveConfiguredSearchProvider("brave")
	require.NoError(t, err)
	assert.Equal(t, providerBrave, provider)
	assert.IsType(t, &BraveSearcher{}, searcher)
}

func TestResolveConfiguredSearchProviderRejectsRetiredDuckDuckGo(t *testing.T) {
	_, _, err := resolveConfiguredSearchProvider("duckduckgo")
	require.Error(t, err)
	assert.ErrorContains(t, err, `unsupported search provider "duckduckgo"`)
}

func TestResolveConfiguredSearchProviderRequiresSelectedKey(t *testing.T) {
	unsetEnv(t, "EXA_API_KEY")

	_, _, err := resolveConfiguredSearchProvider("exa")
	require.Error(t, err)
	assert.ErrorContains(t, err, "EXA_API_KEY")
}

func TestSearchWithProvider_IncludesProviderNameOnFailure(t *testing.T) {
	_, err := searchWithProvider(context.Background(), "test", "Brave", failingSearcher{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "search failed with Brave provider")
	assert.ErrorContains(t, err, "backend down")
}

func TestSearch_EmptyQueryDoesNotResolveProvider(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	_, err := Search(context.Background(), "")
	require.Error(t, err)
	assert.EqualError(t, err, "query is required")
}

type failingSearcher struct{}

func (failingSearcher) Search(context.Context, string) ([]SearchResult, error) {
	return nil, errors.New("backend down")
}

// unsetEnv removes an env var for the duration of the test, restoring the
// original value (or absence) via t.Cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, wasSet := os.LookupEnv(key)
	os.Unsetenv(key) //nolint:errcheck
	t.Cleanup(func() {
		if wasSet {
			os.Setenv(key, orig) //nolint:errcheck
		} else {
			os.Unsetenv(key) //nolint:errcheck
		}
	})
}

func TestFormatSearchResults_Empty(t *testing.T) {
	out := FormatResults(nil)
	assert.Contains(t, out, "No results found")
}

func TestFormatSearchResults_WithResults(t *testing.T) {
	results := []SearchResult{
		{Title: "Go Blog", Link: "https://go.dev/blog", Snippet: "The Go programming language blog.", Position: 1},
		{Title: "Go Docs", Link: "https://pkg.go.dev", Snippet: "Go package documentation.", Position: 2},
	}
	out := FormatResults(results)
	assert.Contains(t, out, "Found 2 search results")
	assert.Contains(t, out, "Go Blog")
	assert.Contains(t, out, "https://go.dev/blog")
	assert.Contains(t, out, "The Go programming language blog.")
}
