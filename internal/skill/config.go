package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config controls additional skill discovery roots beyond the built-in default.
//
// The built-in default is the .agents convention only: ~/.agents/skills.
// Configured entries are appended after it, in the order listed, and are
// therefore lower priority. A leading "~" expands to the user's home
// directory (done in GlobalDiscoveryPaths against the passed home).
type Config struct {
	// Global lists extra skills directories, searched for every invocation
	// (CLI and MCP).
	Global []string `toml:"global"`
}

// LoadConfig reads skills discovery configuration from path. A missing file
// returns an empty Config with no error. Entries are trimmed, blank entries
// dropped, and paths cleaned; "~" is left for GlobalDiscoveryPaths to expand.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read skills config %s: %w", path, err)
	}
	cfg.Global = cleanConfigPaths(cfg.Global)
	return cfg, nil
}

// cleanConfigPaths trims entries, drops blanks, and cleans paths.
func cleanConfigPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, filepath.Clean(p))
	}
	return out
}
