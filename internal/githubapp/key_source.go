package githubapp

import (
	"crypto/rsa"
	"fmt"
)

// KeySource loads the App's private key without exposing its storage details.
type KeySource interface {
	PrivateKey() (*rsa.PrivateKey, error)
}

// NewKeySource constructs the configured private-key source.
func NewKeySource(cfg Config, configDir string) (KeySource, error) {
	if cfg.KeySource != "file" {
		return nil, fmt.Errorf("github_app.key_source %q is not supported", cfg.KeySource)
	}
	return newFileKeySource(configDir, cfg.KeyRef), nil
}
