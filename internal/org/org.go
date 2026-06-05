package org

import (
	"fmt"
	"os"
	"sort"

	"github.com/BurntSushi/toml"
)

// Entry represents a single org from orgs.toml.
type Entry struct {
	Name           string `toml:"-"`
	GitHubTokenEnv string `toml:"github_token_env"`
}

// File is the on-disk TOML structure.
type File struct {
	Orgs map[string]struct {
		GitHubTokenEnv string `toml:"github_token_env"`
	} `toml:"-"`
}

// Load reads orgs.toml from path. Returns empty if the file doesn't exist.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading orgs file: %w", err)
	}

	var raw map[string]struct {
		GitHubTokenEnv string `toml:"github_token_env"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing orgs file: %w", err)
	}

	entries := make([]Entry, 0, len(raw))
	for name, e := range raw {
		entries = append(entries, Entry{
			Name:           name,
			GitHubTokenEnv: e.GitHubTokenEnv,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// Get returns a single org by name. Returns nil if not found.
func Get(path, name string) (*Entry, error) {
	entries, err := Load(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, nil
}
