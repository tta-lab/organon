package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFileLoadsMissingEnvValues(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	if err := os.Unsetenv("EXA_API_KEY"); err != nil {
		t.Fatalf("unset EXA_API_KEY: %v", err)
	}

	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("EXA_API_KEY=from-file\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := os.Getenv("EXA_API_KEY"); got != "from-file" {
		t.Fatalf("EXA_API_KEY = %q, want from-file", got)
	}
}

func TestLoadEnvFileDoesNotOverrideExistingEnvValues(t *testing.T) {
	t.Setenv("EXA_API_KEY", "from-process")

	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("EXA_API_KEY=from-file\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("load env file: %v", err)
	}

	if got := os.Getenv("EXA_API_KEY"); got != "from-process" {
		t.Fatalf("EXA_API_KEY = %q, want from-process", got)
	}
}

func TestLoadEnvFileIgnoresMissingFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "missing.env")

	if err := loadEnvFile(envPath); err != nil {
		t.Fatalf("load missing env file: %v", err)
	}
}

func TestLoadEnvFileReportsMalformedFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("not valid dotenv\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	err := loadEnvFile(envPath)
	if err == nil {
		t.Fatal("expected malformed env file error")
	}
	if !strings.Contains(err.Error(), "load") {
		t.Fatalf("error = %q, want load context", err.Error())
	}
}
