package skill

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Catalog is one prioritized, deduplicated set of skills shared by adapters.
type Catalog struct {
	skills []Skill
}

// NewCatalog merges skill groups in priority order and sorts the selected
// skills by their exact frontmatter names.
func NewCatalog(groups ...[]Skill) Catalog {
	seen := make(map[string]bool)
	merged := make([]Skill, 0)
	for _, group := range groups {
		for _, candidate := range group {
			if seen[candidate.Name] {
				continue
			}
			seen[candidate.Name] = true
			merged = append(merged, candidate)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Name < merged[j].Name })
	return Catalog{skills: merged}
}

// LoadCatalog discovers a catalog from prioritized filesystem roots.
func LoadCatalog(paths []string) (Catalog, error) {
	skills, err := ListSkills(paths)
	return NewCatalog(skills), err
}

// List returns the selected skills in name order.
func (c Catalog) List() []Skill {
	return append([]Skill(nil), c.skills...)
}

// Find returns skills ranked for a natural-language query.
func (c Catalog) Find(query string, limit int) ([]Skill, error) {
	return SearchSkills(c.skills, query, limit)
}

// Get returns one skill by its exact, case-sensitive frontmatter name.
func (c Catalog) Get(name string) (Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Skill{}, fmt.Errorf("name must not be blank")
	}
	for _, candidate := range c.skills {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return Skill{}, fmt.Errorf("skill %q not found: %w", name, fs.ErrNotExist)
}
