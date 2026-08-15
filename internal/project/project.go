package project

import "errors"

// Entry represents a project from projects.toml.
type Entry struct {
	Alias    string `toml:"-"    json:"alias"`
	Name     string `toml:"name" json:"name"`
	Path     string `toml:"path" json:"path"`
	Remote   string `toml:"remote" json:"remote"`
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

// Resolve returns a project for a canonical alias or an alternate exact project reference.
func Resolve(path, reference string) (*Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.Resolve(reference)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// Find returns ranked active projects for a natural-language query.
func Find(path, query string, limit int) ([]Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return catalog.Find(query, limit)
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
