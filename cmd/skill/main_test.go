package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var skillBin string

func TestMain(m *testing.M) {
	origCwd, _ := os.Getwd()
	bin, err := os.CreateTemp("", "skill-test-*")
	if err != nil {
		os.Exit(1)
	}
	bin.Close()
	binPath := bin.Name()
	defer os.Remove(binPath)

	cmd := exec.Command("go", "build", "-o", binPath, "github.com/tta-lab/organon/cmd/skill")
	cmd.Dir = origCwd
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
	skillBin = binPath
	os.Exit(m.Run())
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

func runSkill(t *testing.T, root, home string, args []string) (
	stdout, stderr string, exitCode int,
) {
	t.Helper()

	if home != "" {
		t.Setenv("HOME", home)
	}

	cmd := exec.Command(skillBin, args...)
	cmd.Dir = root
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err := cmd.Run()

	return strings.TrimSpace(outBuf.String()),
		strings.TrimSpace(errBuf.String()),
		exitCodeFromErr(err)
}

func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func TestSkillList_Empty(t *testing.T) {
	tmp := t.TempDir()
	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"list"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Fatalf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
	_ = stdout
}

func TestSkillList_OneSkill(t *testing.T) {
	tmp := t.TempDir()
	writeSkillAt(t, tmp, "my-skill", "A test skill", "tool", "skill body content")

	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"list"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
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
	_ = stderr
}

func TestSkillGet_Found(t *testing.T) {
	tmp := t.TempDir()
	writeSkillAt(t, tmp, "my-skill", "A test skill", "tool", "skill body content")

	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"get", "my-skill"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "skill body content") {
		t.Fatalf("stdout = %q, want to contain 'skill body content'", stdout)
	}
	_ = stderr
}

func TestSkillGet_NotFound(t *testing.T) {
	tmp := t.TempDir()
	_, stderr, code := runSkill(t, tmp, tmp, []string{"get", "does-not-exist"})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "does-not-exist") {
		t.Fatalf("stderr = %q, want to contain 'does-not-exist'", stderr)
	}
}

func TestSkillFind_Match(t *testing.T) {
	tmp := t.TempDir()
	writeSkillAt(t, tmp, "git-omz", "Git plugin abbreviations", "tool", "body")

	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"find", "git"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "git-omz") {
		t.Fatalf("output = %q, want to contain 'git-omz'", stdout)
	}
	_ = stderr
}

func TestSkillFind_NoMatch(t *testing.T) {
	tmp := t.TempDir()
	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"find", "nonexistent"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Fatalf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
	_ = stdout
}

func TestSkillList_EmptyCategoryRendersDash(t *testing.T) {
	tmp := t.TempDir()
	writeSkillAt(t, tmp, "no-category-skill", "A skill with no category", "", "body")

	stdout, stderr, code := runSkill(t, tmp, tmp, []string{"list"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, " - ") {
		t.Fatalf("output should contain '-' for empty category, got: %q", stdout)
	}
	if !strings.Contains(stdout, "no-category-skill") {
		t.Fatalf("output should contain 'no-category-skill', got: %q", stdout)
	}
}
