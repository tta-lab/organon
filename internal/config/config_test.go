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

func TestDefaultReferencesPath(t *testing.T) {
	p := DefaultReferencesPath()
	if !strings.HasSuffix(p, filepath.Join("code", "references")) {
		t.Errorf("unexpected path: %s", p)
	}
}
