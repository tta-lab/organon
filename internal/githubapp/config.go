package githubapp

import (
	"fmt"
	"strings"
)

// Config is the validated GitHub App section of og.toml.
type Config struct {
	AppID         int64    `toml:"app_id"`
	KeySource     string   `toml:"key_source"`
	KeyRef        string   `toml:"key_ref"`
	AllowedOwners []string `toml:"allowed_owners"`
}

// Validate normalizes and validates a GitHub App configuration section.
func (c *Config) Validate() error {
	if c.AppID <= 0 {
		return fmt.Errorf("github_app.app_id must be a positive integer")
	}
	if c.KeySource == "" {
		return fmt.Errorf("github_app.key_source is required")
	}
	if c.KeySource != "file" {
		return fmt.Errorf("github_app.key_source %q is not supported", c.KeySource)
	}
	if strings.TrimSpace(c.KeyRef) == "" {
		return fmt.Errorf("github_app.key_ref is required")
	}
	if len(c.AllowedOwners) == 0 {
		return fmt.Errorf("github_app.allowed_owners is required")
	}

	seen := make(map[string]struct{}, len(c.AllowedOwners))
	for i, owner := range c.AllowedOwners {
		normalized := strings.ToLower(strings.TrimSpace(owner))
		if normalized == "" {
			return fmt.Errorf("github_app.allowed_owners contains an empty owner")
		}
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("github_app.allowed_owners contains duplicate owner %q", owner)
		}
		seen[normalized] = struct{}{}
		c.AllowedOwners[i] = normalized
	}
	return nil
}

// RequireOwner rejects repositories outside the configured owner allowlist.
func (c Config) RequireOwner(owner string) error {
	normalized := strings.ToLower(owner)
	for _, allowed := range c.AllowedOwners {
		if strings.ToLower(allowed) == normalized {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not in github_app.allowed_owners", ErrOwnerNotAllowed, owner)
}
