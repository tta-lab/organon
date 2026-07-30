package search

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config controls web search provider selection.
type Config struct {
	Search SearchConfig `toml:"search"`
}

// SearchConfig controls the search command.
type SearchConfig struct {
	Provider string `toml:"provider"`
}

// LoadConfig reads web search configuration from path. A missing file keeps
// the existing automatic provider selection behavior.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read web config %s: %w", path, err)
	}

	cfg.Search.Provider = strings.ToLower(strings.TrimSpace(cfg.Search.Provider))
	switch cfg.Search.Provider {
	case "", "exa", "brave", "duckduckgo":
		return cfg, nil
	default:
		return Config{}, fmt.Errorf("read web config %s: search.provider must be exa, brave, or duckduckgo", path)
	}
}
