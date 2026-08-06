package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")
	os.WriteFile(p, []byte(`
[organon]
name = "Organon"
path = "/home/neil/code/projects/tta-lab/organon"

[len]
name = "Lenos agent cli"
path = "/home/neil/code/projects/tta-lab/lenos"
`), 0644)

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Alias != "len" || entries[1].Alias != "organon" {
		t.Errorf("expected sorted aliases, got %v", entries)
	}
}
func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")

	entries, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")
	os.WriteFile(p, []byte(`
[organon]
name = "Organon"
path = "/home/neil/code/projects/tta-lab/organon"
`), 0644)

	e, err := Get(p, "organon")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
		return
	}
	if e.Path != "/home/neil/code/projects/tta-lab/organon" {
		t.Errorf("unexpected path: %s", e.Path)
	}
}

func TestCatalogGetExactDoesNotFallBackToParentAlias(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")
	os.WriteFile(p, []byte(`
[fb]
name = "FlickNote Backend"
path = "/home/neil/code/projects/GuionAI/flick-backend"
`), 0644)

	catalog, err := OpenCatalog(p)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	if _, err := catalog.GetExact("fb.zz"); !errors.Is(err, ErrInvalidAlias) {
		t.Fatalf("GetExact(fb.zz) error = %v, want ErrInvalidAlias", err)
	}
}

func TestOpenCatalogRejectsInvalidActiveAliases(t *testing.T) {
	tests := map[string]string{
		"quoted dotted key": `["fse.gw"]
path = "/projects/fse-gw"
`,
		"nested active table": `[fse.gw]
path = "/projects/fse-gw"
`,
		"slash": `["fse/gw"]
path = "/projects/fse-gw"
`,
		"whitespace": `["fse gw"]
path = "/projects/fse-gw"
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeProjectFile(t, content)
			if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidAlias) {
				t.Fatalf("OpenCatalog error = %v, want ErrInvalidAlias", err)
			}
		})
	}
}

func TestOpenCatalogSkipsArchivedNamespace(t *testing.T) {
	path := writeProjectFile(t, `[active]
name = "Active"
path = "/projects/active"

[archived.fse.gw]
name = "Archived gateway"
path = "/projects/fse-gw"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	entries := catalog.List("")
	if len(entries) != 1 || entries[0].Alias != "active" {
		t.Fatalf("List = %+v, want only active", entries)
	}
}

func TestOpenCatalogValidatesPathsAndUniqueness(t *testing.T) {
	tests := map[string]string{
		"empty": `[one]
path = ""
`,
		"relative": `[one]
path = "projects/one"
`,
		"duplicate cleaned": `[one]
path = "/projects/shared/../repo"

[two]
path = "/projects/repo"
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeProjectFile(t, content)
			if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("OpenCatalog error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestCatalogGetByPathUsesCleanedAbsolutePath(t *testing.T) {
	path := writeProjectFile(t, `[one]
path = "/projects/shared/../one"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	entry, err := catalog.GetByPath("/projects/one/.")
	if err != nil {
		t.Fatalf("GetByPath: %v", err)
	}
	if entry.Alias != "one" || entry.Path != "/projects/one" {
		t.Fatalf("entry = %+v", entry)
	}
}

func writeProjectFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write projects file: %v", err)
	}
	return path
}

func TestDeriveOrg(t *testing.T) {
	tests := []struct {
		path string
		org  string
	}{
		{"/home/neil/code/projects/tta-lab/organon", "tta-lab"},
		{"/home/neil/code/projects/GuionAI/flick-backend", "GuionAI"},
		{"/home/neil/code/references/github.com/tta-lab/agon", "tta-lab"},
		{"/home/neil/code/projects/neil/sustech-mar-slides", "neil"},
	}

	for _, tt := range tests {
		got := DeriveOrg(tt.path)
		if got != tt.org {
			t.Errorf("DeriveOrg(%q) = %q, want %q", tt.path, got, tt.org)
		}
	}
}

func TestListFiltered(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")
	os.WriteFile(p, []byte(`
[organon]
name = "Organon"
path = "/home/neil/code/projects/tta-lab/organon"

[fb]
name = "FlickNote Backend"
path = "/home/neil/code/projects/GuionAI/flick-backend"
`), 0644)

	all, err := ListFiltered(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2, got %d", len(all))
	}

	tta, err := ListFiltered(p, "tta-lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(tta) != 1 {
		t.Errorf("expected 1 tta-lab project, got %d", len(tta))
	}
	if tta[0].Alias != "organon" {
		t.Errorf("expected organon, got %s", tta[0].Alias)
	}
}
