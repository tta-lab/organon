package reporef

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOrgRepo(t *testing.T) {
	org, repo, ok := parseOrgRepo("tta-lab/agon")
	if !ok {
		t.Fatal("expected ok")
	}
	if org != "tta-lab" || repo != "agon" {
		t.Errorf("unexpected: %s/%s", org, repo)
	}

	_, _, ok = parseOrgRepo("barename")
	if ok {
		t.Fatal("expected not ok for bare name")
	}

	_, _, ok = parseOrgRepo("a/b/c")
	if ok {
		t.Fatal("expected not ok for triple")
	}
}

func TestIsSafePathPart(t *testing.T) {
	if !isSafePathPart("hello") {
		t.Error("expected safe")
	}
	if isSafePathPart("") {
		t.Error("expected unsafe")
	}
	if isSafePathPart("..") {
		t.Error("expected unsafe")
	}
	if isSafePathPart("a/b") {
		t.Error("expected unsafe")
	}
}

func TestFindClonedRepo(t *testing.T) {
	dir := t.TempDir()
	refsPath := filepath.Join(dir, "references")
	hostPath := filepath.Join(refsPath, "github.com")
	orgPath := filepath.Join(hostPath, "tta-lab")
	repoPath := filepath.Join(orgPath, "agon")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}

	path, err := FindClonedRepo("agon", refsPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != repoPath {
		t.Errorf("expected %s, got %s", repoPath, path)
	}
}

func TestFindClonedRepoNotFound(t *testing.T) {
	dir := t.TempDir()
	refsPath := filepath.Join(dir, "references")

	_, err := FindClonedRepo("nope", refsPath)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	refsPath := filepath.Join(dir, "references")
	hostPath := filepath.Join(refsPath, "github.com")
	orgPath := filepath.Join(hostPath, "tta-lab")
	repoPath := filepath.Join(orgPath, "agon")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}

	path, err := Resolve("tta-lab/agon", refsPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != repoPath {
		t.Errorf("expected %s, got %s", repoPath, path)
	}
}

func TestDeriveOrg(t *testing.T) {
	got := DeriveOrg("/home/neil/code/references/github.com/tta-lab/agon")
	if got != "tta-lab" {
		t.Errorf("expected tta-lab, got %s", got)
	}
}
