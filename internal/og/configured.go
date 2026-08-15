package og

import (
	"errors"
	"os"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

// LoadService composes the configured OG domain implementation for one process.
// The returned service owns the hot project registry and, when configured, the
// in-memory GitHub App credential broker.
func LoadService(configPath, configDir string) (Service, error) {
	cfg, err := ogconfig.Load(configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg = ogconfig.Config{}
	} else if err != nil {
		return Service{}, err
	}

	var broker githubapp.CredentialBroker
	if cfg.GitHubApp != nil {
		keySource, err := githubapp.NewKeySource(*cfg.GitHubApp, configDir)
		if err != nil {
			return Service{}, err
		}
		broker, err = githubapp.NewBroker(*cfg.GitHubApp, keySource)
		if err != nil {
			return Service{}, err
		}
	}
	return NewServiceWithConfig(broker, project.NewStore(config.ProjectsPath()), cfg), nil
}
