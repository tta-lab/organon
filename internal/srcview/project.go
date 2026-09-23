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

const maxSourceBytes = 16 * 1024 * 1024

type projectResolver interface {
	Resolve(string) (project.Entry, error)
	GetByPath(string) (project.Entry, error)
}

type ProjectService struct{ projects projectResolver }

func NewProjectService(projects projectResolver) *ProjectService {
	return &ProjectService{projects: projects}
}

type File struct {
	Project string
	Path    string
	Source  []byte
}

func (s *ProjectService) Resolve(selector string) (project.Entry, error) {
	if strings.TrimSpace(selector) == "" {
		return project.Entry{}, fmt.Errorf("project must not be blank")
	}
	if filepath.IsAbs(selector) {
		return s.projects.GetByPath(selector)
	}
	return s.projects.Resolve(selector)
}

func cleanRelativePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be a non-empty repository-relative path")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes project root", path)
	}
	return clean, nil
}

func (s *ProjectService) ReadFile(selector, path string) (File, error) {
	entry, err := s.Resolve(selector)
	if err != nil {
		return File{}, err
	}
	clean, err := cleanRelativePath(path)
	if err != nil {
		return File{}, err
	}
	root, err := os.OpenRoot(entry.Path)
	if err != nil {
		return File{}, fmt.Errorf("open project root: %w", err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(clean, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return File{}, fmt.Errorf("open project file %q: %w", clean, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return File{}, err
	}
	if !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("project path %q is not a regular file", clean)
	}
	if info.Size() > maxSourceBytes {
		return File{}, fmt.Errorf("project file %q exceeds 16 MiB", clean)
	}
	source, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil {
		return File{}, err
	}
	if len(source) > maxSourceBytes {
		return File{}, fmt.Errorf("project file %q exceeds 16 MiB", clean)
	}
	if err := ValidateText(source); err != nil {
		return File{}, fmt.Errorf("project file %q: %w", clean, err)
	}
	if bytes.HasPrefix(source, []byte("GIF")) ||
		(bytes.HasPrefix(source, []byte("RIFF")) && len(source) >= 12 && string(source[8:12]) == "WEBP") {
		return File{}, fmt.Errorf("project file %q: binary image file", clean)
	}
	return File{Project: entry.Alias, Path: filepath.ToSlash(clean), Source: source}, nil
}

var binarySignatures = [][]byte{
	[]byte("%PDF-"), []byte("PK\x03\x04"), []byte("PK\x05\x06"), []byte("PK\x07\x08"),
	{0x7f, 'E', 'L', 'F'}, {0xfe, 0xed, 0xfa, 0xce}, {0xce, 0xfa, 0xed, 0xfe},
	{0xfe, 0xed, 0xfa, 0xcf}, {0xcf, 0xfa, 0xed, 0xfe},
	{0xca, 0xfe, 0xba, 0xbe}, {0xbe, 0xba, 0xfe, 0xca}, {0xca, 0xfe, 0xba, 0xbf},
	{0xbf, 0xba, 0xfe, 0xca}, {0, 'a', 's', 'm'},
}

// IsBinaryBytes reports common binary signatures and NUL bytes in the first 8 KiB.
func IsBinaryBytes(source []byte) bool {
	check := source
	if len(check) > 8192 {
		check = check[:8192]
	}
	if bytes.IndexByte(check, 0) >= 0 {
		return true
	}
	for _, sig := range binarySignatures {
		if bytes.HasPrefix(source, sig) {
			return true
		}
	}
	return false
}

// ValidateText rejects binary signatures and invalid UTF-8 before source parsing.
func ValidateText(source []byte) error {
	if IsBinaryBytes(source) {
		return fmt.Errorf("binary file")
	}
	if !utf8.Valid(source) {
		return fmt.Errorf("invalid UTF-8 text")
	}
	return nil
}
