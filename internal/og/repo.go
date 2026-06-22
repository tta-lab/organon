package og

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/project"
)

const (
	branchMain   = "main"
	branchMaster = "master"
	headRefName  = "HEAD"
	remoteOrigin = "origin"
	stateAll     = "all"
)

var (
	semverTagRe = regexp.MustCompile(
		`^v\d+\.\d+\.\d+(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
	semverTagBaseRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
)

type repoContext struct {
	WorkDir      string
	ProjectAlias string
	Provider     gitprovider.ProviderType
	Host         string
	Owner        string
	Repo         string
	RemoteURL    string
	TokenEnv     string
	Token        string
	DefaultBase  string
	Branch       string
}

func resolveRepoContextFor(workDir string) (*repoContext, error) {
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	root, err := gitOutput(workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	e, err := project.GetByPath(config.ProjectsPath(), root)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("workdir %q is not inside a registered project", root)
	}
	remote, err := gitOutput(root, "remote", "get-url", remoteOrigin)
	if err != nil {
		return nil, fmt.Errorf("get origin remote: %w", err)
	}
	info, err := gitprovider.ParseRemoteURL(remote)
	if err != nil {
		return nil, err
	}
	branch, err := gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get current branch: %w", err)
	}
	if branch == headRefName || branch == "" {
		return nil, fmt.Errorf("not on a named branch")
	}
	base := defaultBranch(root)
	tokenEnv := tokenEnvFor(info.Provider, e)
	token := ""
	if tokenEnv != "" {
		token = os.Getenv(tokenEnv)
	}
	return &repoContext{
		WorkDir:      root,
		ProjectAlias: e.Alias,
		Provider:     info.Provider,
		Host:         info.Host,
		Owner:        info.Owner,
		Repo:         info.Repo,
		RemoteURL:    remote,
		TokenEnv:     tokenEnv,
		Token:        token,
		DefaultBase:  base,
		Branch:       branch,
	}, nil
}

func tokenEnvFor(provider gitprovider.ProviderType, e *project.Entry) string {
	if provider == gitprovider.ProviderGitHub {
		if e != nil && e.GitHubTokenEnv != "" {
			return e.GitHubTokenEnv
		}
		for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
			if os.Getenv(name) != "" {
				return name
			}
		}
		return "GITHUB_TOKEN"
	}
	return gitutil.ForgeTokenEnv()
}

func gitOutput(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func runGit(workDir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

var runGitWithCredsFunc = runGitWithCredsImpl

func runGitWithCreds(ctxInfo *repoContext, args ...string) error {
	if ctxInfo.TokenEnv != "" && ctxInfo.Token == "" {
		return requireToken(ctxInfo)
	}
	return runGitWithCredsFunc(ctxInfo, args...)
}

func runGitWithCredsImpl(ctxInfo *repoContext, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ctxInfo.WorkDir}, args...)...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, gitCredentialEnv(ctxInfo)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitCredentialEnv(ctxInfo *repoContext) []string {
	return gitutil.GitCredEnvWithToken(ctxInfo.Token)
}

func defaultBranch(workDir string) string {
	out, err := gitOutput(workDir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		if _, branch, ok := strings.Cut(out, "origin/"); ok && branch != "" {
			return branch
		}
	}
	return branchMain
}

func latestTag(workDir string) (string, error) {
	out, err := gitOutput(workDir, "tag", "--sort=-version:refname")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}

func computeBumpedTag(ctxInfo *repoContext, level string) (string, error) {
	latest, err := latestTag(ctxInfo.WorkDir)
	if err != nil {
		return "", err
	}
	if latest == "" {
		switch level {
		case "major":
			return "v1.0.0", nil
		case "minor":
			return "v0.1.0", nil
		case "patch":
			return "v0.0.1", nil
		default:
			return "", fmt.Errorf("invalid --bump value %q", level)
		}
	}
	shouldBump, err := shouldBumpLatestTag(ctxInfo, latest)
	if err != nil {
		return "", err
	}
	if !shouldBump {
		return latest, nil
	}
	m := semverTagBaseRe.FindStringSubmatch(latest)
	if m == nil {
		return "", fmt.Errorf("latest tag %q is not a plain semver tag", latest)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	suffix := m[4]
	switch level {
	case "major":
		maj++
		min = 0
		pat = 0
	case "minor":
		min++
		pat = 0
	case "patch":
		pat++
	default:
		return "", fmt.Errorf("invalid --bump value %q", level)
	}
	return fmt.Sprintf("v%d.%d.%d%s", maj, min, pat, suffix), nil
}

func shouldBumpLatestTag(ctxInfo *repoContext, tag string) (bool, error) {
	if err := runGit(ctxInfo.WorkDir, "remote", "get-url", remoteOrigin); err != nil {
		return true, nil
	}
	ref := "refs/tags/" + tag
	if err := runGitWithCreds(ctxInfo, "ls-remote", "--exit-code", "--tags", remoteOrigin, ref); err != nil {
		if exitCode(err) == 2 {
			return false, nil
		}
		return false, fmt.Errorf("check remote tag %q: %w", tag, err)
	}
	return true, nil
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func localTagExists(workDir, tag string) bool {
	err := runGit(workDir, "show-ref", "--verify", "--quiet", "refs/tags/"+tag)
	return err == nil
}

func ensureCleanBranchForCleanup(ctxInfo *repoContext) error {
	out, err := gitOutput(ctxInfo.WorkDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("refusing merged-branch cleanup: cannot verify worktree is clean: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("refusing merged-branch cleanup: worktree has uncommitted changes")
	}
	if err := runGitWithCreds(ctxInfo, "fetch", "--prune", remoteOrigin); err != nil {
		return fmt.Errorf("refusing merged-branch cleanup: cannot refresh origin: %w", err)
	}
	remoteRef := "refs/remotes/" + remoteOrigin + "/" + ctxInfo.Branch
	if err := runGit(ctxInfo.WorkDir, "show-ref", "--verify", "--quiet", remoteRef); err != nil {
		return nil
	}
	compareRef := remoteOrigin + "/" + ctxInfo.Branch + "..." + ctxInfo.Branch
	ahead, err := gitOutput(ctxInfo.WorkDir, "rev-list", "--right-only", "--count", compareRef)
	if err != nil {
		return fmt.Errorf("refusing merged-branch cleanup: cannot check local commits: %w", err)
	}
	if strings.TrimSpace(ahead) != "0" {
		return fmt.Errorf(
			"refusing merged-branch cleanup: %s has %s local commit(s) not on origin/%s",
			ctxInfo.Branch,
			strings.TrimSpace(ahead),
			ctxInfo.Branch,
		)
	}
	return nil
}

func cleanupMergedBranch(ctxInfo *repoContext) error {
	if err := ensureCleanBranchForCleanup(ctxInfo); err != nil {
		return err
	}
	remoteExists := remoteBranchExists(ctxInfo)
	for _, args := range [][]string{
		{"switch", ctxInfo.DefaultBase},
		{"pull", "--ff-only", remoteOrigin, ctxInfo.DefaultBase},
	} {
		if err := runGitWithCreds(ctxInfo, args...); err != nil {
			return err
		}
	}
	if remoteExists {
		if err := runGitWithCreds(ctxInfo, "push", remoteOrigin, "--delete", ctxInfo.Branch); err != nil {
			return err
		}
	}
	return runGitWithCreds(ctxInfo, "branch", "-D", ctxInfo.Branch)
}

func remoteBranchExists(ctxInfo *repoContext) bool {
	remoteRef := "refs/remotes/" + remoteOrigin + "/" + ctxInfo.Branch
	return runGit(ctxInfo.WorkDir, "show-ref", "--verify", "--quiet", remoteRef) == nil
}
