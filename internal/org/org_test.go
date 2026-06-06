package org

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")
	os.WriteFile(p, []byte(`
[tta-lab]
github_token_env = "GITHUB_TOKEN"

[guionai]
github_token_env = "GUION_GITHUB_TOKEN"
`), 0644)

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Name != "guionai" || entries[1].Name != "tta-lab" {
		t.Errorf("expected sorted, got %v", entries)
	}
	if entries[1].GitHubTokenEnv != "GITHUB_TOKEN" {
		t.Errorf("unexpected token env: %s", entries[1].GitHubTokenEnv)
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")
	os.WriteFile(p, []byte(`
[tta-lab]
github_token_env = "GITHUB_TOKEN"
`), 0644)

	e, err := Get(p, "tta-lab")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
	}
	if e.GitHubTokenEnv != "GITHUB_TOKEN" {
		t.Errorf("unexpected token env: %s", e.GitHubTokenEnv)
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0, got %d", len(entries))
	}
}
