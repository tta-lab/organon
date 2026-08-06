package org

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

var (
	ErrInvalidName   = errors.New("invalid org name")
	ErrInvalidConfig = errors.New("invalid org configuration")
	ErrNotFound      = errors.New("org not found")
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Catalog is an immutable snapshot of registered orgs.
type Catalog struct {
	entries []Entry
	byName  map[string]Entry
}

// OpenCatalog reads and validates one orgs.toml snapshot.
func OpenCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newCatalog(nil), nil
		}
		return nil, fmt.Errorf("reading orgs file: %w", err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing orgs file: %w", err)
	}
	entries := make([]Entry, 0, len(raw))
	for name, value := range raw {
		if !namePattern.MatchString(name) {
			return nil, fmt.Errorf("%w %q", ErrInvalidName, name)
		}
		table, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w for %q: expected a table", ErrInvalidConfig, name)
		}
		for key, field := range table {
			if _, nested := field.(map[string]any); nested {
				return nil, fmt.Errorf("%w %q", ErrInvalidName, name+"."+key)
			}
		}
		entry := Entry{Name: name}
		entry.GitHubTokenEnv, _ = table["github_token_env"].(string)
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return newCatalog(entries), nil
}

func newCatalog(entries []Entry) *Catalog {
	catalog := &Catalog{entries: append([]Entry(nil), entries...), byName: make(map[string]Entry, len(entries))}
	for _, entry := range entries {
		catalog.byName[entry.Name] = entry
	}
	return catalog
}

// List returns a sorted copy of registered orgs.
func (c *Catalog) List() []Entry { return append([]Entry(nil), c.entries...) }

// GetExact returns an org by exact, single-layer name.
func (c *Catalog) GetExact(name string) (Entry, error) {
	if !namePattern.MatchString(name) {
		return Entry{}, fmt.Errorf("%w %q", ErrInvalidName, name)
	}
	entry, ok := c.byName[name]
	if !ok {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return entry, nil
}
