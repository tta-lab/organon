package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// DotEnvPath returns the ttal-compatible secrets file path.
func DotEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ttal", ".env"), nil
}

// LoadDotEnv reads ~/.config/ttal/.env and returns key-value pairs.
func LoadDotEnv() (map[string]string, error) {
	path, err := DotEnvPath()
	if err != nil {
		return nil, err
	}

	env, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return env, nil
}

// InjectDotEnvFallback sets values from ~/.config/ttal/.env when env is empty.
func InjectDotEnvFallback() error {
	dotEnv, err := LoadDotEnv()
	if err != nil {
		return err
	}
	for k, v := range dotEnv {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}
