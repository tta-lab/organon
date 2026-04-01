package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const tavilyBaseURL = "https://api.tavily.com"

// TavilySearcher searches via the Tavily Search API.
type TavilySearcher struct {
	apiKey  string
	baseURL string // defaults to tavilyBaseURL; tests override
	client  *http.Client
}

// NewTavilySearcher creates a Tavily searcher with the given API key.
func NewTavilySearcher(apiKey string) *TavilySearcher {
	if apiKey == "" {
		slog.Warn("NewTavilySearcher called with empty API key — searches will fail")
	}
	return &TavilySearcher{
		apiKey:  apiKey,
		baseURL: tavilyBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *TavilySearcher) Search(ctx context.Context, query string) ([]SearchResult, error) {
	reqBody := tavilySearchRequest{
		APIKey:     s.apiKey,
		Query:      query,
		MaxResults: 10,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily search: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily search: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("tavily search: HTTP %d (body read error: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("tavily search: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result tavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tavily search: decode: %w", err)
	}

	results := toTavilyResults(result)
	if len(results) == 0 {
		slog.Warn("tavily_search returned zero results — possible quota exhaustion or no matches")
	}
	return results, nil
}

type tavilySearchRequest struct {
	APIKey     string `json:"api_key"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type tavilySearchResponse struct {
	Results []tavilyResult `json:"results"`
}

func toTavilyResults(resp tavilySearchResponse) []SearchResult {
	results := make([]SearchResult, 0, len(resp.Results))
	for i, r := range resp.Results {
		results = append(results, SearchResult{
			Title:    r.Title,
			Link:     r.URL,
			Snippet:  r.Content,
			Position: i + 1,
		})
	}
	return results
}
