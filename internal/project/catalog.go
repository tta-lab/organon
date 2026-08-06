package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

var (
	ErrInvalidAlias  = errors.New("invalid project alias")
	ErrInvalidConfig = errors.New("invalid project configuration")
	ErrNotFound      = errors.New("project not found")
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Catalog is an immutable snapshot of active registered projects.
type Catalog struct {
	entries []Entry
	byAlias map[string]Entry
	byPath  map[string]Entry
}

// OpenCatalog reads and validates one projects.toml snapshot.
func OpenCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newCatalog(nil), nil
		}
		return nil, fmt.Errorf("reading projects file: %w", err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing projects file: %w", err)
	}

	entries := make([]Entry, 0, len(raw))
	paths := make(map[string]string, len(raw))
	for alias, value := range raw {
		if alias == "archived" {
			continue
		}
		if !validAlias(alias) {
			return nil, fmt.Errorf("%w %q: aliases must contain only letters, digits, '_' or '-'", ErrInvalidAlias, alias)
		}
		table, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w for %q: expected a table", ErrInvalidConfig, alias)
		}
		for key, field := range table {
			if _, nested := field.(map[string]any); nested {
				return nil, fmt.Errorf("%w %q: nested active tables are not allowed", ErrInvalidAlias, alias+"."+key)
			}
		}

		entry := Entry{Alias: alias}
		entry.Name, _ = table["name"].(string)
		entry.Path, _ = table["path"].(string)
		entry.K8sApp, _ = table["k8s_app"].(string)
		entry.K8sNamespace, _ = table["k8s_namespace"].(string)
		if entry.Path == "" || !filepath.IsAbs(entry.Path) {
			return nil, fmt.Errorf("%w for %q: path must be a non-empty absolute path", ErrInvalidConfig, alias)
		}
		entry.Path = filepath.Clean(entry.Path)
		entry.Org = DeriveOrg(entry.Path)
		if previous, duplicate := paths[entry.Path]; duplicate {
			return nil, fmt.Errorf("%w: projects %q and %q use the same path %q", ErrInvalidConfig, previous, alias, entry.Path)
		}
		paths[entry.Path] = alias
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Alias < entries[j].Alias })
	return newCatalog(entries), nil
}

func newCatalog(entries []Entry) *Catalog {
	catalog := &Catalog{
		entries: append([]Entry(nil), entries...),
		byAlias: make(map[string]Entry, len(entries)),
		byPath:  make(map[string]Entry, len(entries)),
	}
	for _, entry := range entries {
		catalog.byAlias[entry.Alias] = entry
		catalog.byPath[entry.Path] = entry
	}
	return catalog
}

func validAlias(alias string) bool { return aliasPattern.MatchString(alias) }

// List returns a copy of active projects, optionally filtered by derived org.
func (c *Catalog) List(orgFilter string) []Entry {
	entries := make([]Entry, 0, len(c.entries))
	for _, entry := range c.entries {
		if orgFilter == "" || entry.Org == orgFilter {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Org != entries[j].Org {
			return entries[i].Org < entries[j].Org
		}
		return entries[i].Alias < entries[j].Alias
	})
	return entries
}

// GetExact returns one project by exact, single-layer alias.
func (c *Catalog) GetExact(alias string) (Entry, error) {
	if !validAlias(alias) {
		return Entry{}, fmt.Errorf("%w %q", ErrInvalidAlias, alias)
	}
	entry, ok := c.byAlias[alias]
	if !ok {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, alias)
	}
	return entry, nil
}

// GetByPath returns one project by cleaned absolute filesystem path.
func (c *Catalog) GetByPath(path string) (Entry, error) {
	if path == "" || !filepath.IsAbs(path) {
		return Entry{}, fmt.Errorf("%w: lookup path must be absolute", ErrInvalidConfig)
	}
	cleaned := filepath.Clean(path)
	entry, ok := c.byPath[cleaned]
	if !ok {
		return Entry{}, fmt.Errorf("%w for path %q", ErrNotFound, cleaned)
	}
	return entry, nil
}
