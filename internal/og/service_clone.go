package og

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

var cloneSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type cloneSource struct {
	info        *gitprovider.RepoInfo
	remote      string
	host        string
	provider    gitprovider.ProviderType
	destination string
}

type cloneInvocation struct {
	Destination string
	Remote      string
	Provider    string
	Owner       string
	Repo        string
	Token       string
}

var runGitCloneFunc = runGitClone

// GitClone clones one URL into its daemon-owned derived path.
func (s Service) GitClone(req Request) (Response, error) {
	ctx := requestContext(req)
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	source, err := s.resolveCloneSource(req.URL, req.Reference)
	if err != nil {
		return Response{}, err
	}
	registered, err := s.cloneRegistration(req, source)
	if err != nil {
		return Response{}, err
	}

	result := cloneResultFor(source, registered, !req.Reference)
	alreadyExists, err := s.validateExistingClone(source)
	if err != nil {
		return Response{}, err
	}
	if alreadyExists {
		result.AlreadyExisted = true
		if err := s.completeCloneRegistration(ctx, req.Reference, registered, &result); err != nil {
			return Response{}, err
		}
		return Response{Clone: &result}, nil
	}
	return s.cloneNew(ctx, req.Reference, source, registered, result)
}

func requestContext(req Request) context.Context {
	if req.Context != nil {
		return req.Context
	}
	return context.Background()
}

func (s Service) cloneRegistration(req Request, source cloneSource) (project.Entry, error) {
	if req.Reference {
		if req.Alias != "" {
			return project.Entry{}, fmt.Errorf("--alias cannot be used with a reference clone")
		}
		return project.Entry{}, nil
	}
	entry, err := s.projectStore().GetByPath(source.destination)
	if err == nil {
		return entry, nil
	}
	if !errors.Is(err, project.ErrNotFound) {
		return project.Entry{}, err
	}
	alias := req.Alias
	if alias == "" {
		alias = source.info.Repo
	}
	if err := project.ValidateAlias(alias); err != nil {
		return project.Entry{}, err
	}
	if existing, err := s.projectStore().Get(alias); err == nil {
		return project.Entry{}, fmt.Errorf(
			"project alias %q already uses path %q", existing.Alias, existing.Path,
		)
	} else if !errors.Is(err, project.ErrNotFound) {
		return project.Entry{}, err
	}
	return project.Entry{Alias: alias, Path: source.destination}, nil
}

func (s Service) cloneNew(
	ctx context.Context,
	reference bool,
	source cloneSource,
	registered project.Entry,
	result CloneResult,
) (Response, error) {

	if err := ensureCloneParent(source.destination); err != nil {
		return Response{}, err
	}
	temp, err := os.MkdirTemp(filepath.Dir(source.destination), "."+source.info.Repo+".clone-")
	if err != nil {
		return Response{}, fmt.Errorf("create temporary clone directory: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(temp)
		}
	}()

	auth, err := s.cloneAuthentication(ctx, source)
	if err != nil {
		return Response{}, err
	}
	if err := runGitCloneFunc(ctx, cloneInvocation{
		Destination: temp, Remote: source.remote,
		Provider: string(source.provider), Owner: source.info.Owner, Repo: source.info.Repo,
		Token: auth.token,
	}); err != nil {
		return Response{}, err
	}
	if err := validateClonedRepository(temp, source); err != nil {
		return Response{}, err
	}
	if _, err := os.Lstat(source.destination); err == nil {
		return Response{}, fmt.Errorf("clone destination %q appeared during clone", source.destination)
	} else if !os.IsNotExist(err) {
		return Response{}, fmt.Errorf("inspect clone destination: %w", err)
	}
	if err := os.Rename(temp, source.destination); err != nil {
		return Response{}, fmt.Errorf("complete clone: %w", err)
	}
	completed = true

	if err := s.completeCloneRegistration(ctx, reference, registered, &result); err != nil {
		return Response{}, err
	}
	return Response{Clone: &result}, nil
}

func (s Service) completeCloneRegistration(
	ctx context.Context,
	reference bool,
	registered project.Entry,
	result *CloneResult,
) error {
	if reference {
		return nil
	}
	entry := registered
	if !registered.Archived {
		var err error
		entry, _, err = s.projectStore().Register(ctx, registered)
		if err != nil {
			return err
		}
	}
	result.Alias = entry.Alias
	result.Archived = entry.Archived
	result.Registered = true
	return nil
}

