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
remote = "https://github.com/tta-lab/lenos.git"

[orientation]
path = "/home/neil/code/projects/tta-lab/orientation"
remote = "https://github.com/tta-lab/orientation.git"
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
remote = "https://github.com/tta-lab/lenos.git"
`)

	stdout, err := runProject(t, []string{"list", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var output struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\noutput: %q", err, stdout)
	}
	if len(output.Projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(output.Projects))
	}
	projects := output.Projects[0]
	if projects["alias"] != "len" || projects["path"] != "/home/neil/code/projects/tta-lab/lenos" {
		t.Fatalf("unexpected JSON project: %v", projects)
	}
	if archived, ok := projects["archived"]; !ok || archived != false {
		t.Fatalf("project archived field = %#v, want required false", projects["archived"])
	}
	if len(projects) != 5 || projects["name"] != "Lenos CLI runtime" ||
		projects["remote"] != "https://github.com/tta-lab/lenos.git" {
		t.Fatalf("project JSON is not the exact five-field DTO: %#v", projects)
	}
}

func TestProjectListIncludesArchivedOnlyWhenRequested(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[active]
path = "/projects/active"
remote = "https://example.com/owner/active.git"

[archived.ttal]
path = "/projects/ttal"
remote = "https://example.com/owner/ttal.git"
`)

	stdout, err := runProject(t, []string{"list", "--include-archived", "--json"})
	if err != nil {
		t.Fatalf("project list: %v", err)
	}
	var output struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	if len(output.Projects) != 2 || output.Projects[1]["alias"] != "ttal" || output.Projects[1]["archived"] != true {
		t.Fatalf("projects = %#v, want archived ttal", output.Projects)
	}

	if _, err := runProject(t, []string{"list", "tta-lab"}); err == nil {
		t.Fatal("project list accepted removed org argument")
	}
}

func TestProjectGetReturnsArchivedEntry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[archived.ttal]
path = "/projects/ttal"
remote = "https://example.com/owner/ttal.git"
`)

	stdout, err := runProject(t, []string{"get", "ttal", "--json"})
	if err != nil {
		t.Fatalf("project get: %v", err)
	}
	var output struct {
		Project map[string]any `json:"project"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode project get: %v", err)
	}
	if output.Project["alias"] != "ttal" || output.Project["archived"] != true {
		t.Fatalf("project get = %#v", output.Project)
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

func TestProjectGetRejectsUnsupportedReferenceWithoutFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `
[fse]
name = "FSE"
path = "/projects/fse"
remote = "https://example.com/owner/fse.git"
`)

	stdout, err := runProject(t, []string{"get", "fse.gw", "--json"})
	if err == nil {
		t.Fatal("expected dotted alias error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(err.Error(), "project find") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want project-reference recovery guidance", err)
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
		want func(t *testing.T, stdout string, err error)
	}{
		{
			name: "get-json-rejects-org-repo",
			args: []string{"get", "tta-lab/demo", "--json"},
			want: func(t *testing.T, stdout string, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("get --json accepted an unregistered org/repo alias")
				}
				if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "invalid project alias") {
					t.Fatalf("get --json error = %v, want alias rejection", err)
				}
			},
		},
		{
			name: "get-human-preserves-reference-fallback",
			args: []string{"get", "tta-lab/demo"},
			want: func(t *testing.T, stdout string, _ error) {
				t.Helper()
				if stdout != repoPath+"\n" {
					t.Fatalf("get output = %q, want %q", stdout, repoPath+"\n")
				}
			},
		},
		{
			name: "resolve",
			args: []string{"resolve", "tta-lab/demo"},
			want: func(t *testing.T, stdout string, _ error) {
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
			want: func(t *testing.T, stdout string, _ error) {
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
			tt.want(t, stdout, err)
		})
	}
}

func TestProjectCommandsPreserveBareReferenceRepoFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repoPath := filepath.Join(tmpHome, "code", "references", "github.com", "tta-lab", "foo.bar")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir bare reference repo: %v", err)
	}

	stdout, err := runProject(t, []string{"get", "foo.bar"})
	if err != nil || stdout != repoPath+"\n" {
		t.Fatalf("get bare reference = %q, err = %v; want %q", stdout, err, repoPath+"\n")
	}

	stdout, err = runProject(t, []string{"resolve", "foo.bar"})
	if err != nil {
		t.Fatalf("resolve bare reference: %v", err)
	}
	var resolved map[string]any
	if err := json.Unmarshal([]byte(stdout), &resolved); err != nil {
		t.Fatalf("decode bare resolve: %v", err)
	}
	if resolved["alias"] != "foo.bar" || resolved["path"] != repoPath || resolved["archived"] != false {
		t.Fatalf("resolve bare reference = %#v", resolved)
	}

	stdout, err = runProject(t, []string{"jump", "foo.bar"})
	if err != nil || stdout != repoPath+"\n" {
		t.Fatalf("jump bare reference = %q, err = %v; want %q", stdout, err, repoPath+"\n")
	}
}

