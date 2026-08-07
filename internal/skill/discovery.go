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
	return listSkills(paths, "")
}

// ListSkillsContained discovers skills while requiring every directory and
// SKILL.md target to resolve inside root. It is intended for project-scoped MCP reads.
func ListSkillsContained(paths []string, root string) ([]Skill, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve skill root %q: %w", root, err)
	}
	return listSkills(paths, realRoot)
}

func listSkills(paths []string, containmentRoot string) ([]Skill, error) {
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
			candidate, err := loadSkill(base, entry.Name(), containmentRoot)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				errs = append(errs, err)
				continue
			}
			if candidate == nil {
				continue
			}
			if seen[candidate.Name] {
				continue
			}
			seen[candidate.Name] = true
			result = append(result, *candidate)
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

func loadSkill(base, entryName, containmentRoot string) (*Skill, error) {
	skillDir := filepath.Join(base, entryName)
	info, err := os.Stat(skillDir)
	if err != nil {
		return nil, fmt.Errorf("stat skill directory %q: %w", skillDir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	if containmentRoot != "" {
		if _, err := resolveContainedSkillPath(containmentRoot, skillDir); err != nil {
			return nil, fmt.Errorf("resolve skill directory %q: %w", skillDir, err)
		}
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	readPath := skillPath
	if containmentRoot != "" {
		readPath, err = resolveContainedSkillPath(containmentRoot, skillPath)
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q: %w", skillPath, err)
		}
	}
	data, err := os.ReadFile(readPath)
	if err != nil {
		return nil, fmt.Errorf("read skill %q: %w", skillPath, err)
	}
	meta, body := ParseFrontmatter(data)
	if meta.Name == "" {
		return nil, nil
	}
	loaded := newSkill(meta.Name, meta, base, skillPath, string(body))
	return &loaded, nil
}

func resolveContainedSkillPath(root, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return "", fmt.Errorf("resolved path %q is outside project root %q", resolved, root)
	}
	return resolved, nil
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
	skills, err := ListSkills(paths)
	return FilterSkills(skills, keywords), err
}

// FilterSkills returns skills matching any keyword in name or description.
func FilterSkills(skills []Skill, keywords []string) []Skill {
	lowerKeywords := make([]string, len(keywords))
	for i, keyword := range keywords {
		lowerKeywords[i] = strings.ToLower(keyword)
	}
	result := make([]Skill, 0, len(skills))
	for _, candidate := range skills {
		if matchesKeywords(candidate.Name, candidate.Description, lowerKeywords) {
			result = append(result, candidate)
		}
	}
	return result
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
