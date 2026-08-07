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
	entries := catalog.ListAll(false)
	if len(entries) != 1 || entries[0].Alias != "active" {
		t.Fatalf("List = %+v, want only active", entries)
	}
}

func TestOpenCatalogExposesArchivedEntriesOnRequest(t *testing.T) {
	path := writeProjectFile(t, `[active]
name = "Active"
path = "/projects/active"

[archived.ttal]
name = "Archived TTAL"
path = "/projects/ttal"
remote = "historical context is preserved"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}

	entries := catalog.ListAll(true)
	if len(entries) != 2 {
		t.Fatalf("ListAll(true) = %+v, want active and archived", entries)
	}
	if entries[0].Alias != "active" || entries[0].Archived {
		t.Fatalf("active entry = %+v", entries[0])
	}
	if entries[1].Alias != "ttal" || !entries[1].Archived {
		t.Fatalf("archived entry = %+v", entries[1])
	}
	archived, err := catalog.GetExact("ttal")
	if err != nil {
		t.Fatalf("GetExact(ttal): %v", err)
	}
	if !archived.Archived || archived.Path != "/projects/ttal" {
		t.Fatalf("GetExact(ttal) = %+v", archived)
	}
}

func TestOpenCatalogRejectsObsoleteActiveMetadata(t *testing.T) {
	for _, key := range []string{"org", "github_token_env", "remote", "k8s_app", "k8s_namespace"} {
		t.Run(key, func(t *testing.T) {
			path := writeProjectFile(t, "[one]\npath = \"/projects/one\"\n"+key+" = \"obsolete\"\n")
			if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("OpenCatalog error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestOpenCatalogUsesActiveEntryForDuplicateArchivedPath(t *testing.T) {
	path := writeProjectFile(t, `[active]
path = "/projects/shared"

[archived.old]
path = "/projects/shared"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	entry, err := catalog.GetByPath("/projects/shared")
	if err != nil {
		t.Fatalf("GetByPath: %v", err)
	}
	if entry.Alias != "active" || entry.Archived {
		t.Fatalf("GetByPath = %+v, want active entry", entry)
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
