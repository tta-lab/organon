package skill

import (
	"errors"
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

// newSkill constructs a Skill from its components.
func newSkill(name string, meta Meta, source, path, body string) Skill {
	return Skill{
		Name:        name,
		Description: meta.Description,
		Category:    meta.Category,
		Source:      source,
		Path:        path,
		Body:        strings.TrimSpace(body),
	}
}

// ProjectDiscoveryPaths returns project-local discovery paths in priority order.
func ProjectDiscoveryPaths(root string) []string {
	return []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".crush", "skills"),
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".cursor", "skills"),
	}
}

// GlobalDiscoveryPaths returns user-global discovery paths in priority order.
func GlobalDiscoveryPaths(home string) []string {
	return []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".crush", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".cursor", "skills"),
	}
}

// DiscoveryPaths returns project-local paths followed by global paths.
func DiscoveryPaths(root, home string) []string {
	return append(ProjectDiscoveryPaths(root), GlobalDiscoveryPaths(home)...)
}

// ListSkills walks all discovery paths and returns all skills, deduplicated by name.
// First-seen wins (paths earlier in the slice have higher priority).
// Returns skills sorted by Name.
func ListSkills(paths []string) ([]Skill, error) {
	seen := make(map[string]bool)
	result := make([]Skill, 0, 8)
	errs := make([]error, 0, 4)

	for _, base := range paths {
		entries, err := os.ReadDir(base)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("read skills directory %q: %w", base, err))
			continue
		}

		for _, entry := range entries {
			skillDir := filepath.Join(base, entry.Name())
			info, err := os.Stat(skillDir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				errs = append(errs, fmt.Errorf("stat skill directory %q: %w", skillDir, err))
				continue
			}
			if !info.IsDir() {
				continue
			}
			skillPath := filepath.Join(skillDir, "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				errs = append(errs, fmt.Errorf("read skill %q: %w", skillPath, err))
				continue
			}

			meta, body := ParseFrontmatter(data)
			if meta.Name == "" {
				continue
			}
			name := meta.Name

			if seen[name] {
				continue
			}
			seen[name] = true

			result = append(result, newSkill(name, meta, base, skillPath, string(body)))
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

// GetSkill returns the skill matching the given frontmatter name, using priority order.
// Returns an error wrapping fs.ErrNotExist if no matching skill is found.
func GetSkill(paths []string, name string) (*Skill, error) {
	skills, err := ListSkills(paths)
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i], nil
		}
	}
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("skill %q not found: %w", name, fs.ErrNotExist))
	}
	return nil, fmt.Errorf("skill %q not found: %w", name, fs.ErrNotExist)
}

// FindSkills returns skills matching any of the keywords (OR match).
// Matching is case-insensitive and checks both Name and Description.
// Results are deduplicated and sorted by Name.
func FindSkills(paths []string, keywords []string) ([]Skill, error) {
	lowerKeywords := make([]string, len(keywords))
	for i, k := range keywords {
		lowerKeywords[i] = strings.ToLower(k)
	}

	skills, err := ListSkills(paths)
	result := make([]Skill, 0, len(skills))
	for _, candidate := range skills {
		if matchesKeywords(candidate.Name, candidate.Description, lowerKeywords) {
			result = append(result, candidate)
		}
	}
	return result, err
}

// matchesKeywords checks if any keyword appears in the skill's name or description.
func matchesKeywords(name, description string, lowerKeywords []string) bool {
	lowerName := strings.ToLower(name)
	lowerDesc := strings.ToLower(description)
	for _, kw := range lowerKeywords {
		if strings.Contains(lowerName, kw) || strings.Contains(lowerDesc, kw) {
			return true
		}
	}
	return false
}
