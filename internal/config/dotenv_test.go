package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInjectDotEnvFallbackLoadsMissingValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Unsetenv("GITHUB_TOKEN"); err != nil {
		t.Fatalf("unset GITHUB_TOKEN: %v", err)
	}

	envPath := filepath.Join(home, ".config", "ttal", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("GITHUB_TOKEN=from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := InjectDotEnvFallback(); err != nil {
		t.Fatalf("InjectDotEnvFallback() error = %v", err)
	}
	if got := os.Getenv("GITHUB_TOKEN"); got != "from-dotenv" {
		t.Fatalf("GITHUB_TOKEN = %q, want from-dotenv", got)
	}
}

func TestInjectDotEnvFallbackDoesNotOverrideEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "from-process")

	envPath := filepath.Join(home, ".config", "ttal", ".env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("GITHUB_TOKEN=from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := InjectDotEnvFallback(); err != nil {
		t.Fatalf("InjectDotEnvFallback() error = %v", err)
	}
	if got := os.Getenv("GITHUB_TOKEN"); got != "from-process" {
		t.Fatalf("GITHUB_TOKEN = %q, want from-process", got)
	}
}

func TestInjectDotEnvFallbackIgnoresMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := InjectDotEnvFallback(); err != nil {
		t.Fatalf("InjectDotEnvFallback() error = %v", err)
	}
}
