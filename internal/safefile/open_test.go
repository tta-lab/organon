package safefile

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenContainedAllowsInternalLinkAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	insidePath := filepath.Join(root, "inside.txt")
	outsidePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(insidePath, []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside.txt", filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}

	file, err := OpenContained(root, "inside-link")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(data) != "inside" {
		t.Fatalf("read = %q, err = %v", data, err)
	}
	if _, err := OpenContained(root, "outside-link"); err == nil {
		t.Fatal("outside link opened successfully")
	}
	if err := CheckContained(root, filepath.Dir(outsidePath)); err == nil {
		t.Fatal("outside directory passed containment check")
	}
}
