package fetch

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// truncateContent truncates content to maxContentChars runes.
const maxContentChars = 30_000

func truncateContent(s string) string {
	if utf8.RuneCountInString(s) <= maxContentChars {
		return s
	}
	return string([]rune(s)[:maxContentChars]) + "\n[content truncated at 30,000 characters]"
}

// CachedFetchBackend wraps a Backend with a file-based daily cache.
// Cache key: sanitized URL + today's date. Cache dir: ~/.cache/organon/scrapes/.
type CachedFetchBackend struct {
	cacheDir string
	fallback Backend
}

// NewCachedFetchBackend creates a CachedFetchBackend wrapping the given fallback.
func NewCachedFetchBackend(cacheDir string, fallback Backend) *CachedFetchBackend {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cachedfetch: failed to create cache dir %q: %v\n", cacheDir, err)
		slog.Warn("cachedfetch: failed to create cache dir, caching disabled", "dir", cacheDir, "error", err)
	}
	return &CachedFetchBackend{cacheDir: cacheDir, fallback: fallback}
}

// Fetch returns cached content if available for today, otherwise delegates to fallback and caches.
func (b *CachedFetchBackend) Fetch(ctx context.Context, rawURL string) (string, error) {
	if cached, ok := b.readCache(rawURL); ok {
		return cached, nil
	}
	content, err := b.fallback.Fetch(ctx, rawURL)
	if err != nil {
		return "", err
	}
	if err := b.writeCache(rawURL, content); err != nil {
		slog.Warn("cachedfetch: failed to write cache", "url", rawURL, "error", err)
	}
	return content, nil
}

func (b *CachedFetchBackend) readCache(rawURL string) (string, bool) {
	path := b.cachePath(rawURL)
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("cachedfetch: unexpected cache read error", "path", path, "error", err)
		}
		return "", false
	}
	return string(data), true
}

func (b *CachedFetchBackend) writeCache(rawURL, content string) error {
	return os.WriteFile(b.cachePath(rawURL), []byte(content), 0o644)
}

func (b *CachedFetchBackend) cachePath(rawURL string) string {
	sanitized := sanitizeURL(rawURL)
	date := time.Now().Format("2006-01-02")
	return filepath.Join(b.cacheDir, sanitized+"__"+date+".md")
}

// sanitizeURL converts a URL into a safe filename segment.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		r := strings.NewReplacer("://", "___", "/", "_", "?", "_", "=", "_", "&", "_", "..", "__")
		return r.Replace(rawURL)
	}

	base := strings.ReplaceAll(rawURL, "://", "___")
	if u.RawQuery != "" {
		withoutQuery := strings.SplitN(base, "?", 2)[0]
		base = withoutQuery
	}
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.TrimSuffix(base, "_")
	base = strings.ReplaceAll(base, "..", "__")

	if u.RawQuery != "" {
		h := sha256.Sum256([]byte(u.RawQuery))
		base += "_q" + fmt.Sprintf("%x", h[:4])
	}
	return base
}
