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
	errs := make([]error, 0, len(paths))

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
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(base, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				errs = append(errs, fmt.Errorf("read skill %q: %w", skillPath, err))
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

// GetSkill returns the skill matching the given directory name, using priority order.
// The name is interpreted as a directory name; if the directory's SKILL.md has a
// frontmatter 'name' field, Skill.Name will be that value instead of the directory name.
// Returns an error wrapping fs.ErrNotExist if no matching skill is found.
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

		s := newSkill(skillName, meta, base, skillPath, string(body))
		return &s, nil
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

	seen := make(map[string]bool)
	var result []Skill
	errs := make([]error, 0, len(paths))

	for _, base := range paths {
		skills, scanErrs := scanSkillDir(base, lowerKeywords, seen)
		errs = append(errs, scanErrs...)
		for _, s := range skills {
			seen[s.Name] = true
			result = append(result, s)
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

// scanSkillDir scans a single discovery directory for matching skills.
// Returns skills found and any non-ENOENT errors encountered.
func scanSkillDir(base string, lowerKeywords []string, seen map[string]bool) ([]Skill, []error) {
	var result []Skill
	var errs []error

	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("read skills directory %q: %w", base, err)}
	}

	for _, entry := range entries {
		if !entry.IsDir() || seen[entry.Name()] {
			continue
		}
		skill, scanErr := scanSkill(base, entry.Name(), lowerKeywords)
		if scanErr != nil {
			if os.IsNotExist(scanErr) {
				continue
			}
			errs = append(errs, scanErr)
			continue
		}
		if skill != nil {
			result = append(result, *skill)
		}
	}

	return result, errs
}

// scanSkill reads and checks a single skill directory for keyword match.
// Returns nil skill if no SKILL.md or no match; returns the error if the file couldn't be read.
func scanSkill(base, dirName string, lowerKeywords []string) (*Skill, error) {
	skillPath := filepath.Join(base, dirName, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}

	meta, body := ParseFrontmatter(data)
	name := dirName
	if meta.Name != "" {
		name = meta.Name
	}

	if !matchesKeywords(name, meta.Description, lowerKeywords) {
		return nil, nil
	}

	s := newSkill(name, meta, base, skillPath, string(body))
	return &s, nil
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
