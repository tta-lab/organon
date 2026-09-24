package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

func TestRepoDiffMCPReturnsStructuredLocalWorkingDiff(t *testing.T) { //nolint:gocyclo // End-to-end MCP contract.
	repo := t.TempDir()
	runRepoMCPGit(t, repo, "init", "-b", "main")
	runRepoMCPGit(t, repo, "config", "user.name", "Test User")
	runRepoMCPGit(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runRepoMCPGit(t, repo, "add", ".")
	runRepoMCPGit(t, repo, "commit", "-m", "base")
	runRepoMCPGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runRepoMCPGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runRepoMCPGit(t, repo, "switch", "-c", "feature")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}

	registry := filepath.Join(t.TempDir(), "projects.toml")
	encoded, _ := json.Marshal(repo)
	config := "[ko]\npath = " + string(encoded) + "\nremote = \"https://github.com/example/ko.git\"\n"
	if err := os.WriteFile(registry, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	session := connectDirectMCP(t, og.Executor(&directExecutor{}), project.NewStore(registry))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "repo_diff", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil || result.IsError {
		t.Fatalf("repo_diff = %#v, %v", result, err)
	}
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output repoDiffOutput
	if err := json.Unmarshal(b, &output); err != nil {
		t.Fatal(err)
	}
	if output.Project != "ko" || output.BaseRef != "origin/main" || len(output.Files) != 1 ||
		output.Files[0].Path != "tracked.txt" || len(output.UntrackedPaths) != 1 ||
		output.UntrackedPaths[0] != "new.txt" || output.Patch == "" {
		t.Fatalf("output = %+v", output)
	}
}

func runRepoMCPGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
