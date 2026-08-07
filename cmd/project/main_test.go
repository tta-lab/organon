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
	if archived, ok := projects[0]["archived"]; !ok || archived != false {
		t.Fatalf("project archived field = %#v, want required false", projects[0]["archived"])
	}
}

func TestProjectListIncludesArchivedOnlyWhenRequested(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[active]
path = "/projects/active"

[archived.ttal]
path = "/projects/ttal"
`)

	stdout, err := runProject(t, []string{"list", "--include-archived", "--json"})
	if err != nil {
		t.Fatalf("project list: %v", err)
	}
	var projects []map[string]any
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	if len(projects) != 2 || projects[1]["alias"] != "ttal" || projects[1]["archived"] != true {
		t.Fatalf("projects = %#v, want archived ttal", projects)
	}

	if _, err := runProject(t, []string{"list", "tta-lab"}); err == nil {
		t.Fatal("project list accepted removed org argument")
	}
}

func TestProjectGetReturnsArchivedEntry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, "[archived.ttal]\npath = \"/projects/ttal\"\n")

	stdout, err := runProject(t, []string{"get", "ttal", "--json"})
	if err != nil {
		t.Fatalf("project get: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode project get: %v", err)
	}
	if got["alias"] != "ttal" || got["archived"] != true {
		t.Fatalf("project get = %#v", got)
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

func TestProjectGetRejectsDottedAliasWithoutReferenceFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `
[fse]
name = "FSE"
path = "/projects/fse"
`)

	stdout, err := runProject(t, []string{"get", "fse.gw", "--json"})
	if err == nil {
		t.Fatal("expected dotted alias error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(err.Error(), "invalid project alias") {
		t.Fatalf("error = %v, want invalid project alias", err)
	}
}

func TestProjectCommandsPreserveOrgRepoReferenceLookup(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoPath := filepath.Join(tmpHome, "code", "references", "github.com", "tta-lab", "demo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir reference repo: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want func(t *testing.T, stdout string)
	}{
		{
			name: "get",
			args: []string{"get", "tta-lab/demo", "--json"},
			want: func(t *testing.T, stdout string) {
				t.Helper()
				var got map[string]any
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("decode get output: %v", err)
				}
				if got["alias"] != "tta-lab/demo" || got["path"] != repoPath || got["archived"] != false {
					t.Fatalf("get output = %#v", got)
				}
				if _, exists := got["org"]; exists {
					t.Fatalf("get output still exposes org: %#v", got)
				}
			},
		},
		{
			name: "resolve",
			args: []string{"resolve", "tta-lab/demo"},
			want: func(t *testing.T, stdout string) {
				t.Helper()
				var got map[string]any
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("decode resolve output: %v", err)
				}
				if got["alias"] != "tta-lab/demo" || got["path"] != repoPath || got["archived"] != false {
					t.Fatalf("resolve output = %#v", got)
				}
				if _, exists := got["org"]; exists {
					t.Fatalf("resolve output still exposes org: %#v", got)
				}
			},
		},
		{
			name: "jump",
			args: []string{"jump", "tta-lab/demo"},
			want: func(t *testing.T, stdout string) {
				t.Helper()
				if stdout != repoPath+"\n" {
					t.Fatalf("jump output = %q, want %q", stdout, repoPath+"\n")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, err := runProject(t, tt.args)
			if err != nil {
				t.Fatalf("run project %v: %v", tt.args, err)
			}
			tt.want(t, stdout)
		})
	}
}