func (s Service) resolveCloneSource(raw string, reference bool) (cloneSource, error) {
	u, owner, repo, err := parseCloneRepositoryURL(raw)
	if err != nil {
		return cloneSource{}, err
	}
	baseURL, err := ogconfig.NormalizeBaseURL(strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host))
	if err != nil {
		return cloneSource{}, fmt.Errorf("normalize clone URL: %w", err)
	}
	normalizedURL, err := url.Parse(baseURL)
	if err != nil {
		return cloneSource{}, fmt.Errorf("parse normalized clone URL: %w", err)
	}
	info := &gitprovider.RepoInfo{Owner: owner, Repo: repo, Host: normalizedURL.Hostname(), BaseURL: baseURL}
	provider, err := s.config.ClassifyRemote(info)
	if err != nil {
		return cloneSource{}, fmt.Errorf("classify clone URL: %w", err)
	}
	info.Provider = provider
	remote := baseURL + "/" + owner + "/" + repo + ".git"
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cloneSource{}, fmt.Errorf("resolve home directory: %w", err)
	}
	hostDir := strings.NewReplacer(":", "_", "[", "", "]", "").Replace(normalizedURL.Host)
	base := filepath.Join(home, "code", "projects")
	if reference {
		base = filepath.Join(home, "code", "references", hostDir)
	}
	return cloneSource{
		info: info, remote: remote, host: normalizedURL.Host,
		provider: provider, destination: filepath.Join(base, owner, repo),
	}, nil
}

func parseCloneRepositoryURL(raw string) (*url.URL, string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid clone URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", "", fmt.Errorf("clone URL must use HTTP(S)")
	}
	if u.User != nil {
		return nil, "", "", fmt.Errorf("clone URL must not contain credentials")
	}
	if u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", "", fmt.Errorf("clone URL must contain a host and no query or fragment")
	}
	if strings.Contains(u.EscapedPath(), "%") {
		return nil, "", "", fmt.Errorf("clone URL path must not contain escaped characters")
	}
	if !validCloneURLPath(u.Path) {
		return nil, "", "", fmt.Errorf("clone URL path must be exactly /owner/repo")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if !safeCloneSegment(owner) || !safeCloneSegment(repo) {
		return nil, "", "", fmt.Errorf("clone URL contains an unsafe owner or repository name")
	}
	return u, owner, repo, nil
}

func validCloneURLPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.HasSuffix(path, "/") {
		return false
	}
	return len(strings.Split(strings.TrimPrefix(path, "/"), "/")) == 2
}

func safeCloneSegment(segment string) bool {
	return segment != "." && segment != ".." && cloneSegmentPattern.MatchString(segment)
}

func cloneResultFor(source cloneSource, entry project.Entry, registered bool) CloneResult {
	return CloneResult{
		Alias: entry.Alias, Path: source.destination, Host: source.host,
		Owner: source.info.Owner, Repo: source.info.Repo, Provider: string(source.provider),
		Remote: source.remote, Registered: registered, Archived: entry.Archived,
	}
}

func (s Service) cloneAuthentication(ctx context.Context, source cloneSource) (gitAuthentication, error) {
	switch source.provider {
	case gitprovider.ProviderGeneric:
		return gitAuthentication{}, nil
	case gitprovider.ProviderForgejo:
		return gitAuthentication{token: os.Getenv(gitutil.ForgeTokenEnv())}, nil
	case gitprovider.ProviderGitHub:
		if s.githubBroker == nil {
			return gitAuthentication{}, nil
		}
		token, err := s.githubBroker.Token(ctx, source.info.Owner, source.info.Repo, githubapp.PurposeGitRead)
		if err == nil {
			return gitAuthentication{token: token}, nil
		}
		if errors.Is(err, githubapp.ErrOwnerNotAllowed) || errors.Is(err, githubapp.ErrInstallationNotFound) {
			return gitAuthentication{}, nil
		}
		return gitAuthentication{}, err
	default:
		return gitAuthentication{}, fmt.Errorf("unsupported clone provider %q", source.provider)
	}
}

