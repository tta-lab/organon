package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Library represents a Context7 library result.
type Library struct {
	ID            string
	Title         string
	Description   string
	TrustScore    float64
	TotalSnippets int
	Versions      []string
}

// Client is a Context7 API client.
type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewClient creates a Context7 client. The API key may be empty for anonymous access.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: "https://context7.com",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewClientWithBaseURL creates a Context7 client with a custom base URL (for tests).
func NewClientWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Resolve searches Context7 for libraries matching the query.
func (c *Client) Resolve(ctx context.Context, query string) ([]Library, error) {
	url := c.baseURL + "/api/v1/search?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("context7 resolve: build request: %w", err)
	}
	c.addAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("context7 resolve: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, mapHTTPError("resolve", resp)
	}

	var result struct {
		Results []struct {
			ID            string   `json:"id"`
			Title         string   `json:"title"`
			Description   string   `json:"description"`
			TrustScore    float64  `json:"trustScore"`
			TotalSnippets int      `json:"totalSnippets"`
			Versions      []string `json:"versions"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("context7 resolve: decode: %w", err)
	}

	libs := make([]Library, len(result.Results))
	for i, r := range result.Results {
		libs[i] = Library{
			ID:            r.ID,
			Title:         r.Title,
			Description:   r.Description,
			TrustScore:    r.TrustScore,
			TotalSnippets: r.TotalSnippets,
			Versions:      r.Versions,
		}
	}
	return libs, nil
}

// Docs fetches documentation for a Context7 library ID.
func (c *Client) Docs(ctx context.Context, libraryID, topic string, tokens int) (string, error) {
	// Strip exactly one leading "/" from libraryID
	id := strings.TrimPrefix(libraryID, "/")

	base := c.baseURL + "/api/v1/" + id + "?type=txt"
	if topic != "" {
		base += "&topic=" + url.QueryEscape(topic)
	}
	if tokens > 0 {
		base += fmt.Sprintf("&tokens=%d", tokens)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return "", fmt.Errorf("context7 docs: build request: %w", err)
	}
	c.addAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("context7 docs: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", mapHTTPError("docs", resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("context7 docs: read body: %w", err)
	}
	return string(body), nil
}

// addAuth adds Authorization header if apiKey is non-empty.
func (c *Client) addAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// mapHTTPError maps HTTP responses to structured errors.
func mapHTTPError(op string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	bodySnippet := strings.TrimSpace(string(body))

	switch resp.StatusCode {
	case http.StatusAccepted:
		return fmt.Errorf("context7 %s: library still indexing (HTTP 202) — try again in a few minutes", op)
	case http.StatusUnauthorized:
		return fmt.Errorf("context7 %s: invalid CONTEXT7_API_KEY (HTTP 401) — keys start with 'ctx7sk'", op)
	case http.StatusNotFound:
		hint := ""
		if op == "docs" {
			hint = ". Run 'web docs resolve <name>' to find the correct library ID"
		}
		return fmt.Errorf("context7 %s: not found (HTTP 404)%s", op, hint)
	case http.StatusTooManyRequests:
		return fmt.Errorf("context7 %s: rate limited (HTTP 429) — set CONTEXT7_API_KEY for higher limits", op)
	default:
		return fmt.Errorf("context7 %s: HTTP %d: %s", op, resp.StatusCode, bodySnippet)
	}
}
