package ogconfig

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

// Config is the validated content of og.toml.
type Config struct {
	GitHubApp *githubapp.Config `toml:"github_app"`
	Forgejo   ForgejoConfig     `toml:"forgejo"`
}

// ForgejoConfig limits which server roots may receive Forgejo credentials.
type ForgejoConfig struct {
	AllowedBaseURLs []string `toml:"allowed_base_urls"`
}

// Load reads and validates the complete og configuration file.
func Load(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("read og config %s: %w", path, err)
	}
	if cfg.GitHubApp != nil {
		if err := cfg.GitHubApp.Validate(); err != nil {
			return Config{}, err
		}
	}
	seen := make(map[string]struct{}, len(cfg.Forgejo.AllowedBaseURLs))
	for i, raw := range cfg.Forgejo.AllowedBaseURLs {
		normalized, err := NormalizeBaseURL(raw)
		if err != nil {
			return Config{}, fmt.Errorf("forgejo.allowed_base_urls[%d]: %w", i, err)
		}
		if _, ok := seen[normalized]; ok {
			return Config{}, fmt.Errorf("forgejo.allowed_base_urls contains duplicate %q", normalized)
		}
		seen[normalized] = struct{}{}
		cfg.Forgejo.AllowedBaseURLs[i] = normalized
	}
	return cfg, nil
}

// ClassifyRemote applies the configured credential boundary to parsed repository metadata.
func (c Config) ClassifyRemote(info *gitprovider.RepoInfo) (gitprovider.ProviderType, error) {
	if info == nil {
		return "", fmt.Errorf("repository remote is required")
	}
	if info.BaseURL == "" {
		return "", fmt.Errorf("remote must use HTTP(S)")
	}
	baseURL, err := NormalizeBaseURL(info.BaseURL)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(info.Host, "github.com") {
		if strings.HasPrefix(baseURL, "https://") {
			return gitprovider.ProviderGitHub, nil
		}
		return "", fmt.Errorf("GitHub HTTP remote base URL %q is not allowed", baseURL)
	}
	for _, allowed := range c.Forgejo.AllowedBaseURLs {
		if baseURL == allowed {
			return gitprovider.ProviderForgejo, nil
		}
	}
	if strings.HasPrefix(baseURL, "https://") {
		return gitprovider.ProviderGeneric, nil
	}
	return "", fmt.Errorf("HTTP remote base URL %q is not allowed", baseURL)
}

// NormalizeBaseURL canonicalizes one HTTP(S) server root for trust comparisons.
func NormalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if err := validateBaseURL(u); err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	hostname := strings.ToLower(u.Hostname())
	port := normalizedPort(scheme, u.Port())
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + host, nil
}

func validateBaseURL(u *url.URL) error {
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL must use HTTP(S)")
	}
	if u.User != nil {
		return fmt.Errorf("URL must not contain userinfo")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL must contain a host")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("URL must not contain a path")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("URL must not contain a query")
	}
	if u.Fragment != "" {
		return fmt.Errorf("URL must not contain a fragment")
	}
	return nil
}

func normalizedPort(scheme, port string) string {
	if scheme == "https" && port == "443" {
		return ""
	}
	if scheme == "http" && port == "80" {
		return ""
	}
	return port
}
