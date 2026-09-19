package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runProjectCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newRootCmdWithExecutor(&stdout, &stderr, nil, nil)
	cmd.SetArgs(append([]string{"project"}, args...))
	err := cmd.Execute()
	return stdout.String(), err
}

func writeProjectCatalog(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "projects.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOGProjectPreservesDiscoveryAndNavigationContracts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeProjectCatalog(t, home, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"

[archived.old]
path = "/projects/old"
remote = "https://example.com/owner/old.git"
`)
	reference := filepath.Join(home, "code", "references", "github.com", "tta-lab", "reference-only")
	if err := os.MkdirAll(reference, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, err := runProjectCommand(t, "list", "--json")
	if err != nil || strings.Contains(stdout, "old") {
		t.Fatalf("active list = %q, err = %v", stdout, err)
	}
	stdout, err = runProjectCommand(t, "list", "--include-archived", "--json")
	if err != nil || !strings.Contains(stdout, `"alias":"old"`) {
		t.Fatalf("archive list = %q, err = %v", stdout, err)
	}

	stdout, err = runProjectCommand(t, "find", "reference-only", "--json")
	if err != nil || !strings.Contains(stdout, `"reference":true`) {
		t.Fatalf("reference find = %q, err = %v", stdout, err)
	}
	stdout, err = runProjectCommand(t, "get", "FLICK-BACKEND", "--json")
	if err != nil || !strings.Contains(stdout, `"alias":"fb"`) {
		t.Fatalf("canonical get = %q, err = %v", stdout, err)
	}
	stdout, err = runProjectCommand(t, "resolve", "/projects/flick-backend")
	if err != nil {
		t.Fatalf("absolute resolve: %v", err)
	}
	var resolved map[string]any
	if err := json.Unmarshal([]byte(stdout), &resolved); err != nil || resolved["alias"] != "fb" {
		t.Fatalf("resolve = %q, err = %v", stdout, err)
	}
	stdout, err = runProjectCommand(t, "jump", "tta-lab/reference-only")
	if err != nil || stdout != reference+"\n" {
		t.Fatalf("jump = %q, err = %v", stdout, err)
	}
}
