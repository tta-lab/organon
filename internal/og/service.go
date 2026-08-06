package og

import "github.com/tta-lab/organon/internal/githubapp"

// Service owns daemon-scoped dependencies shared across requests.
type Service struct {
	githubBroker githubapp.CredentialBroker
}

// NewService constructs a daemon service. A nil broker leaves GitHub App
// authentication unavailable while preserving non-GitHub operations.
func NewService(githubBroker githubapp.CredentialBroker) Service {
	return Service{githubBroker: githubBroker}
}

// GitHubAppConfigured reports whether this daemon has GitHub App credentials.
func (s Service) GitHubAppConfigured() bool {
	return s.githubBroker != nil
}

func (s Service) resolveRepoContextFor(workDir string) (*repoContext, error) {
	ctx, err := resolveRepoContextFor(workDir)
	if err != nil {
		return nil, err
	}
	ctx.githubBroker = s.githubBroker
	return ctx, nil
}

func (s Service) resolveRemoteRepoContextFor(workDir string) (*repoContext, error) {
	ctx, err := resolveRemoteRepoContextFor(workDir)
	if err != nil {
		return nil, err
	}
	ctx.githubBroker = s.githubBroker
	return ctx, nil
}
