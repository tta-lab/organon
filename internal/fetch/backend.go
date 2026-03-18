package fetch

import (
	"context"
	"os"
	"path/filepath"
)

// Backend controls how HTML is fetched and converted to markdown.
type Backend interface {
	Fetch(ctx context.Context, url string) (content string, err error)
}

// Resolve returns the appropriate backend based on environment.
// If BROWSER_GATEWAY_URL is set, uses BrowserGatewayBackend (no cache).
// Otherwise, uses DefuddleCLIBackend wrapped with daily file cache.
func Resolve(httpClient interface{}) Backend {
	gatewayURL := os.Getenv("BROWSER_GATEWAY_URL")
	if gatewayURL != "" {
		return NewBrowserGatewayBackend(gatewayURL, nil)
	}
	defuddle := NewDefuddleCLIBackend()
	return NewCachedFetchBackend(defaultCacheDir(), defuddle)
}

// defaultCacheDir returns the default cache directory for organon scrapes.
func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "organon", "scrapes")
	}
	return filepath.Join(home, ".cache", "organon", "scrapes")
}
