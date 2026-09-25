package srcview

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

//nolint:gocyclo // One fixture covers listing and containment.
func TestListDirectoryShowsDirectChildrenAndKeepsPathsContained(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	for _, name := range []string{"nested", ".git"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "file.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	files := NewProjectService(fixedProjectRoot(root))
	top, err := files.ListDirectory("ko", "")
	if err != nil {
		t.Fatal(err)
	}
	if top.Path != "." || len(top.Entries) != 3 ||
		top.Entries[0].Name != "escape" || top.Entries[0].Type != "symlink" ||
		top.Entries[1].Path != "nested" || top.Entries[1].Type != "directory" ||
		top.Entries[2].Path != "new.txt" || top.Entries[2].Type != "file" {
		t.Fatalf("top = %+v", top)
	}
	nested, err := files.ListDirectory("ko", "nested")
	if err != nil || len(nested.Entries) != 1 || nested.Entries[0].Path != "nested/file.txt" {
		t.Fatalf("nested = %+v, %v", nested, err)
	}
	for _, path := range []string{"../outside", outside, "escape", "new.txt"} {
		if _, err := files.ListDirectory("ko", path); err == nil {
			t.Fatalf("accepted path %q", path)
		}
	}
}

func TestListDirectoryBoundsLargeDirectory(t *testing.T) {
	root := t.TempDir()
	for i := range MaxListEntries + 1 {
		name := fmt.Sprintf("%03d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := NewProjectService(fixedProjectRoot(root)).ListDirectory("ko", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != MaxListEntries || !result.Truncated ||
		result.Entries[0].Name != "000.txt" || result.Entries[MaxListEntries-1].Name != "199.txt" {
		t.Fatalf("list = %+v", result)
	}
}
