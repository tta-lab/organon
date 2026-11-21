package og

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tta-lab/organon/internal/project"
)

type worktreeRecord struct {
	path, head, branch string
	locked, prunable   bool
}

// checkLinkedCleanup verifies the selected worktree and its HEAD. Detached HEAD
// is allowed only for cleanup recovery.
//
//nolint:gocyclo
func checkLinkedCleanup(ctxInfo *repoContext, target branchCleanupTarget, allowDetached bool) (bool, error) {
	ctx := operationContext(ctxInfo)
	selectedCommon, err := gitCommonDir(ctx, ctxInfo.WorkDir)
	if err != nil {
		return false, err
	}
	registeredCommon, err := gitCommonDir(ctx, ctxInfo.RegisteredDir)
	if err != nil || !sameRealPath(selectedCommon, registeredCommon) {
		return false, fmt.Errorf("selected worktree no longer belongs to the registered repository")
	}
	root, err := controlledGitOutput(ctx, ctxInfo.WorkDir, "rev-parse", "--show-toplevel")
	if err != nil || !sameRealPath(root, ctxInfo.WorkDir) || sameRealPath(root, ctxInfo.RegisteredDir) {
		return false, fmt.Errorf("selected linked worktree root is no longer valid")
	}
	records, err := worktreeRecords(ctx, ctxInfo.RegisteredDir)
	if err != nil {
		return false, err
	}
	var selected *worktreeRecord
	registered := false
	for i := range records {
		record := &records[i]
		if sameRealPath(record.path, ctxInfo.RegisteredDir) && !record.prunable {
			registered = true
		}
		if sameRealPath(record.path, ctxInfo.WorkDir) {
			if selected != nil {
				return false, fmt.Errorf("selected worktree has ambiguous Git records")
			}
			selected = record
		} else if record.branch == "refs/heads/"+target.HeadBranch {
			return false, fmt.Errorf("head branch %q is checked out in another worktree %q", target.HeadBranch, record.path)
		}
	}
	if !registered || selected == nil || selected.locked || selected.prunable {
		return false, fmt.Errorf("selected worktree is missing, locked, or no longer registered in Git")
	}
	if !sameGitObjectID(selected.head, target.HeadSHA) {
		return false, fmt.Errorf("selected worktree HEAD moved from approved SHA")
	}
	detached := selected.branch == ""
	if selected.branch != "refs/heads/"+target.HeadBranch && (!detached || !allowDetached) {
		return false, fmt.Errorf("selected worktree is not on approved head branch %q", target.HeadBranch)
	}
	if err := ensureCleanupWorktreeClean(ctxInfo); err != nil {
		return false, err
	}
	return detached, nil
}

func ensureLinkedCleanupTarget(ctxInfo *repoContext, target branchCleanupTarget, allowDetached bool) (bool, error) {
	if err := refreshCleanupRemote(ctxInfo); err != nil {
		return false, err
	}
	detached, err := checkLinkedCleanup(ctxInfo, target, allowDetached)
	if err != nil {
		return false, err
	}
	if err := ensureCleanupDefaultRef(ctxInfo, target); err != nil {
		return false, err
	}
	_, err = verifyBranchCleanupRefs(ctxInfo, target)
	return detached, err
}

func cleanupLinkedWorktree(ctxInfo *repoContext, target branchCleanupTarget) error {
	detached, err := ensureLinkedCleanupTarget(ctxInfo, target, ctxInfo.ExplicitPR)
	if err != nil {
		return err
	}
	ctx := operationContext(ctxInfo)
	if !detached {
		if err := runGit(ctx, ctxInfo.WorkDir, "update-ref", "--no-deref", "HEAD",
			target.HeadSHA, target.HeadSHA); err != nil {
			return err
		}
	}
	if err := deleteCleanupLocal(ctxInfo, target); err != nil {
		return err
	}
	refs, err := verifyBranchCleanupRefs(ctxInfo, target)
	if err != nil {
		return err
	}
	if err := deleteCleanupRemote(ctxInfo, target, refs); err != nil {
		return err
	}
	detached, err = checkLinkedCleanup(ctxInfo, target, true)
	if err != nil {
		return err
	}
	if !detached {
		return fmt.Errorf("selected worktree was reattached during cleanup")
	}
	return runGit(ctx, ctxInfo.RegisteredDir, "worktree", "remove", "--", ctxInfo.WorkDir)
}

