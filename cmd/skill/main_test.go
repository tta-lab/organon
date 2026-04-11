package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func runSkill(t *testing.T, root, home string, args []string) (stdout, stderr string, exitCode int) {
	t.Helper()

	origCwd, _ := os.Getwd()
	binPath := filepath.Join(origCwd, "..", "..", "skill")

	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir %q: %v", root, err)
	}
	t.Cleanup(func() { _ = os.Chdir(origCwd) })

	if home != "" {
		t.Setenv("HOME", home)
	}

	cmd := exec.Command(binPath, args...)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	stdout = strings.TrimSpace(cmd.Stdout.(*bytes.Buffer).String())
	stderr = strings.TrimSpace(cmd.Stderr.(*bytes.Buffer).String())

	if err == nil {
		return stdout, stderr, 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, exitErr.ExitCode()
	}
	t.Fatalf("exec error: %v", err)
	return stdout, stderr, 1
}

func TestSkillList_Empty(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	_, stderr, exit := runSkill(t, root, home, []string{"list"})
	if exit != 0 {
		t.Errorf("exit code = %d, want 0", exit)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Errorf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
}

func TestSkillList_OneSkill(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeSkillAt(t, root, "my-skill", "A useful skill", "tools", "skill body content")

	stdout, _, exit := runSkill(t, root, home, []string{"list"})
	if exit != 0 {
		t.Errorf("exit code = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "my-skill") {
		t.Errorf("output = %q, want to contain 'my-skill'", stdout)
	}
	headers := []string{"NAME", "CATEGORY", "SOURCE", "DESCRIPTION"}
	for _, h := range headers {
		if !strings.Contains(stdout, h) {
			t.Errorf("output missing header %q, got: %q", h, stdout)
		}
	}
	if strings.Contains(stdout, "MATCH") {
		t.Errorf("output should not contain MATCH column, got: %q", stdout)
	}
	if !strings.Contains(stdout, "~") && !strings.Contains(stdout, ".agents/skills") {
		t.Errorf("output should contain abbreviated home path with ~ or path with .agents/skills, got: %q", stdout)
	}
}

func TestSkillGet_Found(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeSkillAt(t, root, "my-skill", "A useful skill", "tools", "skill body content")

	stdout, _, exit := runSkill(t, root, home, []string{"get", "my-skill"})
	if exit != 0 {
		t.Errorf("exit code = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "skill body content") {
		t.Errorf("stdout = %q, want to contain 'skill body content'", stdout)
	}
}

func TestSkillGet_NotFound(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeSkillAt(t, root, "my-skill", "A useful skill", "tools", "skill body")

	_, stderr, exit := runSkill(t, root, home, []string{"get", "nonexistent"})
	if exit == 0 {
		t.Errorf("exit code = 0, want non-zero")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want to contain 'not found'", stderr)
	}
}

func TestSkillFind_Match(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeSkillAt(t, root, "git-omz", "git plugin abbreviations", "git", "git body")
	writeSkillAt(t, root, "taskwarrior", "task management", "tools", "task body")

	stdout, _, exit := runSkill(t, root, home, []string{"find", "git"})
	if exit != 0 {
		t.Errorf("exit code = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "git-omz") {
		t.Errorf("output = %q, want to contain 'git-omz'", stdout)
	}
	if strings.Contains(stdout, "taskwarrior") {
		t.Errorf("output should not contain 'taskwarrior', got: %q", stdout)
	}
}

func TestSkillFind_NoMatch(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeSkillAt(t, root, "taskwarrior", "task management", "tools", "task body")

	_, stderr, exit := runSkill(t, root, home, []string{"find", "nonexistent"})
	if exit != 0 {
		t.Errorf("exit code = %d, want 0", exit)
	}
	if !strings.Contains(stderr, "No skills found.") {
		t.Errorf("stderr = %q, want to contain 'No skills found.'", stderr)
	}
}
