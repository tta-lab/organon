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
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
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

// branchCleanupTarget is the identity that a guarded cleanup is allowed to
// remove. HeadSHA is optional for the legacy closed-PR pull workflow; an
// automatic merge always supplies it and requires exact ref matches.
type branchCleanupTarget struct {
	BaseBranch          string
	HeadBranch          string
	HeadSHA             string
	AllowMissingRemote  bool
	AllowMissingLocal   bool
	RequireExactHeadSHA bool
}

type branchCleanupRefs struct {
	LocalExists  bool
	RemoteExists bool
}

// ensureBranchCleanupTarget performs the non-destructive checks shared by
// closed-PR cleanup and automatic approved-merge cleanup. It fetches origin so
// remote-tracking refs are current, then verifies the worktree and target refs
// before any switch, pull, or deletion is attempted.
func ensureBranchCleanupTarget(ctxInfo *repoContext, target branchCleanupTarget) error {
	if err := validateBranchCleanupTarget(ctxInfo, target); err != nil {
		return err
	}
	if err := ensureCleanupWorktreeClean(ctxInfo); err != nil {
		return err
	}
	if err := refreshCleanupRemote(ctxInfo); err != nil {
		return err
	}
	if err := ensureCleanupDefaultRef(ctxInfo, target); err != nil {
		return err
	}
	refs, err := verifyBranchCleanupRefs(ctxInfo, target)
	if err != nil {
		return err
	}
	return validateCleanupRefs(ctxInfo, target, refs)
}

func ensureCleanupWorktreeClean(ctxInfo *repoContext) error {
	ctx := operationContext(ctxInfo)
	out, err := gitOutput(ctx, ctxInfo.WorkDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot verify worktree is clean: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("refusing closed PR branch cleanup: worktree has uncommitted changes")
	}
	return nil
}

func refreshCleanupRemote(ctxInfo *repoContext) error {
	if err := runGitWithCreds(ctxInfo, githubapp.PurposeGitRead, "fetch", "--prune", remoteOrigin); err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot refresh origin: %w", err)
	}
	return nil
}

func ensureCleanupDefaultRef(ctxInfo *repoContext, target branchCleanupTarget) error {
	ctx := operationContext(ctxInfo)
	defaultRef := "refs/remotes/" + remoteOrigin + "/" + target.BaseBranch
	if _, exists, err := readOptionalRef(ctx, ctxInfo.WorkDir, defaultRef); err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: cannot inspect default branch: %w", err)
	} else if !exists {
		return fmt.Errorf("refusing closed PR branch cleanup: origin default branch %q is missing", target.BaseBranch)
	}
	return nil
}

func validateCleanupRefs(ctxInfo *repoContext, target branchCleanupTarget, refs branchCleanupRefs) error {
	if err := rejectUnpushedCleanupCommits(ctxInfo, target, refs); err != nil {
		return err
	}
	if !refs.RemoteExists && !target.AllowMissingRemote {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: remote branch is missing; local branch may be the only remaining ref",
		)
	}
	if !refs.LocalExists && !target.AllowMissingLocal {
		return fmt.Errorf("refusing closed PR branch cleanup: local branch is missing")
	}
	return nil
}

func rejectUnpushedCleanupCommits(
	ctxInfo *repoContext, target branchCleanupTarget, refs branchCleanupRefs,
) error {
	if target.RequireExactHeadSHA || !refs.RemoteExists {
		return nil
	}
	ctx := operationContext(ctxInfo)
	compareRef := remoteOrigin + "/" + target.HeadBranch + "..." + target.HeadBranch
	ahead, compareErr := gitOutput(ctx, ctxInfo.WorkDir, "rev-list", "--right-only", "--count", compareRef)
	if compareErr != nil {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: cannot check local commits: %w", compareErr,
		)
	}
	if strings.TrimSpace(ahead) != "0" {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: %s has %s local commit(s) not on origin/%s",
			target.HeadBranch, strings.TrimSpace(ahead), target.HeadBranch,
		)
	}
	return nil
}

