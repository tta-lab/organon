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
	mediatype, _, _ := mime.ParseMediaType(ct)

	if isBinaryContentType(mediatype) || isBinaryBody(body) {
		return "", binaryFetchError(url, ct)
	}

	if mediatype == "" {
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

// isBinaryContentType returns true for content types that are never human-readable text.
func isBinaryContentType(mediatype string) bool {
	// Known binary types
	binaryTypes := []string{
		"application/octet-stream",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-tar",
		"application/pdf",
		"application/msword",
		"application/vnd.ms-",
		"application/vnd.openxmlformats",
		"image/",
		"audio/",
		"video/",
		"font/",
		"application/x-msdownload",
		"application/x-executable",
		"application/x-mach-binary",
	}
	for _, bt := range binaryTypes {
		if strings.HasPrefix(mediatype, bt) {
			return true
		}
	}
	return false
}

// isBinaryBody checks the first 8KB for null bytes.
// Null bytes are a reliable signal of binary content.
func isBinaryBody(data []byte) bool {
	max := 8192
	if len(data) < max {
		max = len(data)
	}
	for _, b := range data[:max] {
		if b == 0 {
			return true
		}
	}
	return false
}

// binaryFetchError returns an error telling the agent to use curl/wget/aria2c instead.
// aria2c is preferred, then wget, then curl.
func binaryFetchError(url, contentType string) error {
	ct := contentType
	if ct == "" {
		ct = "(none)"
	}
	escapedURL := url
	return fmt.Errorf(
		"binary content at %s (Content-Type: %s)\n\n"+
			"web fetch only handles text. Use a download tool:\n"+
			"  # Preferred:\n"+
			"  aria2c -x4 %s\n"+
			"  # Or:\n"+
			"  wget %s\n"+
			"  # Or:\n"+
			"  curl -L -O %s",
		url, ct, escapedURL, escapedURL, escapedURL)
}
