package og

import (
	"fmt"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

// Service owns configured dependencies shared across direct operations.
type Service struct {
	githubBroker githubapp.CredentialBroker
	projects     *project.Store
	config       ogconfig.Config
}

// NewService constructs a service with the default project registry. A nil broker
// leaves GitHub App authentication unavailable while preserving non-GitHub operations.
func NewService(githubBroker githubapp.CredentialBroker) Service {
	return NewServiceWithConfig(
		githubBroker, project.NewStore(config.ProjectsPath()), ogconfig.Config{},
	)
}

// NewServiceWithConfig constructs a service with an explicit hot registry and trust policy.
func NewServiceWithConfig(
	githubBroker githubapp.CredentialBroker,
	projects *project.Store,
	cfg ogconfig.Config,
) Service {
	return Service{githubBroker: githubBroker, projects: projects, config: cfg}
}

// GitHubAppConfigured reports whether this service has GitHub App credentials.
func (s Service) GitHubAppConfigured() bool {
	return s.githubBroker != nil
}

// Validate checks configured project and remote policy without network operations.
func (s Service) Validate() error {
	catalog, err := s.projectStore().Snapshot()
	if err != nil {
		return err
	}
	for _, entry := range catalog.ListAll(true) {
		info, parseErr := gitprovider.ParseHTTPRemoteURL(entry.Remote)
		if parseErr != nil {
			return fmt.Errorf("project %q remote: %w", entry.Alias, parseErr)
		}
		if _, classifyErr := s.config.ClassifyRemote(info); classifyErr != nil {
			return fmt.Errorf("project %q remote: %w", entry.Alias, classifyErr)
		}
	}
	return nil
}

func (s Service) resolveRepoContextForRequest(req Request) (*repoContext, error) {
	ctx, err := s.resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return nil, err
	}
	bindRequestContext(ctx, req)
	return ctx, nil
}

func (s Service) resolveRemoteRepoContextForRequest(req Request) (*repoContext, error) {
	ctx, err := s.resolveRemoteRepoContextFor(req.WorkDir)
	if err != nil {
		return nil, err
	}
	bindRequestContext(ctx, req)
	return ctx, nil
}

func bindRequestContext(ctx *repoContext, req Request) {
	ctx.Context = requestContext(req)
}

func (s Service) resolveRepoContextFor(workDir string) (*repoContext, error) {
	ctx, err := resolveRepoContextWith(workDir, s.projectStore(), s.config)
	if err != nil {
		return nil, err
	}
	ctx.githubBroker = s.githubBroker
	return ctx, nil
}

func (s Service) resolveRemoteRepoContextFor(workDir string) (*repoContext, error) {
	ctx, err := resolveRemoteRepoContextWith(workDir, s.projectStore(), s.config)
	if err != nil {
		return nil, err
	}
	ctx.githubBroker = s.githubBroker
	return ctx, nil
}

// ProjectStore returns the hot registry used by this service.
func (s Service) ProjectStore() *project.Store {
	return s.projectStore()
}

func (s Service) projectStore() *project.Store {
	if s.projects != nil {
		return s.projects
	}
	return project.NewStore(config.ProjectsPath())
}
