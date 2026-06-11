package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runProject(t *testing.T, args []string) (stdout string, err error) {
	t.Helper()

	readOut, writeOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = writeOut
	t.Cleanup(func() { os.Stdout = origStdout })

	cmd := newRootCmd()
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	if err := writeOut.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, readOut); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return buf.String(), execErr
}

func writeProjectsConfig(t *testing.T, home string, content string) {
	t.Helper()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write projects.toml: %v", err)
	}
}

func TestProjectList_PrintsModelFriendlyBullets(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `
[len]
name = "Lenos CLI runtime"
path = "/home/neil/code/projects/tta-lab/lenos"

[orientation]
path = "/home/neil/code/projects/tta-lab/orientation"
`)

	stdout, err := runProject(t, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Available projects:\n" +
		"- len: Lenos CLI runtime (path: /home/neil/code/projects/tta-lab/lenos)\n" +
		"- orientation: /home/neil/code/projects/tta-lab/orientation\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	for _, unwanted := range []string{"ALIAS", "ORG", "NAME"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("stdout should not contain %q, got: %q", unwanted, stdout)
		}
	}
}

func TestProjectList_JSONOutputUnchanged(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `
[len]
name = "Lenos CLI runtime"
path = "/home/neil/code/projects/tta-lab/lenos"
`)

	stdout, err := runProject(t, []string{"list", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var projects []map[string]any
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\noutput: %q", err, stdout)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}
	if projects[0]["alias"] != "len" || projects[0]["path"] != "/home/neil/code/projects/tta-lab/lenos" {
		t.Fatalf("unexpected JSON project: %v", projects[0])
	}
}

func TestProjectList_Empty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	stdout, err := runProject(t, []string{"list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "No projects found.\n" {
		t.Fatalf("stdout = %q, want empty message", stdout)
	}
}
