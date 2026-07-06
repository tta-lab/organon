package navidrome

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	defaultClient     = "nd-playlist"
	defaultAPIVersion = "1.16.1"
)

// ErrConfig marks configuration and credential setup failures.
var ErrConfig = errors.New("navidrome config error")

// Config contains connection settings for Navidrome's Subsonic API.
type Config struct {
	Server      string `toml:"server"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	PasswordEnv string `toml:"password_env"`
	Client      string `toml:"client"`
	APIVersion  string `toml:"api_version"`
}

// ConfigOptions holds explicit command-line config overrides.
type ConfigOptions struct {
	Path           string
	Server         string
	Username       string
	Password       string
	Client         string
	APIVersion     string
	PromptPassword func() (string, error)
}

// DefaultConfigPath returns ~/.config/nd-playlist/config.toml.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", defaultClient, "config.toml")
}

// LoadConfig resolves config from file, environment, and explicit overrides.
func LoadConfig(opts ConfigOptions) (Config, error) {
	cfg := Config{
		Client:     defaultClient,
		APIVersion: defaultAPIVersion,
	}

	path := opts.Path
	if path == "" {
		path = DefaultConfigPath()
	}
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, &cfg); err != nil {
				return Config{}, fmt.Errorf("read config %s: %w", path, err)
			}
		} else if opts.Path != "" {
			return Config{}, fmt.Errorf("read config %s: %w", path, err)
		}
	}

	applyEnv(&cfg)
	applyOptions(&cfg, opts)
	if cfg.Password == "" && opts.PromptPassword != nil {
		password, err := opts.PromptPassword()
		if err != nil {
			return Config{}, fmt.Errorf("%w: read password: %v", ErrConfig, err)
		}
		cfg.Password = password
	}

	cfg.Server = strings.TrimRight(strings.TrimSpace(cfg.Server), "/")
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.Client = strings.TrimSpace(cfg.Client)
	cfg.APIVersion = strings.TrimSpace(cfg.APIVersion)

	if cfg.Client == "" {
		cfg.Client = defaultClient
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = defaultAPIVersion
	}
	if cfg.Server == "" {
		return Config{}, fmt.Errorf("%w: missing Navidrome server; set --server, NAVIDROME_URL, or config server", ErrConfig)
	}
	if cfg.Username == "" {
		return Config{}, fmt.Errorf(
			"%w: missing Navidrome username; set --username, NAVIDROME_USER, or config username",
			ErrConfig,
		)
	}
	if cfg.Password == "" {
		return Config{}, fmt.Errorf(
			"%w: missing Navidrome password; set --password, NAVIDROME_PASS, config password, or password_env",
			ErrConfig,
		)
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("NAVIDROME_URL"); value != "" {
		cfg.Server = value
	}
	if value := os.Getenv("NAVIDROME_USER"); value != "" {
		cfg.Username = value
	}
	if value := os.Getenv("NAVIDROME_PASS"); value != "" {
		cfg.Password = value
		return
	}
	if cfg.Password == "" && cfg.PasswordEnv != "" {
		cfg.Password = os.Getenv(cfg.PasswordEnv)
	}
}

func applyOptions(cfg *Config, opts ConfigOptions) {
	if opts.Server != "" {
		cfg.Server = opts.Server
	}
	if opts.Username != "" {
		cfg.Username = opts.Username
	}
	if opts.Password != "" {
		cfg.Password = opts.Password
	}
	if opts.Client != "" {
		cfg.Client = opts.Client
	}
	if opts.APIVersion != "" {
		cfg.APIVersion = opts.APIVersion
	}
}
