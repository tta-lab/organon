package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectsPath(t *testing.T) {
	p := ProjectsPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "ttal", "projects.toml")) {
		t.Errorf("unexpected path: %s", p)
	}
}

func TestOrgsPath(t *testing.T) {
	p := OrgsPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "ttal", "orgs.toml")) {
		t.Errorf("unexpected path: %s", p)
	}
}

func TestWebConfigPath(t *testing.T) {
	p := WebConfigPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "ttal", "web.toml")) {
		t.Errorf("unexpected path: %s", p)
	}
}

func TestOGConfigPath(t *testing.T) {
	p := OGConfigPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "ttal", "og.toml")) {
		t.Errorf("unexpected path: %s", p)
	}
}

func TestDefaultReferencesPath(t *testing.T) {
	p := DefaultReferencesPath()
	if !strings.HasSuffix(p, filepath.Join("code", "references")) {
		t.Errorf("unexpected path: %s", p)
	}
}
