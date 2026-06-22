package og

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/project"
)

func TestTokenEnvForDoesNotUseGitHubTokenEnvForForgejo(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("FORGEJO_ACCESS_TOKEN", "")
	t.Setenv("ORG_GITHUB_TOKEN", "gh-token")
	t.Setenv("GITEA_TOKEN", "forge-token")

	got := tokenEnvFor(gitprovider.ProviderForgejo, &project.Entry{GitHubTokenEnv: "ORG_GITHUB_TOKEN"})
	if got != "GITEA_TOKEN" {
		t.Fatalf("tokenEnvFor(Forgejo) = %q, want GITEA_TOKEN", got)
	}
}

func TestCleanupMergedBranchSkipsMissingRemoteBranch(t *testing.T) {
	repo := testGitRepoWithMissingRemoteFeature(t)

	if err := cleanupMergedBranch(&repoContext{
		WorkDir:     repo,
		RemoteURL:   "file://" + filepath.Join(repo, "..", "origin.git"),
		DefaultBase: branchMain,
		Branch:      "feature",
	}); err != nil {
		t.Fatalf("cleanupMergedBranch: %v", err)
	}

	current := gitOut(t, repo, "branch", "--show-current")
	if current != branchMain {
		t.Fatalf("current branch = %q, want main", current)
	}
	if err := gitCmd(repo, "rev-parse", "--verify", "feature"); err == nil {
		t.Fatal("feature branch still exists locally")
	}
}

func testGitRepoWithMissingRemoteFeature(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	gitRun(t, "", "init", "--bare", origin)
	gitRun(t, "", "clone", origin, repo)
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "switch", "-c", branchMain)
	gitRun(t, repo, "commit", "--allow-empty", "-m", "initial")
	gitRun(t, repo, "push", "-u", remoteOrigin, branchMain)
	gitRun(t, repo, "switch", "-c", "feature")
	gitRun(t, repo, "commit", "--allow-empty", "-m", "feature")
	gitRun(t, repo, "push", "-u", remoteOrigin, "feature")
	gitRun(t, repo, "push", remoteOrigin, "--delete", "feature")
	gitRun(t, repo, "remote", "set-head", remoteOrigin, branchMain)
	return repo
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitCmd(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	return cmd.Run()
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
