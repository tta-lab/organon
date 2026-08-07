package og

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/gitutil"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

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
	if err := validateCloneRequest(req); err != nil {
		return Response{}, err
	}
	source, registered, err := s.resolveCloneRequest(req)
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

func validateCloneRequest(req Request) error {
	hasProject := strings.TrimSpace(req.Project) != ""
	hasURL := strings.TrimSpace(req.URL) != ""
	if hasProject == hasURL {
		return fmt.Errorf("exactly one of project and URL is required")
	}
	if hasProject && (req.Alias != "" || req.Reference) {
		return fmt.Errorf("project clone does not accept alias or reference")
	}
	if req.Reference && req.Alias != "" {
		return fmt.Errorf("reference clone does not accept alias")
	}
	return nil
}

func (s Service) resolveCloneRequest(req Request) (cloneSource, project.Entry, error) {
	if req.Project != "" {
		entry, err := s.projectStore().Get(req.Project)
		if err != nil {
			return cloneSource{}, project.Entry{}, err
		}
		source, err := s.resolveCloneSource(entry.Remote, false)
		if err != nil {
			return cloneSource{}, project.Entry{}, err
		}
		source.destination = entry.Path
		return source, entry, nil
	}
	source, err := s.resolveCloneSource(req.URL, req.Reference)
	if err != nil {
		return cloneSource{}, project.Entry{}, err
	}
	entry, err := s.cloneRegistration(req, &source)
	return source, entry, err
}

func requestContext(req Request) context.Context {
	if req.Context != nil {
		return req.Context
	}
	return context.Background()
}

func (s Service) cloneRegistration(req Request, source *cloneSource) (project.Entry, error) {
	if req.Reference {
		return project.Entry{}, nil
	}
	entry, err := s.projectStore().GetByRemote(source.remote)
	if err == nil {
		if req.Alias != "" && req.Alias != entry.Alias {
			return project.Entry{}, fmt.Errorf(
				"project alias %q conflicts with registered alias %q for remote %q",
				req.Alias, entry.Alias, source.remote,
			)
		}
		source.destination = entry.Path
		return entry, nil
	}
	if !errors.Is(err, project.ErrNotFound) {
		return project.Entry{}, err
	}
	if existing, pathErr := s.projectStore().GetByPath(source.destination); pathErr == nil {
		return project.Entry{}, fmt.Errorf(
			"project path %q conflicts with alias %q and remote %q",
			source.destination, existing.Alias, existing.Remote,
		)
	} else if !errors.Is(pathErr, project.ErrNotFound) {
		return project.Entry{}, pathErr
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
	return project.Entry{Alias: alias, Path: source.destination, Remote: source.remote}, nil
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
	info, err := gitprovider.ParseHTTPRemoteURL(raw)
	if err != nil {
		return cloneSource{}, err
	}
	provider, err := s.config.ClassifyRemote(info)
	if err != nil {
		return cloneSource{}, fmt.Errorf("classify clone URL: %w", err)
	}
	info.Provider = provider
	remote := info.CanonicalURL
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cloneSource{}, fmt.Errorf("resolve home directory: %w", err)
	}
	hostDir := strings.TrimPrefix(strings.TrimPrefix(info.BaseURL, "https://"), "http://")
	hostDir = strings.NewReplacer(":", "_", "[", "", "]", "").Replace(hostDir)
	base := filepath.Join(home, "code", "projects")
	if reference {
		base = filepath.Join(home, "code", "references", hostDir)
	}
	return cloneSource{
		info: info, remote: remote, host: strings.TrimPrefix(strings.TrimPrefix(info.BaseURL, "https://"), "http://"),
		provider: provider, destination: filepath.Join(base, info.Owner, info.Repo),
	}, nil
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
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create clone destination parent: %w", err)
	}
	return nil
}

func (s Service) validateExistingClone(source cloneSource) (bool, error) {
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
	root, err := gitOutput(directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("validate cloned repository: %w", err)
	}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realDirectory, directoryErr := filepath.EvalSymlinks(directory)
	if rootErr != nil || directoryErr != nil || filepath.Clean(realRoot) != filepath.Clean(realDirectory) {
		return fmt.Errorf("cloned repository top-level %q does not match destination %q", root, directory)
	}
	remote, err := controlledGitOutput(directory, "remote", "get-url", remoteOrigin)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	provider, err := ogconfigForSource(source).ClassifyRemote(info)
	if err != nil {
		return fmt.Errorf("validate cloned origin: %w", err)
	}
	if provider != source.provider || info.CanonicalURL != source.remote {
		return fmt.Errorf("existing origin %q does not match requested repository %q", remote, source.remote)
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
		return fmt.Errorf("git clone: %w: %s", err, redactSecret(string(out), invocation.Token))
	}
	return nil
}
