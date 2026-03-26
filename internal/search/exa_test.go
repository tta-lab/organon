package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExaSearcher_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "test-exa-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody exaSearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.Equal(t, "golang generics", reqBody.Query)
		assert.Equal(t, 10, reqBody.NumResults)
		assert.True(t, reqBody.Contents.Highlights)

		resp := exaSearchResponse{
			Results: []exaResult{
				{
					Title:      "Go Generics Guide",
					URL:        "https://go.dev/generics",
					Highlights: []string{"A comprehensive guide to generics in Go."},
				},
				{
					Title:      "Generics Tutorial",
					URL:        "https://example.com/generics",
					Highlights: []string{"Learn how to use generics.", "More details here."},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	searcher := &ExaSearcher{
		apiKey:  "test-exa-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	results, err := searcher.Search(context.Background(), "golang generics")
	require.NoError(t, err)
	assert.Len(t, results, 2)

	assert.Equal(t, "Go Generics Guide", results[0].Title)
	assert.Equal(t, "https://go.dev/generics", results[0].Link)
	assert.Equal(t, "A comprehensive guide to generics in Go.", results[0].Snippet)
	assert.Equal(t, 1, results[0].Position)

	assert.Equal(t, "Generics Tutorial", results[1].Title)
	assert.Equal(t, "Learn how to use generics.", results[1].Snippet, "should use first highlight only")
	assert.Equal(t, 2, results[1].Position)
}

func TestExaSearcher_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	searcher := &ExaSearcher{
		apiKey:  "bad-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	_, err := searcher.Search(context.Background(), "test")
	assert.ErrorContains(t, err, "HTTP 401")
}

func TestExaSearcher_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := exaSearchResponse{Results: []exaResult{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	searcher := &ExaSearcher{
		apiKey:  "test-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	results, err := searcher.Search(context.Background(), "no results query")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExaSearcher_HighlightFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := exaSearchResponse{
			Results: []exaResult{
				{
					Title:         "Page with date",
					URL:           "https://example.com/1",
					PublishedDate: "2024-01-15",
					Author:        "Jane Doe",
					Highlights:    nil,
				},
				{
					Title:      "Page with no metadata",
					URL:        "https://example.com/2",
					Highlights: nil,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	searcher := &ExaSearcher{
		apiKey:  "test-key",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	results, err := searcher.Search(context.Background(), "test")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Contains(t, results[0].Snippet, "2024-01-15")
	assert.Contains(t, results[0].Snippet, "Jane Doe")
	assert.Equal(t, "", results[1].Snippet)
}

func TestFirstHighlight_WithHighlights(t *testing.T) {
	r := exaResult{Highlights: []string{"First highlight", "Second highlight"}}
	assert.Equal(t, "First highlight", firstHighlight(r))
}

func TestFirstHighlight_DateAndAuthorFallback(t *testing.T) {
	r := exaResult{PublishedDate: "2024-03-01", Author: "Alice"}
	snippet := firstHighlight(r)
	assert.Contains(t, snippet, "2024-03-01")
	assert.Contains(t, snippet, "Alice")
}

func TestFirstHighlight_DateOnlyFallback(t *testing.T) {
	r := exaResult{PublishedDate: "2024-03-01"}
	assert.Equal(t, "2024-03-01", firstHighlight(r))
}

func TestFirstHighlight_NoMetadata(t *testing.T) {
	r := exaResult{URL: "https://example.com"}
	assert.Equal(t, "", firstHighlight(r))
}
