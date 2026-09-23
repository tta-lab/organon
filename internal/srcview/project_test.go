package srcview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tta-lab/organon/internal/project"
	"golang.org/x/sys/unix"
)

type fixedProjectRoot string

func (root fixedProjectRoot) Resolve(selector string) (project.Entry, error) {
	return project.Entry{Alias: "ko", Path: string(root)}, nil
}
func (root fixedProjectRoot) GetByPath(path string) (project.Entry, error) {
	return project.Entry{Alias: "ko", Path: string(root)}, nil
}

func TestProjectServiceContainedRegularFile(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "file.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/file.txt", filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape-link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	files := NewProjectService(fixedProjectRoot(root))
	for _, path := range []string{"nested/file.txt", "inside-link"} {
		file, err := files.ReadFile("ko", path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if string(file.Source) != "inside" {
			t.Fatalf("%s: %q", path, file.Source)
		}
	}
	for _, path := range []string{"../secret.txt", filepath.Join(outside, "secret.txt"),
		"escape-link", "nested", "pipe"} {
		if _, err := files.ReadFile("ko", path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}
