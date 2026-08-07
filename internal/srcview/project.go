package srcview

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/safefile"
)

const maximumSourceBytes = 16 * 1024 * 1024

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
	source, err := readTextFile(entry.Path, cleanPath)
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

func readTextFile(root, cleanPath string) ([]byte, error) {
	file, err := safefile.OpenContained(root, cleanPath)
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
	if info.Size() > maximumSourceBytes {
		return nil, fmt.Errorf("project file %q exceeds the 16 MiB source limit", cleanPath)
	}
	source, err := readBoundedSource(file)
	if err != nil {
		return nil, fmt.Errorf("read project file %q: %w", cleanPath, err)
	}
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return nil, fmt.Errorf("project file %q is not UTF-8 text", cleanPath)
	}
	return source, nil
}

func readBoundedSource(reader io.Reader) ([]byte, error) {
	source, err := io.ReadAll(io.LimitReader(reader, maximumSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(source) > maximumSourceBytes {
		return nil, fmt.Errorf("source exceeds the 16 MiB source limit")
	}
	return source, nil
}
