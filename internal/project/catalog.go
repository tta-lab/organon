package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/tta-lab/organon/internal/gitprovider"
)

var (
	ErrInvalidAlias  = errors.New("invalid project alias")
	ErrInvalidConfig = errors.New("invalid project configuration")
	ErrNotFound      = errors.New("project not found")
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Catalog is an immutable snapshot of active registered projects.
type Catalog struct {
	activeEntries   []Entry
	archivedEntries []Entry
	byAlias         map[string]Entry
	byPath          map[string]Entry
	byRemote        map[string]Entry
}

// OpenCatalog reads and validates one projects.toml snapshot.
func OpenCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newCatalog(nil, nil), nil
		}
		return nil, fmt.Errorf("reading projects file: %w", err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing projects file: %w", err)
	}

	active, err := parseActiveEntries(raw)
	if err != nil {
		return nil, err
	}
	archived, err := parseArchivedEntries(raw["archived"])
	if err != nil {
		return nil, err
	}
	if err := validateGlobalUniqueness(append(append([]Entry(nil), active...), archived...)); err != nil {
		return nil, err
	}

	sort.Slice(active, func(i, j int) bool { return active[i].Alias < active[j].Alias })
	sort.Slice(archived, func(i, j int) bool { return archived[i].Alias < archived[j].Alias })
	return newCatalog(active, archived), nil
}

