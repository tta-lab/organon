package og

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tta-lab/organon/internal/truncate"
)

const DefaultCommitLimit = 20
const MaxCommitLimit = 100

type RepositoryStatus struct {
	Branch          string   `json:"branch,omitempty"`
	HeadCommit      string   `json:"head_commit,omitempty"`
	Detached        bool     `json:"detached"`
	StagedPaths     []string `json:"staged_paths"`
	UnstagedPaths   []string `json:"unstaged_paths"`
	UntrackedPaths  []string `json:"untracked_paths"`
	ConflictedPaths []string `json:"conflicted_paths"`
}

// RepoStatus reports the local checkout state without contacting a remote.
//
//nolint:gocyclo // Porcelain's mutually exclusive path states are parsed in one pass.
func RepoStatus(ctx context.Context, workDir string) (RepositoryStatus, error) {
	raw, err := workingDiffGitRaw(ctx, workDir, "status", "--porcelain=v1", "-z",
		"--untracked-files=all", "--no-renames")
	if err != nil {
		return RepositoryStatus{}, fmt.Errorf("read repository status: %w", err)
	}
	branch, err := workingDiffGit(ctx, workDir, "branch", "--show-current")
	if err != nil {
		return RepositoryStatus{}, fmt.Errorf("read current branch: %w", err)
	}
	head, _ := workingDiffGit(ctx, workDir, "rev-parse", "--verify", "HEAD^{commit}")
	result := RepositoryStatus{
		Branch: branch, HeadCommit: head, Detached: branch == "",
		StagedPaths: []string{}, UnstagedPaths: []string{},
		UntrackedPaths: []string{}, ConflictedPaths: []string{},
	}
	if raw == "" {
		return result, nil
	}
	for _, record := range strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00") {
		if len(record) < 4 || record[2] != ' ' {
			return RepositoryStatus{}, fmt.Errorf("parse repository status")
		}
		xy, path := record[:2], record[3:]
		switch xy {
		case "??":
			result.UntrackedPaths = append(result.UntrackedPaths, path)
		case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
			result.ConflictedPaths = append(result.ConflictedPaths, path)
		default:
			if xy[0] != ' ' {
				result.StagedPaths = append(result.StagedPaths, path)
			}
			if xy[1] != ' ' {
				result.UnstagedPaths = append(result.UnstagedPaths, path)
			}
		}
	}
	sort.Strings(result.StagedPaths)
	sort.Strings(result.UnstagedPaths)
	sort.Strings(result.UntrackedPaths)
	sort.Strings(result.ConflictedPaths)
	return result, nil
}

type CommitSummary struct {
	ID         string   `json:"id"`
	Parents    []string `json:"parents"`
	AuthoredAt string   `json:"authored_at"`
	Subject    string   `json:"subject"`
}

type CommitListResult struct {
	Ref     string          `json:"ref"`
	Commits []CommitSummary `json:"commits"`
}

// RepoCommits lists recent commits reachable from HEAD or the local origin default.
func RepoCommits(ctx context.Context, workDir, ref string, limit int) (CommitListResult, error) {
	if limit < 1 || limit > MaxCommitLimit {
		return CommitListResult{}, fmt.Errorf("limit must be between 1 and %d", MaxCommitLimit)
	}
	resolved := "HEAD"
	switch ref {
	case "", "head":
	case "default":
		branch, ok := defaultBranch(ctx, workDir)
		if !ok {
			return CommitListResult{}, fmt.Errorf("default remote branch is unknown; fetch origin and set origin/HEAD")
		}
		resolved = remoteOrigin + "/" + branch
	default:
		return CommitListResult{}, fmt.Errorf("ref must be head or default")
	}
	raw, err := workingDiffGitRaw(ctx, workDir, "log", "--max-count="+strconv.Itoa(limit),
		"--format=%H%x00%P%x00%aI%x00%s", resolved)
	if err != nil {
		return CommitListResult{}, fmt.Errorf("list commits from %q: %w", resolved, err)
	}
	result := CommitListResult{Ref: resolved, Commits: []CommitSummary{}}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			return CommitListResult{}, fmt.Errorf("parse commit history")
		}
		parents := []string{}
		if fields[1] != "" {
			parents = strings.Fields(fields[1])
		}
		result.Commits = append(result.Commits, CommitSummary{
			ID: fields[0], Parents: parents, AuthoredAt: fields[2], Subject: fields[3],
		})
	}
	return result, nil
}

var fullCommitID = regexp.MustCompile(`(?i)^[0-9a-f]{40}([0-9a-f]{24})?$`)

type CommitCompareResult struct {
	BaseCommit string            `json:"base_commit"`
	HeadCommit string            `json:"head_commit"`
	Files      []WorkingDiffFile `json:"files"`
	Patch      string            `json:"patch"`
	Truncated  bool              `json:"truncated"`
}

// RepoCompare compares two committed snapshots, never the working tree.
func RepoCompare(ctx context.Context, workDir, baseCommit, headCommit, path string) (CommitCompareResult, error) {
	path, err := workingDiffPath(path)
	if err != nil {
		return CommitCompareResult{}, err
	}
	for _, id := range []string{baseCommit, headCommit} {
		if !fullCommitID.MatchString(id) {
			return CommitCompareResult{}, fmt.Errorf("commit IDs must be full hexadecimal object IDs")
		}
		kind, err := workingDiffGit(ctx, workDir, "cat-file", "-t", id)
		if err != nil || kind != "commit" {
			return CommitCompareResult{}, fmt.Errorf("object %q is not a commit", id)
		}
	}
	args := appendWorkingDiffPath(
		[]string{gitDiffCommand, "--no-ext-diff", noRenamesFlag, baseCommit, headCommit}, path,
	)
	patch, err := workingDiffGitRaw(ctx, workDir, args...)
	if err != nil {
		return CommitCompareResult{}, fmt.Errorf("read commit diff: %w", err)
	}
	files, err := diffFiles(ctx, workDir, baseCommit, headCommit, path)
	if err != nil {
		return CommitCompareResult{}, err
	}
	bounded := truncate.Head(patch, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	return CommitCompareResult{
		BaseCommit: baseCommit, HeadCommit: headCommit,
		Files: files, Patch: bounded.Content, Truncated: bounded.Truncated,
	}, nil
}