// ResolveWorktree resolves an absolute, exact linked worktree root to its
// registered repository. It does not create registry entries.
func ResolveWorktree(ctx context.Context, path string, projects *project.Store) (string, project.Entry, error) {
	if !filepath.IsAbs(path) || strings.TrimSpace(path) == "" {
		return "", project.Entry{}, fmt.Errorf("worktree must be an absolute path")
	}
	root, entry, err := resolveRegisteredRepo(ctx, path, projects)
	if err != nil {
		return "", project.Entry{}, err
	}
	if !sameRealPath(path, root) || sameRealPath(root, entry.Path) {
		return "", project.Entry{}, fmt.Errorf("worktree must be an exact linked worktree root")
	}
	return root, entry, nil
}

func gitCommonDir(ctx context.Context, dir string) (string, error) {
	return controlledGitOutput(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

func worktreeRecords(ctx context.Context, dir string) ([]worktreeRecord, error) {
	out, err := controlledGitOutput(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var records []worktreeRecord
	var record worktreeRecord
	for _, field := range strings.Split(out, "\x00") {
		switch {
		case field == "":
			if record.path != "" {
				records = append(records, record)
				record = worktreeRecord{}
			}
		case strings.HasPrefix(field, "worktree "):
			record.path = strings.TrimPrefix(field, "worktree ")
		case strings.HasPrefix(field, "HEAD "):
			record.head = strings.TrimPrefix(field, "HEAD ")
		case strings.HasPrefix(field, "branch "):
			record.branch = strings.TrimPrefix(field, "branch ")
		case field == "locked" || strings.HasPrefix(field, "locked "):
			record.locked = true
		case field == "prunable" || strings.HasPrefix(field, "prunable "):
			record.prunable = true
		}
	}
	if record.path != "" {
		records = append(records, record)
	}
	return records, nil
}

//nolint:gocyclo
func resolveWorktreeRoot(ctx context.Context, root string, projects *project.Store) (string, project.Entry, error) {
	entries, err := projects.List(true)
	if err != nil {
		return "", project.Entry{}, err
	}
	common, err := gitCommonDir(ctx, root)
	if err != nil {
		return "", project.Entry{}, err
	}
	records, err := worktreeRecords(ctx, root)
	if err != nil {
		return "", project.Entry{}, err
	}
	foundRoot := false
	for _, record := range records {
		if !record.prunable && sameRealPath(record.path, root) {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		return "", project.Entry{}, fmt.Errorf("workdir %q is not in Git worktree records", root)
	}
	var matched []project.Entry
	for _, entry := range entries {
		foundRegistered := false
		for _, record := range records {
			if !record.prunable && sameRealPath(record.path, entry.Path) {
				foundRegistered = true
				break
			}
		}
		if !foundRegistered {
			continue
		}
		registeredRoot, rootErr := controlledGitOutput(ctx, entry.Path, "rev-parse", "--show-toplevel")
		if rootErr != nil || !sameRealPath(registeredRoot, entry.Path) {
			continue
		}
		registeredCommon, commonErr := gitCommonDir(ctx, entry.Path)
		if commonErr != nil || !sameRealPath(common, registeredCommon) {
			continue
		}
		matched = append(matched, entry)
	}
	if len(matched) != 1 {
		return "", project.Entry{}, fmt.Errorf("workdir %q has %d registered repository owners", root, len(matched))
	}
	return root, matched[0], nil
}
