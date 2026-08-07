package srcview

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/tta-lab/organon/internal/project"
)

type projectGetter interface {
	Get(alias string) (project.Entry, error)
}

// File is one safely resolved UTF-8 text file in a registered project.
type File struct {
	Project string
	Path    string
	Source  []byte
}

// ProjectService resolves project aliases and safely reads repository-relative files.
type ProjectService struct {
	projects projectGetter
}

// NewProjectService creates a project-backed source read service.
func NewProjectService(projects projectGetter) *ProjectService {
	return &ProjectService{projects: projects}
}

// ReadFile resolves an exact alias and repository-relative path without allowing escape.
func (s *ProjectService) ReadFile(projectAlias, relativePath string) (File, error) {
	projectAlias = strings.TrimSpace(projectAlias)
	if projectAlias == "" {
		return File{}, fmt.Errorf("project must not be blank")
	}
	cleanPath, err := cleanRelativePath(relativePath)
	if err != nil {
		return File{}, err
	}
	entry, err := s.projects.Get(projectAlias)
	if err != nil {
		return File{}, fmt.Errorf("get project %q: %w", projectAlias, err)
	}
	resolved, err := resolveInsideRoot(entry.Path, cleanPath)
	if err != nil {
		return File{}, err
	}
	source, err := readTextFile(resolved, cleanPath)
	if err != nil {
		return File{}, err
	}
	return File{Project: entry.Alias, Path: filepath.ToSlash(cleanPath), Source: source}, nil
}

func cleanRelativePath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("path must be a non-empty repository-relative path")
	}
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the project root", relativePath)
	}
	return cleanPath, nil
}

type resolvedFile struct {
	root     string
	relative string
}

func resolveInsideRoot(root, cleanPath string) (resolvedFile, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return resolvedFile{}, fmt.Errorf("resolve project root: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(filepath.Join(realRoot, cleanPath))
	if err != nil {
		return resolvedFile{}, fmt.Errorf("resolve project file %q: %w", cleanPath, err)
	}
	rel, err := filepath.Rel(realRoot, realTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return resolvedFile{}, fmt.Errorf("path %q resolves outside the project root", cleanPath)
	}
	return resolvedFile{root: realRoot, relative: rel}, nil
}

func readTextFile(resolved resolvedFile, cleanPath string) ([]byte, error) {
	file, err := openResolvedFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("open project file %q: %w", cleanPath, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat project file %q: %w", cleanPath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("project path %q is not a regular file", cleanPath)
	}
	source, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read project file %q: %w", cleanPath, err)
	}
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return nil, fmt.Errorf("project file %q is not UTF-8 text", cleanPath)
	}
	return source, nil
}

func openResolvedFile(resolved resolvedFile) (*os.File, error) {
	directory, err := openAbsoluteDirectory(resolved.root)
	if err != nil {
		return nil, err
	}
	components := strings.Split(filepath.Clean(resolved.relative), string(filepath.Separator))
	for _, component := range components[:len(components)-1] {
		next, err := unix.Openat(directory, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(directory)
		if err != nil {
			return nil, err
		}
		directory = next
	}
	fileDescriptor, err := unix.Openat(
		directory, components[len(components)-1],
		unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0,
	)
	_ = unix.Close(directory)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fileDescriptor), filepath.Join(resolved.root, resolved.relative)), nil
}

func openAbsoluteDirectory(path string) (int, error) {
	directory, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	cleanPath := strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator))
	components := strings.Split(cleanPath, string(filepath.Separator))
	for _, component := range components {
		if component == "" {
			continue
		}
		next, err := unix.Openat(directory, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(directory)
		if err != nil {
			return -1, err
		}
		directory = next
	}
	return directory, nil
}
