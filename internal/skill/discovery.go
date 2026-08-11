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

// DiscoveryPath is one skills root directory. Builtin marks the ~/.agents/skills
// default; configured extras are not built-in and may legitimately be absent on
// a given machine (worth warning about, but not fatal).
type DiscoveryPath struct {
	Dir     string
	Builtin bool
}

// GlobalDiscoveryPaths returns user-global discovery paths in priority order,
// starting with the built-in .agents convention followed by configured extras.
// A leading "~" in a configured entry expands against home; when home is empty
// such entries are skipped rather than resolved to the current directory.
func GlobalDiscoveryPaths(home string, cfg Config) []DiscoveryPath {
	paths := make([]DiscoveryPath, 0, len(cfg.Global)+1)
	paths = append(paths, DiscoveryPath{Dir: filepath.Join(home, ".agents", "skills"), Builtin: true})
	for _, extra := range cfg.Global {
		if expanded, ok := expandHome(extra, home); ok {
			paths = append(paths, DiscoveryPath{Dir: expanded})
		}
	}
	return dedupePaths(paths)
}

// expandHome expands a leading "~" against home. It reports false (entry
// skipped) when the entry starts with "~" but home is empty, so a "~" entry
// never silently resolves to ".".
func expandHome(path, home string) (string, bool) {
	switch {
	case path == "~":
		if home == "" {
			return "", false
		}
		return home, true
	case strings.HasPrefix(path, "~"+string(filepath.Separator)):
		if home == "" {
			return "", false
		}
		return filepath.Join(home, path[2:]), true
	default:
		return path, true
	}
}

// dedupePaths removes duplicate discovery directories while preserving order.
func dedupePaths(paths []DiscoveryPath) []DiscoveryPath {
	seen := make(map[string]bool, len(paths))
	out := make([]DiscoveryPath, 0, len(paths))
	for _, p := range paths {
		dir := filepath.Clean(p.Dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		p.Dir = dir
		out = append(out, p)
	}
	return out
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
