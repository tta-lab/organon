package srcview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/project"
)

type fixedProjects map[string]project.Entry

func (p fixedProjects) Get(alias string) (project.Entry, error) {
	entry, ok := p[alias]
	if !ok {
		return project.Entry{}, project.ErrNotFound
	}
	return entry, nil
}

func TestProjectServiceAllowsOnlyUTF8RegularFilesInsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "valid.txt"), []byte("héllo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.dat"), []byte{'a', 0, 'b'}, 0644); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("valid.txt", filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	service := NewProjectService(fixedProjects{"ko": {Alias: "ko", Path: root}})

	for _, path := range []string{"valid.txt", "inside-link"} {
		file, err := service.ReadFile("ko", path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if string(file.Source) != "héllo\n" {
			t.Fatalf("source = %q", file.Source)
		}
	}
	for _, path := range []string{"../secret.txt", outsideFile, ".", "outside-link", "binary.dat", "missing"} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			if _, err := service.ReadFile("ko", path); err == nil {
				t.Fatalf("ReadFile(%q) succeeded", path)
			}
		})
	}
}

func TestProjectServiceRejectsOversizedFileBeforeReading(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "generated.txt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maximumSourceBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	service := NewProjectService(fixedProjects{"ko": {Alias: "ko", Path: root}})
	if _, err := service.ReadFile("ko", "generated.txt"); err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("ReadFile oversized error = %v", err)
	}
}
