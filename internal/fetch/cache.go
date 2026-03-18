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

// MaxContentChars is the rune limit applied by TruncateContent.
const MaxContentChars = 30_000

// TruncateContent truncates content to MaxContentChars runes.
func TruncateContent(s string) string {
	if utf8.RuneCountInString(s) <= MaxContentChars {
		return s
	}
	return string([]rune(s)[:MaxContentChars]) + fmt.Sprintf("\n[content truncated at %d characters]", MaxContentChars)
}

// CachedFetchBackend wraps a Backend with a file-based daily cache.
// Cache key: sanitized URL + today's date. Cache dir: ~/.cache/organon/scrapes/.
type CachedFetchBackend struct {
	cacheDir string
	fallback Backend
}

// NewCachedFetchBackend creates a CachedFetchBackend wrapping the given fallback.
// If the cache directory cannot be created, returns the fallback directly (caching disabled).
func NewCachedFetchBackend(cacheDir string, fallback Backend) Backend {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cachedfetch: failed to create cache dir %q: %v — caching disabled\n", cacheDir, err)
		slog.Warn("cachedfetch: failed to create cache dir, caching disabled", "dir", cacheDir, "error", err)
		return fallback
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

	// Cap at 200 chars to stay within filesystem filename limits.
	const maxFilenameLen = 200
	if len(base) > maxFilenameLen {
		h := sha256.Sum256([]byte(base))
		base = base[:maxFilenameLen-9] + "_" + fmt.Sprintf("%x", h[:4])
	}

	return base
}
