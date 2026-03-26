package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const exaBaseURL = "https://api.exa.ai"

// ExaSearcher searches via the Exa Search API.
type ExaSearcher struct {
	apiKey  string
	baseURL string // defaults to exaBaseURL; tests override
	client  *http.Client
}

// NewExaSearcher creates an Exa searcher with the given API key.
func NewExaSearcher(apiKey string) *ExaSearcher {
	if apiKey == "" {
		slog.Warn("NewExaSearcher called with empty API key — searches will return HTTP 401")
	}
	return &ExaSearcher{
		apiKey:  apiKey,
		baseURL: exaBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *ExaSearcher) Search(ctx context.Context, query string) ([]SearchResult, error) {
	reqBody := exaSearchRequest{
		Query:      query,
		NumResults: 10,
		Contents:   exaContents{Highlights: true},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("exa search: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("exa search: build request: %w", err)
	}
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa search: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("exa search: HTTP %d (body read error: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("exa search: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result exaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("exa search: decode: %w", err)
	}

	results := toExaResults(result)
	if len(results) == 0 {
		slog.Warn("exa_search returned zero results — possible quota exhaustion or no matches")
	}
	return results, nil
}

type exaContents struct {
	Highlights bool `json:"highlights"`
}

type exaSearchRequest struct {
	Query      string      `json:"query"`
	NumResults int         `json:"numResults"`
	Contents   exaContents `json:"contents"`
}

type exaResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate"`
	Author        string   `json:"author"`
	Highlights    []string `json:"highlights"`
}

type exaSearchResponse struct {
	Results []exaResult `json:"results"`
}

func toExaResults(resp exaSearchResponse) []SearchResult {
	results := make([]SearchResult, 0, len(resp.Results))
	for i, r := range resp.Results {
		snippet := firstHighlight(r)
		results = append(results, SearchResult{
			Title:    r.Title,
			Link:     r.URL,
			Snippet:  snippet,
			Position: i + 1,
		})
	}
	return results
}

// firstHighlight returns the first highlight for a result, falling back to
// publishedDate + author when highlights are absent, or empty string with a
// warning if neither is available.
func firstHighlight(r exaResult) string {
	if len(r.Highlights) > 0 {
		return r.Highlights[0]
	}
	if r.PublishedDate != "" || r.Author != "" {
		var parts []string
		if r.PublishedDate != "" {
			parts = append(parts, r.PublishedDate)
		}
		if r.Author != "" {
			parts = append(parts, "by "+r.Author)
		}
		return strings.Join(parts, " ")
	}
	slog.Warn("exa_search result has no highlights or metadata", "url", r.URL)
	return ""
}
