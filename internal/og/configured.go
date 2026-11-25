package og

import (
	"errors"
	"os"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/gitcredential"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

// LoadService composes the configured OG domain implementation for one process.
// The returned service owns the hot project registry and Git credential lookup.
func LoadService(configPath string) (Service, error) {
	cfg, err := ogconfig.Load(configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg = ogconfig.Config{}
	} else if err != nil {
		return Service{}, err
	}

	return NewServiceWithConfig(gitcredential.Git{}, project.NewStore(config.ProjectsPath()), cfg), nil
}
