package config

import (
	"os"
	"path/filepath"
)

// DefaultConfigDir returns ~/.config/ttal (matching current ttal convention).
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ttal")
}

// ResolveDataDir returns the Organon/ttal-compatible data directory.
func ResolveDataDir() string {
	if dir := os.Getenv("TTAL_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "ttal")
}

// ProjectsPath returns the path to projects.toml.
func ProjectsPath() string {
	return filepath.Join(DefaultConfigDir(), "projects.toml")
}

// OrgsPath returns the path to orgs.toml.
func OrgsPath() string {
	return filepath.Join(DefaultConfigDir(), "orgs.toml")
}

// DefaultReferencesPath returns ~/code/references.
func DefaultReferencesPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "code", "references")
}
