package og

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

const (
	branchMain   = "main"
	branchMaster = "master"
	headRefName  = "HEAD"
	remoteOrigin = "origin"
	stateAll     = PRStateAll
)

var (
	semverTagRe = regexp.MustCompile(
		`^v\d+\.\d+\.\d+(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
	semverTagBaseRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
)

type repoContext struct {
	Context          context.Context
	WorkDir          string
	ProjectAlias     string
	Archived         bool
	Provider         gitprovider.ProviderType
	Host             string
	BaseURL          string
	Owner            string
	Repo             string
	RemoteURL        string
	RegistryRemote   string
	TokenEnv         string
	Token            string
	DefaultBase      string
	DefaultBaseKnown bool
	Branch           string
	config           ogconfig.Config
	githubBroker     githubapp.CredentialBroker
}

func resolveRepoContextFor(ctx context.Context, workDir string) (*repoContext, error) {
	return resolveRepoContextWith(
		ctx, workDir, project.NewStore(config.ProjectsPath()), ogconfig.Config{},
	)
}

func resolveRepoContextWith(
	ctx context.Context, workDir string, projects *project.Store, cfg ogconfig.Config,
) (*repoContext, error) {
	ctxInfo, err := resolveRemoteRepoContextWith(ctx, workDir, projects, cfg)
	if err != nil {
		return nil, err
	}
	branch, err := gitOutput(ctx, ctxInfo.WorkDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get current branch: %w", err)
	}
	if branch == headRefName || branch == "" {
		return nil, fmt.Errorf("not on a named branch")
	}
	ctxInfo.DefaultBase, ctxInfo.DefaultBaseKnown = defaultBranch(ctx, ctxInfo.WorkDir)
	ctxInfo.Branch = branch
	return ctxInfo, nil
}

func resolveRemoteRepoContextFor(ctx context.Context, workDir string) (*repoContext, error) {
	return resolveRemoteRepoContextWith(
		ctx, workDir, project.NewStore(config.ProjectsPath()), ogconfig.Config{},
	)
}

func resolveRemoteRepoContextWith(
	ctx context.Context, workDir string, projects *project.Store, cfg ogconfig.Config,
) (*repoContext, error) {
	root, entry, err := resolveRegisteredRepo(ctx, workDir, projects)
	if err != nil {
		return nil, err
	}
	info, err := gitprovider.ParseHTTPRemoteURL(entry.Remote)
	if err != nil {
		return nil, fmt.Errorf("parse registered remote: %w", err)
	}
	provider, err := cfg.ClassifyRemote(info)
	if err != nil {
		return nil, fmt.Errorf("classify origin remote: %w", err)
	}
	info.Provider = provider
	tokenEnv := tokenEnvFor(provider)
	token := ""
	if tokenEnv != "" {
		token = os.Getenv(tokenEnv)
	}
	return &repoContext{
		Context:        ctx,
		WorkDir:        root,
		ProjectAlias:   entry.Alias,
		Archived:       entry.Archived,
		Provider:       info.Provider,
		Host:           info.Host,
		BaseURL:        info.BaseURL,
		Owner:          info.Owner,
		Repo:           info.Repo,
		RemoteURL:      entry.Remote,
		RegistryRemote: entry.Remote,
		TokenEnv:       tokenEnv,
		Token:          token,
		config:         cfg,
	}, nil
}

func validateCurrentRemoteTargets(ctxInfo *repoContext, requirePush bool) error {
	ctx := operationContext(ctxInfo)
	if err := ctx.Err(); err != nil {
		return err
	}
	if ctxInfo.WorkDir == "" || ctxInfo.RemoteURL == "" || ctxInfo.BaseURL == "" {
		return nil
	}
	remote, err := controlledGitOutput(ctx, ctxInfo.WorkDir, "remote", "get-url", remoteOrigin)
	if err != nil {
		return fmt.Errorf("origin fetch target cannot be verified")
	}
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil {
		return fmt.Errorf("origin fetch target is not an allowed HTTP(S) repository")
	}
	provider, err := ctxInfo.config.ClassifyRemote(info)
	expected := ctxInfo.RegistryRemote
	if expected == "" {
		expected = ctxInfo.RemoteURL
	}
	if err != nil || provider != ctxInfo.Provider || info.CanonicalURL != expected {
		return fmt.Errorf("origin fetch target does not match registered remote")
	}
	if requirePush {
		if err := validatePushTargets(ctx, ctxInfo.WorkDir, ctxInfo.config, expected, provider); err != nil {
			return err
		}
	}
	ctxInfo.RemoteURL = remote
	return nil
}

func validatePushTargets(
	ctx context.Context,
	workDir string,
	cfg ogconfig.Config,
	expected string,
	fetchProvider gitprovider.ProviderType,
) error {
	out, err := controlledGitOutput(ctx, workDir, "remote", "get-url", "--push", "--all", remoteOrigin)
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("origin push target cannot be verified")
	}
	for _, raw := range strings.Split(out, "\n") {
		raw = strings.TrimSpace(raw)
		pushInfo, parseErr := gitprovider.ParseHTTPRemoteURL(raw)
		if parseErr != nil {
			return fmt.Errorf("origin push target is not an allowed HTTP(S) repository")
		}
		pushProvider, classifyErr := cfg.ClassifyRemote(pushInfo)
		if classifyErr != nil || pushProvider != fetchProvider || pushInfo.CanonicalURL != expected {
			return fmt.Errorf("origin push target does not match registered remote")
		}
	}
	return nil
}

func resolveRegisteredRepo(
	ctx context.Context,
	workDir string,
	projects *project.Store,
) (string, project.Entry, error) {
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", project.Entry{}, fmt.Errorf("get working directory: %w", err)
		}
	}
	requestedPath, err := filepath.Abs(workDir)
	if err != nil {
		return "", project.Entry{}, fmt.Errorf("resolve working directory: %w", err)
	}
	root, err := gitOutput(ctx, workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", project.Entry{}, fmt.Errorf("not in a git repository: %w", err)
	}
	root = filepath.Clean(root)
	if projects == nil {
		return "", project.Entry{}, fmt.Errorf("project store is not configured")
	}
	registeredInput, inputErr := projects.GetByPath(requestedPath)
	if inputErr == nil {
		if !sameRealPath(registeredInput.Path, root) {
			return "", project.Entry{}, fmt.Errorf(
				"registered project %q path %q must be the Git top-level %q",
				registeredInput.Alias, registeredInput.Path, root,
			)
		}
		return registeredInput.Path, registeredInput, nil
	}
	if inputErr != nil && !errors.Is(inputErr, project.ErrNotFound) {
		return "", project.Entry{}, inputErr
	}
	entry, err := projects.GetByPath(root)
	if errors.Is(err, project.ErrNotFound) {
		entries, listErr := projects.List(true)
		if listErr != nil {
			return "", project.Entry{}, listErr
		}
		for _, candidate := range entries {
			if sameRealPath(candidate.Path, root) {
				return candidate.Path, candidate, nil
			}
		}
		return "", project.Entry{}, fmt.Errorf("workdir %q is not inside a registered project", root)
	}
	if err != nil {
		return "", project.Entry{}, err
	}
	return entry.Path, entry, nil
}

func sameRealPath(left, right string) bool {
	realLeft, leftErr := filepath.EvalSymlinks(left)
	realRight, rightErr := filepath.EvalSymlinks(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(realLeft) == filepath.Clean(realRight)
}

func tokenEnvFor(provider gitprovider.ProviderType) string {
	if provider != gitprovider.ProviderForgejo {
		return ""
	}
	return gitutil.ForgeTokenEnv()
}

func gitOutput(ctx context.Context, workDir string, args ...string) (string, error) {
	return gitOutputWithEnv(ctx, workDir, gitutil.AnonymousGitEnv(os.Environ()), args...)
}

func controlledGitOutput(ctx context.Context, workDir string, args ...string) (string, error) {
	return gitOutput(ctx, workDir, args...)
}

func gitOutputWithEnv(ctx context.Context, workDir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func runGit(ctx context.Context, workDir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	cmd.Env = gitutil.AnonymousGitEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

type gitAuthentication struct {
	token string
}

var runGitWithCredsFunc = runGitWithCredsImpl

func runGitWithCreds(ctxInfo *repoContext, purpose githubapp.Purpose, args ...string) error {
	if err := operationContext(ctxInfo).Err(); err != nil {
		return err
	}
	if err := validateCurrentRemoteTargets(ctxInfo, purpose == githubapp.PurposeGitWrite); err != nil {
		return err
	}
	auth, err := gitAuthenticationFor(ctxInfo, purpose)
	if err != nil {
		return err
	}
	err = runGitWithCredsFunc(ctxInfo, auth, args...)
	if err != nil && ctxInfo.Provider == gitprovider.ProviderGitHub && auth.token != "" &&
		confirmedGitAuthenticationFailure(err) {
		invalidateErr := ctxInfo.githubBroker.Invalidate(
			ctxInfo.Owner, ctxInfo.Repo, purpose, auth.token,
		)
		if invalidateErr != nil {
			return fmt.Errorf("%w (invalidate rejected credential: %v)", err, invalidateErr)
		}
	}
	return err
}

func gitAuthenticationFor(ctxInfo *repoContext, purpose githubapp.Purpose) (gitAuthentication, error) {
	if ctxInfo.Provider == gitprovider.ProviderGeneric {
		if purpose != githubapp.PurposeGitRead {
			return gitAuthentication{}, fmt.Errorf("generic HTTPS repository is read-only")
		}
		return gitAuthentication{}, nil
	}
	if ctxInfo.Provider == gitprovider.ProviderForgejo {
		if err := requireToken(ctxInfo); err != nil {
			return gitAuthentication{}, err
		}
		return gitAuthentication{token: ctxInfo.Token}, nil
	}
	if ctxInfo.githubBroker == nil {
		return gitAuthentication{}, fmt.Errorf("GitHub App authentication is not configured")
	}
	token, err := ctxInfo.githubBroker.Token(operationContext(ctxInfo), ctxInfo.Owner, ctxInfo.Repo, purpose)
	if err != nil {
		if purpose == githubapp.PurposeGitRead &&
			(errors.Is(err, githubapp.ErrOwnerNotAllowed) || errors.Is(err, githubapp.ErrInstallationNotFound)) {
			return gitAuthentication{}, nil
		}
		return gitAuthentication{}, err
	}
	return gitAuthentication{token: token}, nil
}

func requireRemoteWrite(ctxInfo *repoContext, operation string) error {
	if ctxInfo.Archived {
		return fmt.Errorf("archived repository is read-only: refusing %s", operation)
	}
	if ctxInfo.Provider == gitprovider.ProviderGeneric {
		return fmt.Errorf("generic HTTPS repository is read-only: refusing %s", operation)
	}
	return nil
}

func requireGitPushTarget(ctxInfo *repoContext, operation string) error {
	if err := validateCurrentRemoteTargets(ctxInfo, true); err != nil {
		return fmt.Errorf("refusing %s: %w", operation, err)
	}
	return nil
}

func runGitWithCredsImpl(ctxInfo *repoContext, auth gitAuthentication, args ...string) error {
	ctx := operationContext(ctxInfo)
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ctxInfo.WorkDir}, args...)...)
	switch ctxInfo.Provider {
	case gitprovider.ProviderGitHub:
		cmd.Env = gitutil.GitHubAppGitEnv(
			os.Environ(), ctxInfo.RemoteURL, ctxInfo.Owner, ctxInfo.Repo, auth.token,
		)
	case gitprovider.ProviderGeneric:
		cmd.Env = gitutil.AnonymousGitEnv(os.Environ())
	default:
		cmd.Env = gitutil.ForgejoGitEnv(os.Environ(), auth.token)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf(
			"git %s: %w: %s",
			strings.Join(args, " "), err, redactSecret(string(out), auth.token),
		)
	}
	return nil
}

func operationContext(ctxInfo *repoContext) context.Context {
	if ctxInfo != nil && ctxInfo.Context != nil {
		return ctxInfo.Context
	}
	return context.Background()
}

func redactSecret(value, secret string) string {
	value = strings.TrimSpace(value)
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

func confirmedGitAuthenticationFailure(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "authentication failed") ||
		strings.Contains(message, "http 401") ||
		strings.Contains(message, "http 403: bad credentials") ||
		strings.Contains(message, "requested url returned error: 403")
}

func defaultBranch(ctx context.Context, workDir string) (string, bool) {
	out, err := gitOutput(ctx, workDir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		if _, branch, ok := strings.Cut(out, "origin/"); ok && branch != "" {
			return branch, true
		}
	}
	return branchMain, false
}

func latestTag(ctx context.Context, workDir string) (string, error) {
	out, err := gitOutput(ctx, workDir, "tag", "--sort=-version:refname")
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
	latest, err := latestTag(operationContext(ctxInfo), ctxInfo.WorkDir)
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
	if err := runGit(operationContext(ctxInfo), ctxInfo.WorkDir, "remote", "get-url", remoteOrigin); err != nil {
		return true, nil
	}
	ref := "refs/tags/" + tag
	if err := runGitWithCreds(
		ctxInfo, githubapp.PurposeGitRead, "ls-remote", "--exit-code", "--tags", remoteOrigin, ref,
	); err != nil {
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

func localTagExists(ctx context.Context, workDir, tag string) bool {
	err := runGit(ctx, workDir, "show-ref", "--verify", "--quiet", "refs/tags/"+tag)
	return err == nil
}

func ensureCleanBranchForCleanup(ctxInfo *repoContext, allowMissingRemote bool) error {
	ctx := operationContext(ctxInfo)
	out, err := gitOutput(ctx, ctxInfo.WorkDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot verify worktree is clean: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("refusing closed PR branch cleanup: worktree has uncommitted changes")
	}
	if err := runGitWithCreds(ctxInfo, githubapp.PurposeGitRead, "fetch", "--prune", remoteOrigin); err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot refresh origin: %w", err)
	}
	remoteRef := "refs/remotes/" + remoteOrigin + "/" + ctxInfo.Branch
	if err := runGit(ctx, ctxInfo.WorkDir, "show-ref", "--verify", "--quiet", remoteRef); err != nil {
		if allowMissingRemote {
			return nil
		}
		return fmt.Errorf(
			"refusing closed PR branch cleanup: remote branch is missing; local branch may be the only remaining ref",
		)
	}
	compareRef := remoteOrigin + "/" + ctxInfo.Branch + "..." + ctxInfo.Branch
	ahead, err := gitOutput(ctx, ctxInfo.WorkDir, "rev-list", "--right-only", "--count", compareRef)
	if err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot check local commits: %w", err)
	}
	if strings.TrimSpace(ahead) != "0" {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: %s has %s local commit(s) not on origin/%s",
			ctxInfo.Branch,
			strings.TrimSpace(ahead),
			ctxInfo.Branch,
		)
	}
	return nil
}

func cleanupClosedPRBranch(ctxInfo *repoContext, prMerged bool) error {
	if err := requireGitPushTarget(ctxInfo, "closed PR branch cleanup"); err != nil {
		return err
	}
	if err := ensureCleanBranchForCleanup(ctxInfo, prMerged); err != nil {
		return err
	}
	ctx := operationContext(ctxInfo)
	remoteExists := remoteBranchExists(ctxInfo)
	if err := runGit(ctx, ctxInfo.WorkDir, "switch", ctxInfo.DefaultBase); err != nil {
		return err
	}
	if err := runGitWithCreds(
		ctxInfo, githubapp.PurposeGitRead, "pull", "--ff-only", remoteOrigin, ctxInfo.DefaultBase,
	); err != nil {
		return err
	}
	if remoteExists {
		if err := runGitWithCreds(
			ctxInfo, githubapp.PurposeGitWrite, "push", remoteOrigin, "--delete", ctxInfo.Branch,
		); err != nil {
			return err
		}
	}
	return runGit(ctx, ctxInfo.WorkDir, "branch", "-D", ctxInfo.Branch)
}

func remoteBranchExists(ctxInfo *repoContext) bool {
	remoteRef := "refs/remotes/" + remoteOrigin + "/" + ctxInfo.Branch
	return runGit(operationContext(ctxInfo), ctxInfo.WorkDir, "show-ref", "--verify", "--quiet", remoteRef) == nil
}
