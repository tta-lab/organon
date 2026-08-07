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
remote = "https://github.com/tta-lab/organon.git"

[len]
name = "Lenos agent cli"
path = "/home/neil/code/projects/tta-lab/lenos"
remote = "https://github.com/tta-lab/lenos.git"
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
remote = "https://github.com/tta-lab/organon.git"
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
remote = "https://github.com/guionai/flick-backend.git"
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

func TestOpenCatalogRejectsNestedArchivedAlias(t *testing.T) {
	path := writeProjectFile(t, `[active]
name = "Active"
path = "/projects/active"
remote = "https://example.com/owner/active.git"

[archived.fse.gw]
name = "Archived gateway"
path = "/projects/fse-gw"
remote = "https://example.com/owner/gateway.git"
`)
	if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidAlias) {
		t.Fatalf("OpenCatalog error = %v, want ErrInvalidAlias", err)
	}
}

func TestOpenCatalogExposesArchivedEntriesOnRequest(t *testing.T) {
	path := writeProjectFile(t, `[active]
name = "Active"
path = "/projects/active"
remote = "https://example.com/owner/active.git"

[archived.ttal]
name = "Archived TTAL"
path = "/projects/ttal"
remote = "https://example.com/owner/ttal.git"
historical_context = "preserved"
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
	for _, key := range []string{"org", "github_token_env", "k8s_app", "k8s_namespace"} {
		t.Run(key, func(t *testing.T) {
			content := "[one]\npath = \"/projects/one\"\n" +
				"remote = \"https://example.com/owner/one.git\"\n" + key + " = \"obsolete\"\n"
			path := writeProjectFile(t, content)
			if _, err := OpenCatalog(path); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("OpenCatalog error = %v, want ErrInvalidConfig", err)
			}
		})
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
remote = "https://example.com/owner/one.git"

[two]
path = "/projects/repo"
remote = "https://example.com/owner/two.git"
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
remote = "https://example.com/owner/one.git"
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

func TestOpenCatalogRequiresCanonicalUniqueRemotesAcrossArchiveStates(t *testing.T) {
	tests := map[string]string{
		"missing active remote": `[one]
path = "/projects/one"
`,
		"noncanonical active remote": `[one]
path = "/projects/one"
remote = "https://github.com/TTA-Lab/Organon"
`,
		"missing archived remote": `[archived.one]
path = "/projects/one"
`,
		"duplicate remote": `[one]
path = "/projects/one"
remote = "https://github.com/tta-lab/organon.git"

[archived.old]
path = "/projects/old"
remote = "https://github.com/tta-lab/organon.git"
`,
		"duplicate path across states": `[one]
path = "/projects/shared"
remote = "https://example.com/owner/one.git"

[archived.old]
path = "/projects/shared"
remote = "https://example.com/owner/old.git"
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := OpenCatalog(writeProjectFile(t, content)); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("OpenCatalog error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestCatalogGetByRemoteUsesCanonicalExactIdentity(t *testing.T) {
	path := writeProjectFile(t, `[one]
name = "One"
path = "/projects/one"
remote = "https://github.com/tta-lab/organon.git"
`)
	catalog, err := OpenCatalog(path)
	if err != nil {
		t.Fatalf("OpenCatalog: %v", err)
	}
	entry, err := catalog.GetByRemote("https://github.com/TTA-LAB/ORGANON")
	if err != nil {
		t.Fatalf("GetByRemote: %v", err)
	}
	if entry.Remote != "https://github.com/tta-lab/organon.git" || entry.Name != "One" {
		t.Fatalf("GetByRemote = %+v", entry)
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
