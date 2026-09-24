package og

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/truncate"
)

const (
	gitDiffCommand = "diff"
	noRenamesFlag  = "--no-renames"
)

// WorkingDiffFile summarizes one tracked file in a working diff.
type WorkingDiffFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

// WorkingDiffResult describes tracked changes from the default branch merge
// base through the working tree, plus untracked paths reported separately.
type WorkingDiffResult struct {
	BaseRef        string            `json:"base_ref"`
	BaseCommit     string            `json:"base_commit"`
	HeadCommit     string            `json:"head_commit"`
	Files          []WorkingDiffFile `json:"files"`
	UntrackedPaths []string          `json:"untracked_paths"`
	Patch          string            `json:"patch"`
	Truncated      bool              `json:"truncated"`
}

// WorkingDiff inspects one registered checkout without fetching or changing it.
func WorkingDiff(ctx context.Context, workDir, path string) (WorkingDiffResult, error) {
	path, err := workingDiffPath(path)
	if err != nil {
		return WorkingDiffResult{}, err
	}
	defaultBase, ok := defaultBranch(ctx, workDir)
	if !ok {
		return WorkingDiffResult{}, fmt.Errorf("default remote branch is unknown; fetch origin and set origin/HEAD")
	}
	baseRef := remoteOrigin + "/" + defaultBase
	baseCommit, err := workingDiffGit(ctx, workDir, "rev-parse", "--verify", baseRef+"^{commit}")
	if err != nil {
		return WorkingDiffResult{}, fmt.Errorf("resolve base ref %q: %w", baseRef, err)
	}
	headCommit, err := workingDiffGit(ctx, workDir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return WorkingDiffResult{}, fmt.Errorf("resolve HEAD: %w", err)
	}
	mergeBase, err := workingDiffGit(ctx, workDir, "merge-base", baseCommit, headCommit)
	if err != nil {
		return WorkingDiffResult{}, fmt.Errorf("find merge base for %q and HEAD: %w", baseRef, err)
	}

	diffArgs := appendWorkingDiffPath(
		[]string{gitDiffCommand, "--no-ext-diff", noRenamesFlag, mergeBase}, path,
	)
	patch, err := workingDiffGitRaw(ctx, workDir, diffArgs...)
	if err != nil {
		return WorkingDiffResult{}, fmt.Errorf("read working diff: %w", err)
	}
	files, err := workingDiffFiles(ctx, workDir, mergeBase, path)
	if err != nil {
		return WorkingDiffResult{}, err
	}
	untracked, err := workingDiffUntracked(ctx, workDir, path)
	if err != nil {
		return WorkingDiffResult{}, err
	}
	bounded := truncate.Head(patch, truncate.DefaultMaxLines, truncate.DefaultMaxBytes)
	return WorkingDiffResult{
		BaseRef: baseRef, BaseCommit: mergeBase, HeadCommit: headCommit,
		Files: files, UntrackedPaths: untracked, Patch: bounded.Content, Truncated: bounded.Truncated,
	}, nil
}

func workingDiffPath(path string) (string, error) {
	if path == "" || path == "." {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must remain within the repository")
	}
	return filepath.ToSlash(clean), nil
}

func appendWorkingDiffPath(args []string, path string) []string {
	if path == "" {
		return args
	}
	return append(args, "--", path)
}

func workingDiffFiles(ctx context.Context, workDir, mergeBase, path string) ([]WorkingDiffFile, error) {
	statusArgs := appendWorkingDiffPath(
		[]string{gitDiffCommand, "--name-status", "-z", noRenamesFlag, mergeBase}, path,
	)
	statusRaw, err := workingDiffGitRaw(ctx, workDir, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("read working diff statuses: %w", err)
	}
	statuses := make(map[string]string)
	fields := strings.Split(statusRaw, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		statuses[fields[i+1]] = workingDiffStatus(fields[i])
	}

	numstatArgs := appendWorkingDiffPath(
		[]string{gitDiffCommand, "--numstat", "-z", noRenamesFlag, mergeBase}, path,
	)
	numstatRaw, err := workingDiffGitRaw(ctx, workDir, numstatArgs...)
	if err != nil {
		return nil, fmt.Errorf("read working diff statistics: %w", err)
	}
	entries := strings.Split(numstatRaw, "\x00")
	files := make([]WorkingDiffFile, 0, len(entries))
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("parse working diff statistics")
		}
		file := WorkingDiffFile{Path: parts[2], Status: statuses[parts[2]]}
		if parts[0] == "-" || parts[1] == "-" {
			file.Binary = true
		} else {
			file.Additions, _ = strconv.Atoi(parts[0])
			file.Deletions, _ = strconv.Atoi(parts[1])
		}
		files = append(files, file)
	}
	return files, nil
}

func workingDiffStatus(status string) string {
	switch status {
	case "A":
		return "added"
	case "D":
		return "deleted"
	default:
		return "modified"
	}
}

func workingDiffUntracked(ctx context.Context, workDir, path string) ([]string, error) {
	args := appendWorkingDiffPath([]string{"ls-files", "--others", "--exclude-standard", "-z"}, path)
	raw, err := workingDiffGitRaw(ctx, workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("list untracked paths: %w", err)
	}
	if raw == "" {
		return []string{}, nil
	}
	paths := strings.Split(raw, "\x00")
	return paths[:len(paths)-1], nil
}

func workingDiffGit(ctx context.Context, workDir string, args ...string) (string, error) {
	out, err := workingDiffGitRaw(ctx, workDir, args...)
	return strings.TrimSpace(out), err
}

func workingDiffGitRaw(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	cmd.Env = gitutil.AnonymousGitEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr)
	}
	return string(out), nil
}
