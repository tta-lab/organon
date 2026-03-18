package fetch

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

const (
	maxBodyBytes  = 1024 * 1024 // 1MB
	webFetchAgent = "organon/1.0"
)

// extractResponse is the JSON response from the browser-gateway.
type extractResponse struct {
	Content     string `json:"content"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	WordCount   int    `json:"wordCount"`
}

type browserGatewayBackend struct {
	gatewayURL string
	client     *http.Client
}

// NewBrowserGatewayBackend creates a backend that fetches via browser-gateway.
func NewBrowserGatewayBackend(gatewayURL string, client *http.Client) Backend {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &browserGatewayBackend{gatewayURL: gatewayURL, client: client}
}

func (b *browserGatewayBackend) Fetch(ctx context.Context, url string) (string, error) {
	body, err := json.Marshal(map[string]string{"url": url})
	if err != nil {
		return "", fmt.Errorf("browser-gateway: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.gatewayURL+"/api/extract", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("browser-gateway: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webFetchAgent)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("browser-gateway: fetch: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		slog.Warn("browser-gateway returned error status", "gateway", b.gatewayURL, "url", url, "status", resp.StatusCode)
		return "", fmt.Errorf("browser-gateway: HTTP %d", resp.StatusCode)
	}

	var extracted extractResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&extracted); err != nil {
		return "", fmt.Errorf("browser-gateway: decode response: %w", err)
	}

	if extracted.Content == "" {
		return "", fmt.Errorf("browser-gateway: empty content for %s", url)
	}

	var sb strings.Builder
	if extracted.Title != "" {
		sb.WriteString("# ")
		sb.WriteString(extracted.Title)
		sb.WriteString("\n\n")
	}
	if extracted.Author != "" {
		sb.WriteString("*By ")
		sb.WriteString(extracted.Author)
		sb.WriteString("*\n\n")
	}
	sb.WriteString(extracted.Content)

	return truncateContent(sb.String()), nil
}
