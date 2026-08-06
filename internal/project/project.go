package project

import (
	"errors"
	"path/filepath"
)

// Entry represents a project from projects.toml.
type Entry struct {
	Alias        string `toml:"-"                json:"alias,omitempty"`
	Name         string `toml:"name"             json:"name,omitempty"`
	Path         string `toml:"path"             json:"path,omitempty"`
	K8sApp       string `toml:"k8s_app"          json:"k8s_app,omitempty"`
	K8sNamespace string `toml:"k8s_namespace"    json:"k8s_namespace,omitempty"`
	Org          string `toml:"-"                json:"org,omitempty"`
}

// Load reads and validates projects.toml from path.
func Load(path string) ([]Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return catalog.List(""), nil
}

// Get returns a project by exact alias. Returns nil if not found.
func Get(path, alias string) (*Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.GetExact(alias)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}

// GetByPath returns a project by exact filesystem path. Returns nil if not found.
func GetByPath(projectsPath, targetPath string) (*Entry, error) {
	catalog, err := OpenCatalog(projectsPath)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.GetByPath(targetPath)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}

// ListFiltered returns all projects, optionally filtered by org derived from path.
func ListFiltered(path, orgFilter string) ([]Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return catalog.List(orgFilter), nil
}

// DeriveOrg extracts the org name from a project or reference path.
// For code/projects paths: /home/user/code/projects/tta-lab/organon -> "tta-lab"
// For code/references paths: /home/user/code/references/github.com/tta-lab/agon -> "tta-lab"
func DeriveOrg(p string) string {
	p = filepath.Clean(p)

	// Walk up from the leaf, collecting path components
	parts := make([]string, 0)
	current := p
	for {
		base := filepath.Base(current)
		if base == "" || base == "." || base == "/" {
			break
		}
		parts = append(parts, base)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Now scan for "projects" or "references" - the component after it (closer to leaf) is the org
	// parts are leaf→root: [repo, org, projects, code, ...]
	// For references: [repo, org, github.com, references, code, ...]
	for i := 0; i < len(parts); i++ {
		if parts[i] == "projects" && i-1 >= 0 {
			return parts[i-1]
		}
		if parts[i] == "references" && i-2 >= 0 {
			// Skip the host (e.g. github.com) between references and org
			return parts[i-2]
		}
	}

	return ""
}
