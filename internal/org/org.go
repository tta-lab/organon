package org

import (
	"errors"
)

// Entry represents a single org from orgs.toml.
type Entry struct {
	Name           string `toml:"-" json:"name"`
	GitHubTokenEnv string `toml:"github_token_env" json:"github_token_env,omitempty"`
}

// Load reads orgs.toml from path. Returns empty if the file doesn't exist.
func Load(path string) ([]Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return catalog.List(), nil
}

// Get returns a single org by name. Returns nil if not found.
func Get(path, name string) (*Entry, error) {
	catalog, err := OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.GetExact(name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entry, nil
}
