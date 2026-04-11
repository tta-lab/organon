package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runSkill(t *testing.T, args []string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	home := os.Getenv("HOME")
	paths := []string{}
	if home != "" {
		paths, _ = resolvePaths(home) //nolint:errcheck // test isolation
	}
	cmd := newRootCmd(&outBuf, &errBuf, paths, home)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func writeSkillAt(t *testing.T, root, name, desc, category, body string) {
	t.Helper()
	dir := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	content := "---\n"
	if name != "" || desc != "" || category != "" {
		if name != "" {
			content += "name: " + name + "\n"
		}
		if desc != "" {
			content += "description: " + desc + "\n"
		}
		if category != "" {
			content += "category: " + category + "\n"
		}
	}
	content += "---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestSkillList_Empty(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	_, stderr, err := runSkill(t, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Fatalf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
}

func TestSkillList_OneSkill(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSkillAt(t, tmpHome, "my-skill", "A test skill", "tool", "skill body content")

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	stdout, _, err := runSkill(t, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, h := range []string{"NAME", "CATEGORY", "SOURCE", "DESCRIPTION"} {
		if !strings.Contains(stdout, h) {
			t.Fatalf("output missing header %q, got: %q", h, stdout)
		}
	}
	if strings.Contains(stdout, "MATCH") {
		t.Fatalf("output should not contain a MATCH column, got: %q", stdout)
	}
	if !strings.Contains(stdout, "my-skill") {
		t.Fatalf("output should contain 'my-skill', got: %q", stdout)
	}
	if !strings.Contains(stdout, "~") && !strings.Contains(stdout, ".agents/skills") {
		t.Fatalf("output should contain abbreviated home path with ~ or path with .agents/skills, got: %q", stdout)
	}
}

func TestSkillGet_Found(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSkillAt(t, tmpHome, "my-skill", "A test skill", "tool", "skill body content")

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	stdout, _, err := runSkill(t, []string{"get", "my-skill"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "skill body content") {
		t.Fatalf("stdout = %q, want to contain 'skill body content'", stdout)
	}
}

func TestSkillGet_NotFound(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	_, stderr, err := runSkill(t, []string{"get", "does-not-exist"})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(stderr, "does-not-exist") {
		t.Fatalf("error = %v, stderr = %q, want 'not found' in either", err, stderr)
	}
}

func TestSkillFind_Match(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSkillAt(t, tmpHome, "git-omz", "Git plugin abbreviations", "tool", "body")

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	stdout, _, err := runSkill(t, []string{"find", "git"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "git-omz") {
		t.Fatalf("output = %q, want to contain 'git-omz'", stdout)
	}
}

func TestSkillFind_NoMatch(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	_, stderr, err := runSkill(t, []string{"find", "nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Fatalf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
}

func TestSkillList_EmptyCategoryRendersDash(t *testing.T) {
	tmp := t.TempDir()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSkillAt(t, tmpHome, "no-category-skill", "A skill with no category", "", "body")

	origCwd, _ := os.Getwd()
	os.Chdir(tmp)                           //nolint:errcheck // test isolation
	t.Cleanup(func() { os.Chdir(origCwd) }) //nolint:errcheck // test isolation

	stdout, _, err := runSkill(t, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, " - ") {
		t.Fatalf("output should contain '-' for empty category, got: %q", stdout)
	}
	if !strings.Contains(stdout, "no-category-skill") {
		t.Fatalf("output should contain 'no-category-skill', got: %q", stdout)
	}
}
