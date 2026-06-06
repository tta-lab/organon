package project

import (
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
	}
	if e.Path != "/home/neil/code/projects/tta-lab/organon" {
		t.Errorf("unexpected path: %s", e.Path)
	}
}

func TestResolveHierarchical(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "projects.toml")
	os.WriteFile(p, []byte(`
[fb]
name = "FlickNote Backend"
path = "/home/neil/code/projects/GuionAI/flick-backend"

[fb.ap]
name = "Ai Processor"
path = "/home/neil/code/projects/GuionAI/flick-backend/services/ai-processor"
`), 0644)

	e, err := Resolve(p, "fb.ap")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
	}
	if e.Alias != "fb.ap" {
		t.Errorf("expected fb.ap, got %s", e.Alias)
	}

	e, err = Resolve(p, "fb.zz")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected fallback entry, got nil")
	}
	if e.Alias != "fb" {
		t.Errorf("expected fallback to fb, got %s", e.Alias)
	}
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
