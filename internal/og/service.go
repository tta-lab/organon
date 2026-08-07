package og

import (
	"fmt"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

// Service owns daemon-scoped dependencies shared across requests.
type Service struct {
	githubBroker githubapp.CredentialBroker
	projects     *project.Store
	config       ogconfig.Config
}

// NewService constructs a daemon service. A nil broker leaves GitHub App
// authentication unavailable while preserving non-GitHub operations.
func NewService(githubBroker githubapp.CredentialBroker) Service {
	return NewServiceWithConfig(
		githubBroker, project.NewStore(config.ProjectsPath()), ogconfig.Config{},
	)
}

// NewServiceWithConfig constructs a daemon service with explicit hot registry and trust policy.
func NewServiceWithConfig(
	githubBroker githubapp.CredentialBroker,
	projects *project.Store,
	cfg ogconfig.Config,
) Service {
	return Service{githubBroker: githubBroker, projects: projects, config: cfg}
}

// GitHubAppConfigured reports whether this daemon has GitHub App credentials.
func (s Service) GitHubAppConfigured() bool {
	return s.githubBroker != nil
}

// Validate checks daemon-owned configuration without starting listeners or network operations.
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

func (s Service) projectStore() *project.Store {
	if s.projects != nil {
		return s.projects
	}
	return project.NewStore(config.ProjectsPath())
}