func ensureCloneParent(destination string) error {
	parent := filepath.Dir(destination)
	if err := validateCloneParentChain(parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create clone destination parent: %w", err)
	}
	return validateCloneParentChain(parent)
}

func validateCloneParentChain(parent string) error {
	for current := parent; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("clone destination parent %q is a symlink", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("clone destination parent %q is not a directory", current)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect clone destination parent: %w", err)
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
	}
	return nil
}

func (s Service) validateExistingClone(source cloneSource) (bool, error) {
	if err := validateCloneParentChain(filepath.Dir(source.destination)); err != nil {
		return false, err
	}
	info, err := os.Lstat(source.destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect clone destination: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("clone destination %q is not a safe directory", source.destination)
	}
	if err := validateClonedRepository(source.destination, source); err != nil {
		return false, err
	}
	return true, nil
}

func validateClonedRepository(directory string, source cloneSource) error {
	if err := validateLocalGitControl(directory); err != nil {
		return err
	}
	root, err := gitOutput(directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("validate cloned repository: %w", err)
	}
	if filepath.Clean(root) != filepath.Clean(directory) {
		return fmt.Errorf("cloned repository top-level %q does not match destination %q", root, directory)
	}
	remote, err := controlledGitOutput(directory, "remote", "get-url", remoteOrigin)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	u, owner, repo, err := parseCloneRepositoryURL(remote)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	rawBaseURL := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	normalizedBaseURL, err := ogconfig.NormalizeBaseURL(rawBaseURL)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	info := &gitprovider.RepoInfo{
		Owner: owner, Repo: repo, Host: strings.ToLower(u.Hostname()), BaseURL: normalizedBaseURL,
	}
	provider, err := ogconfigForSource(source).ClassifyRemote(info)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	if provider != source.provider || !strings.EqualFold(info.Host, source.info.Host) ||
		info.Owner != source.info.Owner || info.Repo != source.info.Repo ||
		!strings.EqualFold(normalizedBaseURL, source.info.BaseURL) {
		return fmt.Errorf("existing origin %q does not match requested repository %q", remote, source.remote)
	}
	return nil
}

func validateLocalGitControl(directory string) error {
	gitControlPath := filepath.Join(directory, ".git")
	gitControl, err := os.Lstat(gitControlPath)
	if err != nil {
		return fmt.Errorf("validate git control directory: %w", err)
	}
	if gitControl.Mode()&os.ModeSymlink != 0 || !gitControl.IsDir() {
		return fmt.Errorf("git control directory %q must be a local directory", gitControlPath)
	}
	absoluteGitDir, err := controlledGitOutput(directory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return fmt.Errorf("validate git control directory: %w", err)
	}
	if filepath.Clean(absoluteGitDir) != filepath.Clean(gitControlPath) {
		return fmt.Errorf("git control directory %q escapes clone destination", absoluteGitDir)
	}
	absoluteCommonDir, err := controlledGitOutput(directory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("validate git control directory: %w", err)
	}
	if filepath.Clean(absoluteCommonDir) != filepath.Clean(gitControlPath) {
		return fmt.Errorf("git control directory %q escapes clone destination", absoluteCommonDir)
	}
	return nil
}

func ogconfigForSource(source cloneSource) ogconfig.Config {
	cfg := ogconfig.Config{}
	if source.provider == gitprovider.ProviderForgejo {
		cfg.Forgejo.AllowedBaseURLs = []string{source.info.BaseURL}
	}
	return cfg
}

func runGitClone(ctx context.Context, invocation cloneInvocation) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		ctx, "git", "clone", "--origin", remoteOrigin, "--", invocation.Remote, invocation.Destination,
	)
	switch gitprovider.ProviderType(invocation.Provider) {
	case gitprovider.ProviderGitHub:
		cmd.Env = gitutil.GitHubAppGitEnv(
			os.Environ(), invocation.Remote, invocation.Owner, invocation.Repo, invocation.Token,
		)
	case gitprovider.ProviderForgejo:
		if invocation.Token == "" {
			cmd.Env = gitutil.AnonymousGitEnv(os.Environ())
		} else {
			cmd.Env = gitutil.ForgejoGitEnv(os.Environ(), invocation.Token)
		}
	default:
		cmd.Env = gitutil.AnonymousGitEnv(os.Environ())
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ctx.Err()
		}
		return fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
