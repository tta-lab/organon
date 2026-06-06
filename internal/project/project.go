package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Entry represents a project from projects.toml.
type Entry struct {
	Alias string `toml:"-"`
	Name  string `toml:"name"`
	Path  string `toml:"path"`
}

// Load reads projects.toml from path. Returns empty if the file doesn't exist.
// Handles the current ttal format: top-level keys are either projects
// (with name/path) or group tables with sub-projects.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading projects file: %w", err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing projects file: %w", err)
	}

	entries := flattenEntries(raw, "")
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Alias < entries[j].Alias
	})

	return entries, nil
}

// flattenEntries recursively extracts project entries from nested TOML tables.
func flattenEntries(m map[string]any, prefix string) []Entry {
	var entries []Entry
	for key, val := range m {
		if key == "archived" {
			continue
		}
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		sub, ok := val.(map[string]any)
		if !ok {
			continue
		}

		// Check if this is a leaf project (has name or path)
		_, hasName := sub["name"]
		_, hasPath := sub["path"]
		if hasName || hasPath {
			var e Entry
			if n, ok := sub["name"].(string); ok {
				e.Name = n
			}
			if p, ok := sub["path"].(string); ok {
				e.Path = p
			}
			// Also check for github_token_env, k8s_app, k8s_namespace - skip those
			e.Alias = fullKey
			entries = append(entries, e)
		}

		// Recurse into sub-tables (like [fb] containing [fb.ap])
		subEntries := flattenEntries(sub, fullKey)
		entries = append(entries, subEntries...)
	}
	return entries
}

// Get returns a project by exact alias. Returns nil if not found.
func Get(path, alias string) (*Entry, error) {
	entries, err := Load(path)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Alias == alias {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// GetByPath returns a project by exact filesystem path. Returns nil if not found.
func GetByPath(projectsPath, targetPath string) (*Entry, error) {
	entries, err := Load(projectsPath)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Path == targetPath {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// Resolve returns a project by alias with hierarchical fallback.
// "fb.ap" tries "fb.ap", then "fb". Returns nil if not found.
func Resolve(path, alias string) (*Entry, error) {
	candidates := []string{alias}
	parts := strings.Split(alias, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		candidates = append(candidates, strings.Join(parts[:i], "."))
	}

	for _, candidate := range candidates {
		e, err := Get(path, candidate)
		if err != nil {
			return nil, err
		}
		if e != nil && e.Path != "" {
			return e, nil
		}
	}

	return nil, nil
}

// ListFiltered returns all projects, optionally filtered by org derived from path.
func ListFiltered(path, orgFilter string) ([]Entry, error) {
	entries, err := Load(path)
	if err != nil {
		return nil, err
	}
	if orgFilter != "" {
		filtered := make([]Entry, 0)
		for _, e := range entries {
			if DeriveOrg(e.Path) == orgFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	sort.Slice(entries, func(i, j int) bool {
		orgI := DeriveOrg(entries[i].Path)
		orgJ := DeriveOrg(entries[j].Path)
		if orgI != orgJ {
			return orgI < orgJ
		}
		return entries[i].Alias < entries[j].Alias
	})
	return entries, nil
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
