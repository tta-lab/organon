package og

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkingDiffIncludesTrackedStatesAndReportsUntrackedPaths(t *testing.T) {
	repo := workingDiffRepo(t)
	writeWorkingDiffFile(t, repo, "committed.txt", "base\ncommitted\n")
	writeWorkingDiffFile(t, repo, "added.txt", "added\n")
	runWorkingDiffGit(t, repo, "add", "committed.txt", "added.txt")
	runWorkingDiffGit(t, repo, "commit", "-m", "feature change")
	writeWorkingDiffFile(t, repo, "staged.txt", "base\nstaged\n")
	runWorkingDiffGit(t, repo, "add", "staged.txt")
	writeWorkingDiffFile(t, repo, "unstaged.txt", "base\nunstaged\n")
	writeWorkingDiffFile(t, repo, "untracked.txt", "new\n")

	result, err := WorkingDiff(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseRef != "origin/main" || result.BaseCommit == "" || result.HeadCommit == "" {
		t.Fatalf("refs = %+v", result)
	}
	if result.Truncated || len(result.Files) != 4 {
		t.Fatalf("files = %+v, truncated = %v", result.Files, result.Truncated)
	}
	for _, name := range []string{"added.txt", "committed.txt", "staged.txt", "unstaged.txt"} {
		if !strings.Contains(result.Patch, "diff --git a/"+name+" b/"+name) {
			t.Fatalf("patch missing %s:\n%s", name, result.Patch)
		}
	}
	statuses := make(map[string]string, len(result.Files))
	for _, file := range result.Files {
		statuses[file.Path] = file.Status
	}
	if statuses["added.txt"] != "added" || statuses["committed.txt"] != "modified" {
		t.Fatalf("statuses = %v", statuses)
	}
	if strings.Contains(result.Patch, "untracked.txt") ||
		len(result.UntrackedPaths) != 1 || result.UntrackedPaths[0] != "untracked.txt" {
		t.Fatalf("untracked = %v, patch = %q", result.UntrackedPaths, result.Patch)
	}
}

func TestWorkingDiffPathFilterAndBoundary(t *testing.T) {
	repo := workingDiffRepo(t)
	writeWorkingDiffFile(t, repo, "one.txt", "base\none\n")
	writeWorkingDiffFile(t, repo, "two.txt", "base\ntwo\n")
	writeWorkingDiffFile(t, repo, "new.txt", "new\n")

	result, err := WorkingDiff(context.Background(), repo, "one.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "one.txt" ||
		strings.Contains(result.Patch, "two.txt") || len(result.UntrackedPaths) != 0 {
		t.Fatalf("filtered result = %+v", result)
	}
	for _, path := range []string{"../outside", filepath.Join(repo, "one.txt")} {
		if _, err := WorkingDiff(context.Background(), repo, path); err == nil {
			t.Fatalf("accepted path %q", path)
		}
	}
}

func TestWorkingDiffReportsMergeBaseWhenDefaultBranchAdvanced(t *testing.T) {
	repo := workingDiffRepo(t)
	runWorkingDiffGit(t, repo, "switch", "main")
	writeWorkingDiffFile(t, repo, "one.txt", "main advanced\n")
	runWorkingDiffGit(t, repo, "add", "one.txt")
	runWorkingDiffGit(t, repo, "commit", "-m", "advance main")
	runWorkingDiffGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mainTip := workingDiffGitOutput(t, repo, "rev-parse", "HEAD")
	runWorkingDiffGit(t, repo, "switch", "feature")
	writeWorkingDiffFile(t, repo, "two.txt", "feature change\n")
	mergeBase := workingDiffGitOutput(t, repo, "merge-base", mainTip, "HEAD")

	result, err := WorkingDiff(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseCommit != mergeBase || result.BaseCommit == mainTip {
		t.Fatalf("base commit = %q, want merge base %q instead of main tip %q", result.BaseCommit, mergeBase, mainTip)
	}
	if !strings.Contains(result.Patch, "two.txt") || strings.Contains(result.Patch, "one.txt") {
		t.Fatalf("patch = %q", result.Patch)
	}
}

func TestWorkingDiffTruncatesPatchButKeepsSummary(t *testing.T) {
	repo := workingDiffRepo(t)
	writeWorkingDiffFile(t, repo, "large.txt", strings.Repeat("changed line\n", 5000))

	result, err := WorkingDiff(context.Background(), repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Files) != 1 || len(result.Patch) > 50*1024 {
		t.Fatalf("result = files %v, patch bytes %d, truncated %v", result.Files, len(result.Patch), result.Truncated)
	}
}

func workingDiffRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runWorkingDiffGit(t, repo, "init", "-b", "main")
	runWorkingDiffGit(t, repo, "config", "user.name", "Test User")
	runWorkingDiffGit(t, repo, "config", "user.email", "test@example.com")
	for _, name := range []string{"committed.txt", "staged.txt", "unstaged.txt", "one.txt", "two.txt", "large.txt"} {
		writeWorkingDiffFile(t, repo, name, "base\n")
	}
	runWorkingDiffGit(t, repo, "add", ".")
	runWorkingDiffGit(t, repo, "commit", "-m", "base")
	runWorkingDiffGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runWorkingDiffGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runWorkingDiffGit(t, repo, "switch", "-c", "feature")
	return repo
}

func writeWorkingDiffFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func runWorkingDiffGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func workingDiffGitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
