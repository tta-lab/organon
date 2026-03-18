package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserGatewayBackend_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/extract", r.URL.Path)
		resp := extractResponse{
			Title:   "Test Page",
			Content: "# Hello\n\nThis is test content.",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	backend := NewBrowserGatewayBackend(srv.URL, nil)
	content, err := backend.Fetch(context.Background(), "https://example.com")
	require.NoError(t, err)
	assert.Contains(t, content, "Test Page")
	assert.Contains(t, content, "Hello")
}

func TestBrowserGatewayBackend_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	backend := NewBrowserGatewayBackend(srv.URL, nil)
	_, err := backend.Fetch(context.Background(), "https://example.com")
	assert.ErrorContains(t, err, "HTTP 404")
}

func TestCachedFetchBackend_CacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	fetchCount := 0
	stub := &stubBackend{content: "fetched content", onFetch: func() { fetchCount++ }}
	backend := NewCachedFetchBackend(cacheDir, stub)

	// First fetch — should call backend
	content1, err := backend.Fetch(context.Background(), "https://example.com/page")
	require.NoError(t, err)
	assert.Equal(t, "fetched content", content1)
	assert.Equal(t, 1, fetchCount)

	// Second fetch — should hit cache
	content2, err := backend.Fetch(context.Background(), "https://example.com/page")
	require.NoError(t, err)
	assert.Equal(t, "fetched content", content2)
	assert.Equal(t, 1, fetchCount, "second fetch should use cache")
}

func TestCachedFetchBackend_DateBasedTTL(t *testing.T) {
	cacheDir := t.TempDir()
	stub := &stubBackend{content: "fresh content"}
	backend := NewCachedFetchBackend(cacheDir, stub)

	rawURL := "https://example.com/page"
	sanitized := sanitizeURL(rawURL)

	// Write a cache file with yesterday's date
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	staleFile := filepath.Join(cacheDir, sanitized+"__"+yesterday+".md")
	require.NoError(t, os.WriteFile(staleFile, []byte("stale content"), 0o644))

	// Fetch should bypass the stale cache and call backend
	content, err := backend.Fetch(context.Background(), rawURL)
	require.NoError(t, err)
	assert.Equal(t, "fresh content", content)
}

func TestResolve_DefaultsToDefuddle(t *testing.T) {
	t.Setenv("BROWSER_GATEWAY_URL", "")
	backend := Resolve()
	// Should be a CachedFetchBackend wrapping defuddle
	_, ok := backend.(*CachedFetchBackend)
	assert.True(t, ok, "expected CachedFetchBackend without BROWSER_GATEWAY_URL")
}

func TestResolve_UsesGatewayWhenSet(t *testing.T) {
	t.Setenv("BROWSER_GATEWAY_URL", "http://gateway.example.com")
	backend := Resolve()
	// Should be a browserGatewayBackend
	_, ok := backend.(*browserGatewayBackend)
	assert.True(t, ok, "expected browserGatewayBackend when BROWSER_GATEWAY_URL is set")
}

func TestSanitizeURL_PlainURL(t *testing.T) {
	result := sanitizeURL("https://example.com/path/to/page")
	assert.NotContains(t, result, "://")
	assert.NotContains(t, result, "/")
	assert.NotContains(t, result, "?")
}

func TestSanitizeURL_QueryString(t *testing.T) {
	result1 := sanitizeURL("https://example.com/search?q=foo&page=2")
	result2 := sanitizeURL("https://example.com/search?q=bar&page=3")
	assert.NotContains(t, result1, "?")
	assert.NotContains(t, result1, "=")
	// Different queries produce different filenames
	assert.NotEqual(t, result1, result2)
	// Same query is deterministic
	assert.Equal(t, result1, sanitizeURL("https://example.com/search?q=foo&page=2"))
}

func TestSanitizeURL_LongURL(t *testing.T) {
	// URLs longer than 200 chars should be capped
	longURL := "https://example.com/" + strings.Repeat("a", 250)
	result := sanitizeURL(longURL)
	assert.LessOrEqual(t, len(result), 200)
}

type stubBackend struct {
	content string
	err     error
	onFetch func()
}

func (s *stubBackend) Fetch(_ context.Context, _ string) (string, error) {
	if s.onFetch != nil {
		s.onFetch()
	}
	return s.content, s.err
}
