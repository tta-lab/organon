package skill

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maximumSkillBytes = 1024 * 1024

// DiscoveryPaths returns project-local and user-global skill directories in
// priority order. A same-named local skill overrides a global one.
func DiscoveryPaths(cwd, home string) []string {
	return []string{
		filepath.Join(cwd, ".agents", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

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

// ListSkills walks all discovery paths and returns all skills, deduplicated by name.
// First-seen wins (paths earlier in the slice have higher priority).
// Returns skills sorted by Name.
func ListSkills(paths []string) ([]Skill, error) {
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
			candidate, err := loadSkill(base, entry.Name())
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
			result = append(result, *candidate)
		}
	}

	result = NewCatalog(result).List()
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

func loadSkill(base, entryName string) (*Skill, error) {
	skillDir := filepath.Join(base, entryName)
	info, err := os.Stat(skillDir)
	if err != nil {
		return nil, fmt.Errorf("stat skill directory %q: %w", skillDir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	data, err := readSkillData(skillPath)
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

func readSkillData(skillPath string) ([]byte, error) {
	file, err := os.Open(skillPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximumSkillBytes {
		return nil, fmt.Errorf("SKILL.md exceeds the 1 MiB skill limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumSkillBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumSkillBytes {
		return nil, fmt.Errorf("SKILL.md exceeds the 1 MiB skill limit")
	}
	return data, nil
}

// GetSkill returns the skill matching the given frontmatter name, using priority order.
// Returns an error wrapping fs.ErrNotExist if no matching skill is found.
func GetSkill(paths []string, name string) (*Skill, error) {
	catalog, discoveryErr := LoadCatalog(paths)
	found, getErr := catalog.Get(name)
	if getErr == nil {
		return &found, nil
	}
	return nil, errors.Join(discoveryErr, getErr)
}