func parseActiveEntries(raw map[string]any) ([]Entry, error) {
	entries := make([]Entry, 0, len(raw))
	for alias, value := range raw {
		if alias == "archived" {
			continue
		}
		entry, err := parseActiveEntry(alias, value)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseActiveEntry(alias string, value any) (Entry, error) {
	if !validAlias(alias) {
		return Entry{}, fmt.Errorf(
			"%w %q: aliases must contain only letters, digits, '_' or '-'",
			ErrInvalidAlias, alias,
		)
	}
	table, ok := value.(map[string]any)
	if !ok {
		return Entry{}, fmt.Errorf("%w for %q: expected a table", ErrInvalidConfig, alias)
	}
	for key, field := range table {
		if _, nested := field.(map[string]any); nested {
			return Entry{}, fmt.Errorf(
				"%w %q: nested active tables are not allowed",
				ErrInvalidAlias, alias+"."+key,
			)
		}
		if key != "name" && key != "path" && key != "remote" {
			return Entry{}, fmt.Errorf("%w for %q: unsupported active field %q", ErrInvalidConfig, alias, key)
		}
	}

	entry := Entry{Alias: alias}
	if name, exists := table["name"]; exists {
		entry.Name, ok = name.(string)
		if !ok {
			return Entry{}, fmt.Errorf("%w for %q: name must be a string", ErrInvalidConfig, alias)
		}
	}
	entry.Path, _ = table["path"].(string)
	if entry.Path == "" || !filepath.IsAbs(entry.Path) {
		return Entry{}, fmt.Errorf(
			"%w for %q: path must be a non-empty absolute path",
			ErrInvalidConfig, alias,
		)
	}
	entry.Path = filepath.Clean(entry.Path)
	var err error
	entry.Remote, err = canonicalRemote(alias, table["remote"])
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func parseArchivedEntries(value any) ([]Entry, error) {
	if value == nil {
		return nil, nil
	}
	tables, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: archived must be a table", ErrInvalidConfig)
	}
	entries := make([]Entry, 0, len(tables))
	for alias, rawEntry := range tables {
		if !validAlias(alias) {
			return nil, fmt.Errorf("%w %q", ErrInvalidAlias, alias)
		}
		table, ok := rawEntry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w for archived %q: expected a table", ErrInvalidConfig, alias)
		}
		for key, field := range table {
			if _, nested := field.(map[string]any); nested {
				return nil, fmt.Errorf("%w %q: nested archived tables are not allowed", ErrInvalidAlias, alias+"."+key)
			}
		}
		entry := Entry{Alias: alias, Archived: true}
		entry.Name, _ = table["name"].(string)
		if name, exists := table["name"]; exists {
			if entry.Name, ok = name.(string); !ok {
				return nil, fmt.Errorf("%w for archived %q: name must be a string", ErrInvalidConfig, alias)
			}
		}
		entry.Path, _ = table["path"].(string)
		if entry.Path == "" || !filepath.IsAbs(entry.Path) {
			return nil, fmt.Errorf("%w for archived %q: path must be a non-empty absolute path", ErrInvalidConfig, alias)
		}
		entry.Path = filepath.Clean(entry.Path)
		var err error
		entry.Remote, err = canonicalRemote(alias, table["remote"])
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func canonicalRemote(alias string, value any) (string, error) {
	remote, ok := value.(string)
	if !ok || remote == "" {
		return "", fmt.Errorf(
			"%w for %q: remote must be a non-empty canonical HTTP(S) repository URL",
			ErrInvalidConfig, alias,
		)
	}
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil {
		return "", fmt.Errorf("%w for %q: invalid remote: %v", ErrInvalidConfig, alias, err)
	}
	if remote != info.CanonicalURL {
		return "", fmt.Errorf("%w for %q: remote must use canonical form %q", ErrInvalidConfig, alias, info.CanonicalURL)
	}
	return info.CanonicalURL, nil
}

func validateGlobalUniqueness(entries []Entry) error {
	aliases := make(map[string]string, len(entries))
	paths := make(map[string]string, len(entries))
	remotes := make(map[string]string, len(entries))
	for _, entry := range entries {
		if previous, ok := aliases[entry.Alias]; ok {
			return fmt.Errorf("%w: projects %q and %q use alias %q", ErrInvalidConfig, previous, entry.Alias, entry.Alias)
		}
		if previous, ok := paths[entry.Path]; ok {
			return fmt.Errorf("%w: projects %q and %q use the same path %q", ErrInvalidConfig, previous, entry.Alias, entry.Path)
		}
		if previous, ok := remotes[entry.Remote]; ok {
			return fmt.Errorf(
				"%w: projects %q and %q use the same remote %q",
				ErrInvalidConfig, previous, entry.Alias, entry.Remote,
			)
		}
		aliases[entry.Alias] = entry.Alias
		paths[entry.Path] = entry.Alias
		remotes[entry.Remote] = entry.Alias
	}
	return nil
}

func newCatalog(active, archived []Entry) *Catalog {
	catalog := &Catalog{
		activeEntries:   append([]Entry(nil), active...),
		archivedEntries: append([]Entry(nil), archived...),
		byAlias:         make(map[string]Entry, len(active)+len(archived)),
		byPath:          make(map[string]Entry, len(active)+len(archived)),
		byRemote:        make(map[string]Entry, len(active)+len(archived)),
	}
	for _, entry := range archived {
		catalog.byAlias[entry.Alias] = entry
		catalog.byPath[entry.Path] = entry
		catalog.byRemote[entry.Remote] = entry
	}
	for _, entry := range active {
		catalog.byAlias[entry.Alias] = entry
		catalog.byPath[entry.Path] = entry
		catalog.byRemote[entry.Remote] = entry
	}
	return catalog
}

// GetByRemote returns one project by canonical HTTP(S) repository identity.
func (c *Catalog) GetByRemote(remote string) (Entry, error) {
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: invalid lookup remote: %v", ErrInvalidConfig, err)
	}
	entry, ok := c.byRemote[info.CanonicalURL]
	if !ok {
		return Entry{}, fmt.Errorf("%w for remote %q", ErrNotFound, info.CanonicalURL)
	}
	return entry, nil
}

func validAlias(alias string) bool { return aliasPattern.MatchString(alias) }

// ValidateAlias verifies the public exact single-layer alias contract.
func ValidateAlias(alias string) error {
	if !validAlias(alias) {
		return fmt.Errorf("%w %q", ErrInvalidAlias, alias)
	}
	return nil
}

// ListAll returns active projects and, when requested, archived projects.
func (c *Catalog) ListAll(includeArchived bool) []Entry {
	entries := append([]Entry(nil), c.activeEntries...)
	if includeArchived {
		entries = append(entries, c.archivedEntries...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Alias < entries[j].Alias })
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