func TestProjectRegisteredBranchesResolveAlternateReferencesCanonically(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"
`)

	stdout, err := runProject(t, []string{"get", "FLICK-BACKEND", "--json"})
	if err != nil {
		t.Fatalf("get alternate reference: %v", err)
	}
	var getOutput struct {
		Project map[string]any `json:"project"`
	}
	if err := json.Unmarshal([]byte(stdout), &getOutput); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if getOutput.Project["alias"] != "fb" || getOutput.Project["path"] != "/projects/flick-backend" {
		t.Fatalf("get output = %#v, want canonical fb", getOutput.Project)
	}

	stdout, err = runProject(t, []string{"resolve", "flick-backend"})
	if err != nil {
		t.Fatalf("resolve alternate reference: %v", err)
	}
	var resolveOutput map[string]any
	if err := json.Unmarshal([]byte(stdout), &resolveOutput); err != nil {
		t.Fatalf("decode resolve: %v", err)
	}
	if resolveOutput["alias"] != "fb" {
		t.Fatalf("resolve output = %#v, want canonical fb", resolveOutput)
	}

	stdout, err = runProject(t, []string{"jump", "flick-backend"})
	if err != nil {
		t.Fatalf("jump alternate reference: %v", err)
	}
	if stdout != "/projects/flick-backend\n" {
		t.Fatalf("jump output = %q", stdout)
	}
}

func TestProjectFindJSONOrderingAndEmptyResult(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"

[backend]
name = "Backend Worker"
path = "/projects/worker"
remote = "https://example.com/owner/worker.git"

[archived.old]
name = "Backend old"
path = "/projects/old-backend"
remote = "https://example.com/owner/old-backend.git"
`)

	stdout, err := runProject(t, []string{"find", "backend", "--json"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	var output struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode find: %v", err)
	}
	if len(output.Projects) != 2 || output.Projects[0]["alias"] != "backend" || output.Projects[1]["alias"] != "fb" {
		t.Fatalf("find projects = %#v, want backend then fb", output.Projects)
	}
	for _, entry := range output.Projects {
		if entry["archived"] == true {
			t.Fatalf("find returned archived project: %#v", entry)
		}
	}

	stdout, err = runProject(t, []string{"find", "no-such-project", "--json"})
	if err != nil {
		t.Fatalf("unmatched find: %v", err)
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode empty find: %v", err)
	}
	if output.Projects == nil || len(output.Projects) != 0 {
		t.Fatalf("empty find projects = %#v, want nonnil empty array", output.Projects)
	}

	stdout, err = runProject(t, []string{"find", "no-such-project"})
	if err != nil || stdout != "No active projects found.\n" {
		t.Fatalf("human empty find = %q, err = %v", stdout, err)
	}
}

func TestProjectNotFoundSuggestsActiveFindRecovery(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[fb]
name = "FlickNote Backend"
path = "/projects/flick-backend"
remote = "https://example.com/owner/flick-backend.git"

[archived.old]
name = "FlickNote Backend old"
path = "/projects/flick-backend-old"
remote = "https://example.com/owner/flick-backend-old.git"
`)

	_, err := runProject(t, []string{"get", "flick-backnd", "--json"})
	if err == nil || !strings.Contains(err.Error(), "fb") ||
		!strings.Contains(err.Error(), "project find") || !strings.Contains(err.Error(), "project list") ||
		strings.Contains(err.Error(), "old") {
		t.Fatalf("not-found error = %v, want active suggestion and recovery hint", err)
	}
}

func TestProjectHumanReferenceFallbackSurvivesActiveSuggestions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	writeProjectsConfig(t, tmpHome, `[near]
name = "TTA Lab Demo"
path = "/projects/near"
remote = "https://example.com/owner/near.git"
`)
	repoPath := filepath.Join(tmpHome, "code", "references", "github.com", "tta-lab", "demo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir reference repo: %v", err)
	}

	stdout, err := runProject(t, []string{"get", "tta-lab/demo"})
	if err != nil {
		t.Fatalf("get reference fallback: %v", err)
	}
	if stdout != repoPath+"\n" {
		t.Fatalf("get output = %q, want %q", stdout, repoPath+"\n")
	}
}
