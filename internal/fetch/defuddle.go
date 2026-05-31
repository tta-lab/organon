package fetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type defuddleCLIBackend struct {
	client *http.Client
}

const downloadLimit = 10 * 1024 * 1024 // 10MB

// NewDefuddleCLIBackend creates a backend that fetches URLs via Go's HTTP client.
// HTML pages are passed to defuddle for parsing; non-HTML content is returned raw.
// Requires defuddle to be installed and on PATH.
func NewDefuddleCLIBackend() Backend {
	return &defuddleCLIBackend{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (b *defuddleCLIBackend) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("defuddle: build request: %w", err)
	}
	req.Header.Set("User-Agent", webFetchAgent)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("defuddle: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("defuddle: HTTP %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, downloadLimit)
	body, err := io.ReadAll(lr)
	if err != nil {
		return "", fmt.Errorf("defuddle: read body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	mediatype, _, err := mime.ParseMediaType(ct)
	if err != nil {
		slog.Warn("defuddle: Content-Type parse failed, trying defuddle anyway",
			"content-type", ct, "error", err)
		mediatype = "text/html"
	}
	if !strings.EqualFold(mediatype, "text/html") {
		return TruncateContent(string(body)), nil
	}

	parsed, err := b.parseHTML(ctx, body)
	if err != nil {
		return "", fmt.Errorf("defuddle parse failed: %w", err)
	}
	return TruncateContent(parsed), nil
}

func (b *defuddleCLIBackend) parseHTML(ctx context.Context, html []byte) (string, error) {
	f, err := os.CreateTemp("", "organon-defuddle-*.html")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.Write(html); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		slog.Warn("defuddle: temp file close error", "path", f.Name(), "error", err)
	}

	cmd := exec.CommandContext(ctx, "defuddle", "parse", f.Name(), "--markdown")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w\noutput: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
