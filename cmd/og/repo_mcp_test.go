package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

//nolint:gocyclo // End-to-end MCP contract.
func TestRepoInspectionMCPReturnsTypedStatusHistoryAndComparison(t *testing.T) {
	repo := t.TempDir()
	runRepoMCPGit(t, repo, "init", "-b", "main")
	runRepoMCPGit(t, repo, "config", "user.name", "Test User")
	runRepoMCPGit(t, repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runRepoMCPGit(t, repo, "add", ".")
	runRepoMCPGit(t, repo, "commit", "-m", "base")
	base := repoMCPGitOutput(t, repo, "rev-parse", "HEAD")
	runRepoMCPGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runRepoMCPGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runRepoMCPGit(t, repo, "switch", "-c", "feature")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runRepoMCPGit(t, repo, "add", ".")
	runRepoMCPGit(t, repo, "commit", "-m", "feature")
	head := repoMCPGitOutput(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(t.TempDir(), "projects.toml")
	encoded, _ := json.Marshal(repo)
	config := "[ko]\npath = " + string(encoded) + "\nremote = \"https://github.com/example/ko.git\"\n"
	if err := os.WriteFile(registry, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	session := connectDirectMCP(t, og.Executor(&directExecutor{}), project.NewStore(registry))
	call := func(name string, args map[string]any, target any) {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("%s = %#v, %v", name, result, err)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	var status repoStatusOutput
	call("repo_status", map[string]any{"project": "ko"}, &status)
	if status.Project != "ko" || status.Branch != "feature" || status.HeadCommit != head ||
		len(status.UntrackedPaths) != 1 || status.UntrackedPaths[0] != "untracked.txt" {
		t.Fatalf("status = %+v", status)
	}
	var commits repoCommitsOutput
	call("repo_commits", map[string]any{"project": "ko", "ref": "default", "limit": 1}, &commits)
	if commits.Project != "ko" || commits.Ref != "origin/main" ||
		len(commits.Commits) != 1 || commits.Commits[0].ID != base {
		t.Fatalf("commits = %+v", commits)
	}
	var compare repoCompareOutput
	call("repo_compare", map[string]any{
		"project": "ko", "base_commit": base, "head_commit": head,
	}, &compare)
	if compare.Project != "ko" || compare.BaseCommit != base || compare.HeadCommit != head ||
		len(compare.Files) != 1 || compare.Files[0].Path != "tracked.txt" ||
		compare.Patch == "" {
		t.Fatalf("compare = %+v", compare)
	}
}

func runRepoMCPGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func repoMCPGitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
