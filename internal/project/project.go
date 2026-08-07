package project

import "errors"

// Entry represents a project from projects.toml.
type Entry struct {
	Alias    string `toml:"-"    json:"alias"`
	Name     string `toml:"name" json:"name,omitempty"`
	Path     string `toml:"path" json:"path"`
	Archived bool   `toml:"-"    json:"archived"`
}

// Load reads and validates projects.toml from path.
func Load(path string) ([]Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return catalog.ListAll(false), nil
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
