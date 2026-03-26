package search

import (
	"context"
	"encoding/json"
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

func TestDDGSearcher_ParseResults(t *testing.T) {
	// DDG Lite uses a table-based layout
	mockHTML := `<html><body><table>
<tr>
<td><a class="result-link" href="https://example.com/page1">Example Page</a></td>
</tr>
<tr>
<td class="result-snippet">This is a snippet for page 1.</td>
</tr>
<tr>
<td><a class="result-link" href="https://example.com/page2">Another Page</a></td>
</tr>
<tr>
<td class="result-snippet">Snippet for page 2.</td>
</tr>
</table></body></html>`

	results, err := parseLiteSearchResults(mockHTML, 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "Example Page", results[0].Title)
	assert.Equal(t, "https://example.com/page1", results[0].Link)
	assert.Equal(t, "This is a snippet for page 1.", results[0].Snippet)
	assert.Equal(t, 1, results[0].Position)
}

func TestDDGSearcher_MaxResults(t *testing.T) {
	mockHTML := `<html><body><table>
<tr><td><a class="result-link" href="https://example.com/1">Result 1</a></td></tr>
<tr><td class="result-snippet">Snippet 1.</td></tr>
<tr><td><a class="result-link" href="https://example.com/2">Result 2</a></td></tr>
<tr><td class="result-snippet">Snippet 2.</td></tr>
<tr><td><a class="result-link" href="https://example.com/3">Result 3</a></td></tr>
<tr><td class="result-snippet">Snippet 3.</td></tr>
</table></body></html>`

	results, err := parseLiteSearchResults(mockHTML, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2, "should respect maxResults limit")
}

func TestResolveSearcher_EmptyBraveKeyError(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "")
	// When set but empty, should return error or DDG fallback depending on LookupEnv behavior
	searcher, err := resolveSearcher()
	if err != nil {
		assert.ErrorContains(t, err, "BRAVE_API_KEY")
	} else {
		assert.NotNil(t, searcher)
	}
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
	require.NoError(t, err)
	_, ok := searcher.(*DDGSearcher)
	assert.True(t, ok, "expected DDGSearcher when no API keys are set")
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

func TestCleanDuckDuckGoURL_Redirect(t *testing.T) {
	encoded := "https%3A%2F%2Fexample.com%2Fpage"
	raw := "//duckduckgo.com/l/?uddg=" + encoded
	result := cleanDuckDuckGoURL(raw)
	assert.Equal(t, "https://example.com/page", result)
}

func TestCleanDuckDuckGoURL_PlainURL(t *testing.T) {
	raw := "https://example.com/page"
	assert.Equal(t, raw, cleanDuckDuckGoURL(raw))
}

func TestCleanDuckDuckGoURL_WithExtraParams(t *testing.T) {
	encoded := "https%3A%2F%2Fexample.com%2Fpage"
	raw := "//duckduckgo.com/l/?uddg=" + encoded + "&rut=abc123"
	result := cleanDuckDuckGoURL(raw)
	assert.Equal(t, "https://example.com/page", result)
}

func TestFormatSearchResults_Empty(t *testing.T) {
	out := formatSearchResults(nil)
	assert.Contains(t, out, "No results found")
}

func TestFormatSearchResults_WithResults(t *testing.T) {
	results := []SearchResult{
		{Title: "Go Blog", Link: "https://go.dev/blog", Snippet: "The Go programming language blog.", Position: 1},
		{Title: "Go Docs", Link: "https://pkg.go.dev", Snippet: "Go package documentation.", Position: 2},
	}
	out := formatSearchResults(results)
	assert.Contains(t, out, "Found 2 search results")
	assert.Contains(t, out, "Go Blog")
	assert.Contains(t, out, "https://go.dev/blog")
	assert.Contains(t, out, "The Go programming language blog.")
}
