package srcview

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/sys/unix"
)

const MaxListEntries = 200

type ListEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type ListResult struct {
	Project   string      `json:"project"`
	Path      string      `json:"path"`
	Entries   []ListEntry `json:"entries"`
	Truncated bool        `json:"truncated"`
}

// ListDirectory returns direct children of a directory in a registered checkout.
//
//nolint:gocyclo // Containment, entry classification, and bounded output are one traversal.
func (s *ProjectService) ListDirectory(selector, path string) (ListResult, error) {
	entry, err := s.Resolve(selector)
	if err != nil {
		return ListResult{}, err
	}
	clean := "."
	if path != "" && path != "." {
		clean, err = cleanRelativePath(path)
		if err != nil {
			return ListResult{}, err
		}
	}
	root, err := os.OpenRoot(entry.Path)
	if err != nil {
		return ListResult{}, fmt.Errorf("open project root: %w", err)
	}
	defer func() { _ = root.Close() }()
	dir, err := root.OpenFile(clean, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return ListResult{}, fmt.Errorf("open project directory %q: %w", clean, err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return ListResult{}, err
	}
	if !info.IsDir() {
		return ListResult{}, fmt.Errorf("project path %q is not a directory", clean)
	}
	children, err := dir.ReadDir(-1)
	if err != nil {
		return ListResult{}, fmt.Errorf("list project directory %q: %w", clean, err)
	}
	slices.SortFunc(children, func(a, b os.DirEntry) int {
		if a.Name() < b.Name() {
			return -1
		}
		if a.Name() > b.Name() {
			return 1
		}
		return 0
	})
	result := ListResult{
		Project: entry.Alias, Path: filepath.ToSlash(clean),
		Entries: make([]ListEntry, 0, len(children)),
	}
	for _, child := range children {
		if clean == "." && child.Name() == ".git" {
			continue
		}
		if len(result.Entries) == MaxListEntries {
			result.Truncated = true
			break
		}
		kind := "file"
		if child.IsDir() {
			kind = "directory"
		} else if child.Type()&os.ModeSymlink != 0 {
			kind = "symlink"
		} else if !child.Type().IsRegular() {
			kind = "other"
		}
		relative := child.Name()
		if clean != "." {
			relative = filepath.Join(clean, relative)
		}
		result.Entries = append(result.Entries, ListEntry{
			Name: child.Name(), Path: filepath.ToSlash(relative), Type: kind,
		})
	}
	return result, nil
}
