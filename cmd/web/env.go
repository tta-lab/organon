package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

func loadTTALEnv() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return loadEnvFile(filepath.Join(home, ".config", "ttal", ".env"))
}

func loadEnvFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}