func validateBranchCleanupTarget(ctxInfo *repoContext, target branchCleanupTarget) error {
	if ctxInfo == nil {
		return fmt.Errorf("refusing closed PR branch cleanup: repository context is missing")
	}
	if target.BaseBranch == "" || target.HeadBranch == "" {
		return fmt.Errorf("refusing closed PR branch cleanup: branch target is incomplete")
	}
	if err := validateBranchName(operationContext(ctxInfo), ctxInfo.WorkDir, target.BaseBranch); err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: invalid default branch %q: %w", target.BaseBranch, err)
	}
	if err := validateBranchName(operationContext(ctxInfo), ctxInfo.WorkDir, target.HeadBranch); err != nil {
		return fmt.Errorf("refusing closed PR branch cleanup: invalid head branch %q: %w", target.HeadBranch, err)
	}
	if target.BaseBranch == target.HeadBranch {
		return fmt.Errorf("refusing closed PR branch cleanup: refusing to delete the default branch %q", target.BaseBranch)
	}
	if err := validateExactBranchCleanupTarget(ctxInfo, target); err != nil {
		return err
	}
	if ctxInfo.DefaultBaseKnown && ctxInfo.DefaultBase != target.BaseBranch {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: approved base branch %q does not match checkout default %q",
			target.BaseBranch, ctxInfo.DefaultBase,
		)
	}
	if ctxInfo.Branch != target.HeadBranch && ctxInfo.Branch != target.BaseBranch {
		return fmt.Errorf(
			"refusing closed PR branch cleanup: current branch %q is neither approved head %q nor default %q",
			ctxInfo.Branch, target.HeadBranch, target.BaseBranch,
		)
	}
	return nil
}

func validateExactBranchCleanupTarget(ctxInfo *repoContext, target branchCleanupTarget) error {
	if !target.RequireExactHeadSHA {
		return nil
	}
	if strings.TrimSpace(target.HeadSHA) == "" {
		return fmt.Errorf("refusing closed PR branch cleanup: approved head SHA is missing")
	}
	if strings.TrimSpace(target.HeadSHA) != target.HeadSHA {
		return fmt.Errorf("refusing closed PR branch cleanup: approved head SHA is invalid")
	}
	if !ctxInfo.DefaultBaseKnown {
		return fmt.Errorf("refusing automatic merge cleanup: checkout default branch is unknown")
	}
	return nil
}

func validateBranchName(ctx context.Context, workDir, branch string) error {
	if strings.TrimSpace(branch) == "" || strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch name is empty or starts with '-'")
	}
	if err := runGit(ctx, workDir, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("branch name is not a valid Git ref")
	}
	return nil
}

func verifyBranchCleanupRefs(
	ctxInfo *repoContext, target branchCleanupTarget,
) (branchCleanupRefs, error) {
	ctx := operationContext(ctxInfo)
	localRef := "refs/heads/" + target.HeadBranch
	remoteRef := "refs/remotes/" + remoteOrigin + "/" + target.HeadBranch
	localSHA, localExists, err := readOptionalRef(ctx, ctxInfo.WorkDir, localRef)
	if err != nil {
		return branchCleanupRefs{}, fmt.Errorf("refusing closed PR branch cleanup: cannot inspect local head: %w", err)
	}
	remoteSHA, remoteExists, err := readOptionalRef(ctx, ctxInfo.WorkDir, remoteRef)
	if err != nil {
		return branchCleanupRefs{}, fmt.Errorf("refusing closed PR branch cleanup: cannot inspect origin head: %w", err)
	}
	if target.RequireExactHeadSHA {
		if localExists && !sameGitObjectID(localSHA, target.HeadSHA) {
			return branchCleanupRefs{}, fmt.Errorf(
				"refusing automatic merge cleanup: local head %q moved from approved SHA", target.HeadBranch,
			)
		}
		if remoteExists && !sameGitObjectID(remoteSHA, target.HeadSHA) {
			return branchCleanupRefs{}, fmt.Errorf(
				"refusing automatic merge cleanup: origin/%s moved from approved SHA", target.HeadBranch,
			)
		}
	}
	return branchCleanupRefs{LocalExists: localExists, RemoteExists: remoteExists}, nil
}

