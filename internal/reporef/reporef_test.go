package reporef

import (
	"os"
	"path/filepath"
	"strings"
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

func TestListReturnsReferenceRepositoriesInDeterministicOrder(t *testing.T) {
	references := t.TempDir()
	paths := []string{
		filepath.Join(references, "gitlab.com", "example", "zeta"),
		filepath.Join(references, "github.com", "tta-lab", "organon"),
		filepath.Join(references, "github.com", "other", "alpha"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := List(references)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("List() = %+v, want 3 entries", entries)
	}
	want := []Entry{
		{Host: "github.com", Owner: "other", Repo: "alpha", Path: paths[2]},
		{Host: "github.com", Owner: "tta-lab", Repo: "organon", Path: paths[1]},
		{Host: "gitlab.com", Owner: "example", Repo: "zeta", Path: paths[0]},
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

func TestListMissingRootIsEmpty(t *testing.T) {
	entries, err := List(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("List() = %+v, want non-nil empty slice", entries)
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

func TestResolveMissingReferenceDoesNotClone(t *testing.T) {
	references := t.TempDir()
	_, err := Resolve("tta-lab/missing", references)
	if err == nil {
		t.Fatal("Resolve missing reference succeeded")
	}
	if !strings.Contains(err.Error(), "og clone --reference") {
		t.Fatalf("Resolve error = %v, want og clone --reference guidance", err)
	}
	if _, statErr := os.Stat(filepath.Join(references, "github.com", "tta-lab", "missing")); !os.IsNotExist(statErr) {
		t.Fatalf("missing reference path was created: %v", statErr)
	}
}
