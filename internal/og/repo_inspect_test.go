package og

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoStatusSeparatesWorkingTreeStates(t *testing.T) {
	repo := workingDiffRepo(t)
	writeWorkingDiffFile(t, repo, "staged.txt", "staged\n")
	runWorkingDiffGit(t, repo, "add", "staged.txt")
	writeWorkingDiffFile(t, repo, "staged.txt", "staged and unstaged\n")
	writeWorkingDiffFile(t, repo, "unstaged.txt", "unstaged\n")
	writeWorkingDiffFile(t, repo, "new file.txt", "new\n")
	result, err := RepoStatus(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.Branch != "feature" || result.HeadCommit == "" || result.Detached ||
		!strings.Contains(strings.Join(result.StagedPaths, ","), "staged.txt") ||
		len(result.UnstagedPaths) != 2 || len(result.UntrackedPaths) != 1 ||
		result.UntrackedPaths[0] != "new file.txt" || len(result.ConflictedPaths) != 0 {
		t.Fatalf("status = %+v", result)
	}
	runWorkingDiffGit(t, repo, "checkout", "--detach")
	detached, err := RepoStatus(context.Background(), repo)
	if err != nil || !detached.Detached || detached.Branch != "" {
		t.Fatalf("detached = %+v, %v", detached, err)
	}
}

func TestRepoStatusReportsConflicts(t *testing.T) {
	repo := workingDiffRepo(t)
	writeWorkingDiffFile(t, repo, "one.txt", "feature\n")
	runWorkingDiffGit(t, repo, "add", "one.txt")
	runWorkingDiffGit(t, repo, "commit", "-m", "feature")
	runWorkingDiffGit(t, repo, "switch", "main")
	writeWorkingDiffFile(t, repo, "one.txt", "main\n")
	runWorkingDiffGit(t, repo, "add", "one.txt")
	runWorkingDiffGit(t, repo, "commit", "-m", "main")
	runWorkingDiffGit(t, repo, "switch", "feature")
	// A conflict is the expected nonzero outcome; do not mutate anything outside this fixture.
	if _, err := workingDiffGitRaw(context.Background(), repo, "merge", "main"); err == nil {
		t.Fatal("expected merge conflict")
	}
	result, err := RepoStatus(context.Background(), repo)
	if err != nil || len(result.ConflictedPaths) != 1 || result.ConflictedPaths[0] != "one.txt" {
		t.Fatalf("status = %+v, %v", result, err)
	}
}

//nolint:gocyclo // One fixture covers the coupled history and compare contract.
func TestRepoCommitsAndCompareExactSnapshots(t *testing.T) {
	repo := workingDiffRepo(t)
	base := workingDiffGitOutput(t, repo, "rev-parse", "HEAD")
	writeWorkingDiffFile(t, repo, "one.txt", "feature\n")
	if err := os.WriteFile(filepath.Join(repo, "added.txt"), []byte("added\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runWorkingDiffGit(t, repo, "add", "one.txt", "added.txt")
	runWorkingDiffGit(t, repo, "commit", "-m", "feature change")
	head := workingDiffGitOutput(t, repo, "rev-parse", "HEAD")
	writeWorkingDiffFile(t, repo, "one.txt", "uncommitted\n")

	history, err := RepoCommits(context.Background(), repo, "", 2)
	if err != nil || history.Ref != "HEAD" || len(history.Commits) != 2 ||
		history.Commits[0].ID != head || history.Commits[0].Parents[0] != base ||
		history.Commits[0].Subject != "feature change" || history.Commits[0].AuthoredAt == "" {
		t.Fatalf("history = %+v, %v", history, err)
	}
	defaultHistory, err := RepoCommits(context.Background(), repo, "default", 1)
	if err != nil || defaultHistory.Ref != "origin/main" || len(defaultHistory.Commits) != 1 ||
		defaultHistory.Commits[0].ID != base {
		t.Fatalf("default history = %+v, %v", defaultHistory, err)
	}
	compare, err := RepoCompare(context.Background(), repo, base, head, "")
	if err != nil || len(compare.Files) != 2 || compare.Truncated ||
		!strings.Contains(compare.Patch, "+feature") || strings.Contains(compare.Patch, "+uncommitted") {
		t.Fatalf("compare = %+v, %v", compare, err)
	}
	filtered, err := RepoCompare(context.Background(), repo, base, head, "one.txt")
	if err != nil || len(filtered.Files) != 1 || filtered.Files[0].Path != "one.txt" ||
		strings.Contains(filtered.Patch, "added.txt") {
		t.Fatalf("filtered = %+v, %v", filtered, err)
	}
	for _, args := range [][3]string{
		{"HEAD", head, ""}, {base[:8], head, ""}, {base, "not-a-commit", ""},
		{base, head, "../outside"},
	} {
		if _, err := RepoCompare(context.Background(), repo, args[0], args[1], args[2]); err == nil {
			t.Fatalf("accepted compare arguments %q", args)
		}
	}
	for _, ref := range []string{"HEAD~1", "main", "--all"} {
		if _, err := RepoCommits(context.Background(), repo, ref, 1); err == nil {
			t.Fatalf("accepted ref %q", ref)
		}
	}
	for _, limit := range []int{0, MaxCommitLimit + 1} {
		if _, err := RepoCommits(context.Background(), repo, "head", limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
}

//nolint:gocyclo // One fixture covers the shared patch, summary, and untracked path filter.
func TestRepositoryDiffPathsAreLiteralForTrackedAndUntrackedFiles(t *testing.T) {
	repo := workingDiffRepo(t)
	base := workingDiffGitOutput(t, repo, "rev-parse", "HEAD")
	for _, name := range []string{"[r]epo", "repo"} {
		if err := os.Mkdir(filepath.Join(repo, name), 0700); err != nil {
			t.Fatal(err)
		}
		writeWorkingDiffFile(t, repo, filepath.Join(name, "tracked.txt"), "committed\n")
	}
	writeWorkingDiffFile(t, repo, "repo.txt", "committed\n")
	runWorkingDiffGit(t, repo, "add", "-A")
	runWorkingDiffGit(t, repo, "commit", "-m", "add similar directories")
	head := workingDiffGitOutput(t, repo, "rev-parse", "HEAD")
	writeWorkingDiffFile(t, repo, filepath.Join("[r]epo", "untracked.txt"), "new\n")
	writeWorkingDiffFile(t, repo, filepath.Join("repo", "untracked.txt"), "other\n")
	writeWorkingDiffFile(t, repo, "new.txt", "other\n")

	working, err := WorkingDiff(context.Background(), repo, "[r]epo")
	if err != nil || len(working.Files) != 1 ||
		working.Files[0].Path != "[r]epo/tracked.txt" ||
		len(working.UntrackedPaths) != 1 || working.UntrackedPaths[0] != "[r]epo/untracked.txt" ||
		strings.Contains(working.Patch, "repo/tracked.txt") {
		t.Fatalf("working diff = %+v, %v", working, err)
	}
	compare, err := RepoCompare(context.Background(), repo, base, head, "[r]epo")
	if err != nil || len(compare.Files) != 1 ||
		compare.Files[0].Path != "[r]epo/tracked.txt" ||
		strings.Contains(compare.Patch, "repo/tracked.txt") {
		t.Fatalf("commit compare = %+v, %v", compare, err)
	}
	for _, path := range []string{"[r]epo.txt", ":(top,glob)**"} {
		working, err := WorkingDiff(context.Background(), repo, path)
		if err != nil || len(working.Files) != 0 || len(working.UntrackedPaths) != 0 || working.Patch != "" {
			t.Fatalf("nonexistent literal working path %q = %+v, %v", path, working, err)
		}
		compare, err := RepoCompare(context.Background(), repo, base, head, path)
		if err != nil || len(compare.Files) != 0 || compare.Patch != "" {
			t.Fatalf("nonexistent literal compare path %q = %+v, %v", path, compare, err)
		}
	}
	working, err = WorkingDiff(context.Background(), repo, "[n]ew.txt")
	if err != nil || len(working.UntrackedPaths) != 0 {
		t.Fatalf("nonexistent literal untracked path = %+v, %v", working, err)
	}
}