func readOptionalRef(ctx context.Context, workDir, ref string) (string, bool, error) {
	err := runGit(ctx, workDir, "show-ref", "--verify", "--quiet", ref)
	if err != nil {
		if exitCode(err) == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	sha, err := gitOutput(ctx, workDir, "rev-parse", "--verify", ref)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(sha) == "" {
		return "", false, fmt.Errorf("empty ref value")
	}
	return strings.TrimSpace(sha), true, nil
}

func sameGitObjectID(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func cleanupClosedPRBranch(ctxInfo *repoContext, prMerged bool) error {
	if err := requireGitPushTarget(ctxInfo, "closed PR branch cleanup"); err != nil {
		return err
	}
	return cleanupBranch(ctxInfo, branchCleanupTarget{
		BaseBranch:         ctxInfo.DefaultBase,
		HeadBranch:         ctxInfo.Branch,
		AllowMissingRemote: prMerged,
	})
}

// cleanupApprovedMergeBranch applies the same guarded policy to an approved
// PR snapshot. Every present ref must still point at the approved head SHA;
// missing refs are already-cleaned state and are skipped.
func cleanupApprovedMergeBranch(ctxInfo *repoContext, approved PRMergeSnapshot) error {
	if ctxInfo == nil {
		return fmt.Errorf("refusing automatic merge cleanup: repository context is missing")
	}
	if err := requireGitPushTarget(ctxInfo, "automatic merge cleanup"); err != nil {
		return err
	}
	target, err := approvedMergeCleanupTarget(ctxInfo, approved)
	if err != nil {
		return err
	}
	return cleanupBranch(ctxInfo, target)
}

func approvedMergeCleanupTarget(
	ctxInfo *repoContext, approved PRMergeSnapshot,
) (branchCleanupTarget, error) {
	if ctxInfo == nil {
		return branchCleanupTarget{}, fmt.Errorf("refusing automatic merge cleanup: repository context is missing")
	}
	if !mergeSnapshotRepositoryMatches(ctxInfo, approved) {
		return branchCleanupTarget{}, fmt.Errorf(
			"refusing automatic merge cleanup: approved repository does not match checkout",
		)
	}
	target := branchCleanupTarget{
		BaseBranch:          approved.BaseBranch,
		HeadBranch:          approved.Head,
		HeadSHA:             approved.HeadSHA,
		AllowMissingRemote:  true,
		AllowMissingLocal:   true,
		RequireExactHeadSHA: true,
	}
	if target.BaseBranch != ctxInfo.DefaultBase {
		return branchCleanupTarget{}, fmt.Errorf(
			"refusing automatic merge cleanup: approved base branch %q does not match checkout default %q",
			target.BaseBranch, ctxInfo.DefaultBase,
		)
	}
	if err := validateBranchCleanupTarget(ctxInfo, target); err != nil {
		return branchCleanupTarget{}, err
	}
	return target, nil
}

func cleanupBranch(ctxInfo *repoContext, target branchCleanupTarget) error {
	if err := ensureBranchCleanupTarget(ctxInfo, target); err != nil {
		return err
	}
	if err := transitionCleanupBranch(ctxInfo, target); err != nil {
		return err
	}
	refs, err := verifyBranchCleanupRefs(ctxInfo, target)
	if err != nil {
		return err
	}
	if err := deleteCleanupRemote(ctxInfo, target, refs); err != nil {
		return err
	}
	return deleteCleanupLocal(ctxInfo, target)
}

func transitionCleanupBranch(ctxInfo *repoContext, target branchCleanupTarget) error {
	ctx := operationContext(ctxInfo)
	if ctxInfo.Branch != target.BaseBranch {
		if err := runGit(ctx, ctxInfo.WorkDir, "switch", "--", target.BaseBranch); err != nil {
			return err
		}
	}
	if err := runGitWithCreds(
		ctxInfo, githubapp.PurposeGitRead, "pull", "--ff-only", remoteOrigin, target.BaseBranch,
	); err != nil {
		return err
	}
	return nil
}

func deleteCleanupRemote(
	ctxInfo *repoContext, target branchCleanupTarget, refs branchCleanupRefs,
) error {
	ctx := operationContext(ctxInfo)
	if !refs.RemoteExists {
		return nil
	}
	if target.RequireExactHeadSHA {
		lease := "--force-with-lease=refs/heads/" + target.HeadBranch + ":" + target.HeadSHA
		if err := runGitWithCreds(
			ctxInfo, githubapp.PurposeGitWrite, "push", remoteOrigin, "--delete", lease, "--", target.HeadBranch,
		); err != nil {
			return err
		}
	} else if err := runGitWithCreds(
		ctxInfo, githubapp.PurposeGitWrite, "push", remoteOrigin, "--delete", target.HeadBranch,
	); err != nil {
		return err
	}
	if target.RequireExactHeadSHA {
		_, stillExists, err := readOptionalRef(
			ctx, ctxInfo.WorkDir, "refs/remotes/"+remoteOrigin+"/"+target.HeadBranch,
		)
		if err != nil {
			return fmt.Errorf("refusing automatic merge cleanup: cannot verify origin head deletion: %w", err)
		}
		if stillExists {
			return fmt.Errorf("refusing automatic merge cleanup: origin/%s still exists after deletion", target.HeadBranch)
		}
	}
	return nil
}

func deleteCleanupLocal(ctxInfo *repoContext, target branchCleanupTarget) error {
	ctx := operationContext(ctxInfo)
	latest, exists, err := readOptionalRef(ctx, ctxInfo.WorkDir, "refs/heads/"+target.HeadBranch)
	if err != nil {
		return fmt.Errorf("refusing automatic merge cleanup: cannot recheck local head: %w", err)
	}
	if target.RequireExactHeadSHA && exists && !sameGitObjectID(latest, target.HeadSHA) {
		return fmt.Errorf("refusing automatic merge cleanup: local head %q moved from approved SHA", target.HeadBranch)
	}
	if exists {
		return runGit(ctx, ctxInfo.WorkDir, "branch", "-D", "--", target.HeadBranch)
	}
	return nil
}
