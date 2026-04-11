package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill represents a discovered skill with its metadata and source location.
type Skill struct {
	Name        string
	Description string
	Category    string
	Source      string // absolute path of the discovery directory
	Path        string // absolute path to SKILL.md
	Body        string // frontmatter-stripped content
}

// DiscoveryPaths returns the 8 discovery paths in priority order.
func DiscoveryPaths(cwd, home string) []string {
	return []string{
		filepath.Join(cwd, ".agents", "skills"),
		filepath.Join(cwd, ".crush", "skills"),
		filepath.Join(cwd, ".claude", "skills"),
		filepath.Join(cwd, ".cursor", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".crush", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".cursor", "skills"),
	}
}

// ListSkills walks all discovery paths and returns all skills, deduplicated by name.
// First-seen wins (paths earlier in the slice have higher priority).
// Returns skills sorted by Name.
func ListSkills(paths []string) ([]Skill, error) {
	seen := make(map[string]bool)
	var result []Skill

	for _, base := range paths {
		entries, err := os.ReadDir(base)
		if err != nil {
			// Skip paths that don't exist or can't be read (permission denied, etc.)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(base, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}

			meta, body := ParseFrontmatter(data)
			name := entry.Name()
			if meta.Name != "" {
				name = meta.Name
			}

			if seen[name] {
				continue
			}
			seen[name] = true

			result = append(result, Skill{
				Name:        name,
				Description: meta.Description,
				Category:    meta.Category,
				Source:      base,
				Path:        skillPath,
				Body:        string(body),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// GetSkill returns the skill with the given name, using priority order.
// Returns an error wrapping fs.ErrNotExist if not found.
func GetSkill(paths []string, name string) (*Skill, error) {
	for _, base := range paths {
		skillPath := filepath.Join(base, name, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}

		meta, body := ParseFrontmatter(data)
		// Use frontmatter name if present, otherwise fall back to dir name
		skillName := name
		if meta.Name != "" {
			skillName = meta.Name
		}

		return &Skill{
			Name:        skillName,
			Description: meta.Description,
			Category:    meta.Category,
			Source:      base,
			Path:        skillPath,
			Body:        string(body),
		}, nil
	}
	return nil, fmt.Errorf("skill %q not found: %w", name, fs.ErrNotExist)
}

// FindSkills returns skills matching any of the keywords (OR match).
// Matching is case-insensitive and checks both Name and Description.
// Results are deduplicated and sorted by Name.
func FindSkills(paths []string, keywords []string) ([]Skill, error) {
	// Lowercase keywords once
	var lowerKeywords []string
	for _, k := range keywords {
		lowerKeywords = append(lowerKeywords, strings.ToLower(k))
	}

	seen := make(map[string]bool)
	var result []Skill

	for _, base := range paths {
		entries, err := os.ReadDir(base)
		if err != nil {
			// Skip paths that don't exist or can't be read (permission denied, etc.)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(base, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}

			meta, body := ParseFrontmatter(data)
			name := entry.Name()
			if meta.Name != "" {
				name = meta.Name
			}

			if seen[name] {
				continue
			}

			lowerName := strings.ToLower(name)
			lowerDesc := strings.ToLower(meta.Description)

			var matched bool
			for _, kw := range lowerKeywords {
				if strings.Contains(lowerName, kw) || strings.Contains(lowerDesc, kw) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			seen[name] = true
			result = append(result, Skill{
				Name:        name,
				Description: meta.Description,
				Category:    meta.Category,
				Source:      base,
				Path:        skillPath,
				Body:        string(body),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}
