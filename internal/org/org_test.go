package org

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")
	os.WriteFile(p, []byte(`
[tta-lab]

[guionai]
`), 0644)

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Name != "guionai" || entries[1].Name != "tta-lab" {
		t.Errorf("expected sorted, got %v", entries)
	}
}

func TestCatalogUsesExactSingleLayerNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orgs.toml")
	if err := os.WriteFile(path, []byte("[tta-lab]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	if _, err := catalog.GetExact("tta-lab.child"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("GetExact error = %v, want ErrInvalidName", err)
	}
	if _, err := catalog.GetExact("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetExact error = %v, want ErrNotFound", err)
	}
}

func TestOpenCatalogRejectsNestedOrg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orgs.toml")
	if err := os.WriteFile(path, []byte("[tta.child]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("OpenCatalog error = %v, want ErrInvalidName", err)
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")
	os.WriteFile(p, []byte(`
[tta-lab]
`), 0644)

	e, err := Get(p, "tta-lab")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
		return
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "orgs.toml")

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0, got %d", len(entries))
	}
}
