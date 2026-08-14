package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/og"
)

func ogWriteProjects(t *testing.T, home string) {
	t.Helper()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[ko]\npath = \"/work/ko\"\n" +
		"remote = \"https://github.com/tta-lab/ko.git\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ogDaemon returns an httptest server that records requests and replies with a
// fixed response for every path.
func ogDaemon(t *testing.T, respond func(path string, req og.Request) og.Response) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req og.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := respond(r.URL.Path, req)
		resp.OK = true
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	t.Setenv("OG_DAEMON_URL", server.URL)
}

func runOGJSON(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd(&outBuf, &errBuf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return outBuf.String(), err
}

func TestAuthStatusProjectJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	var gotPath string
	ogDaemon(t, func(path string, req og.Request) og.Response {
		gotPath = path
		if req.WorkDir != "/work/ko" {
			t.Errorf("WorkDir = %q, want /work/ko", req.WorkDir)
		}
		return og.Response{Auth: &og.AuthStatus{
			Project: "ko", Provider: "github", Host: "github.com",
			Owner: "tta-lab", Repo: "ko", AuthMode: "token", Ready: true,
		}}
	})
	stdout, err := runOGJSON(t, "auth", "status", "--project", "ko", "--json")
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	if gotPath != "/auth/status" {
		t.Fatalf("path = %s", gotPath)
	}
	var out ogAuthJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Project != "ko" || out.Auth.Provider != "github" {
		t.Fatalf("output = %+v", out)
	}
}

func TestPRFindProjectJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	var gotState string
	ogDaemon(t, func(path string, req og.Request) og.Response {
		gotState = req.State
		return og.Response{PR: &og.PullRequest{Index: 7, Title: "t", State: "open"}}
	})
	stdout, err := runOGJSON(t, "pr", "find", "--project", "ko", "--state", "closed", "--json")
	if err != nil {
		t.Fatalf("pr find: %v", err)
	}
	if gotState != "closed" {
		t.Fatalf("state = %q", gotState)
	}
	var out ogPRJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Project != "ko" || out.PR.Index != 7 {
		t.Fatalf("output = %+v", out)
	}
}

func TestPRChecksProjectJSONLines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	ogDaemon(t, func(path string, req og.Request) og.Response {
		return og.Response{
			PR:    &og.PullRequest{Index: 3, Title: "ci", State: "open"},
			Lines: []string{"check: pass"},
		}
	})
	stdout, err := runOGJSON(t, "pr", "checks", "--project", "ko", "--json")
	if err != nil {
		t.Fatalf("pr checks: %v", err)
	}
	var out ogPRLinesJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Project != "ko" || out.PR.Index != 3 || len(out.Lines) != 1 {
		t.Fatalf("output = %+v", out)
	}
}

func TestPushProjectJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	var gotForce bool
	ogDaemon(t, func(path string, req og.Request) og.Response {
		gotForce = req.Force
		return og.Response{Message: "pushed"}
	})
	stdout, err := runOGJSON(t, "push", "--project", "ko", "--force", "--json")
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !gotForce {
		t.Fatal("force flag not forwarded")
	}
	var out ogMessageJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Project != "ko" || out.Message != "pushed" {
		t.Fatalf("output = %+v", out)
	}
}

func TestCloneURLJSONShape(t *testing.T) {
	ogDaemon(t, func(path string, req og.Request) og.Response {
		return og.Response{Clone: &og.CloneResult{
			Path: "/work/refs/owner/repo", Host: "github.com", Owner: "owner", Repo: "repo",
			Provider: "github", Remote: "https://github.com/owner/repo.git",
		}}
	})
	stdout, err := runOGJSON(t, "clone", "https://github.com/owner/repo", "--json")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	var out ogCloneJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if out.Clone.Path != "/work/refs/owner/repo" {
		t.Fatalf("output = %+v", out)
	}
}

func TestUnknownProjectAliasFailsBeforeDaemon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	called := false
	ogDaemon(t, func(path string, req og.Request) og.Response {
		called = true
		return og.Response{Message: "unexpected"}
	})
	_, err := runOGJSON(t, "auth", "status", "--project", "missing", "--json")
	if err == nil || !strings.Contains(err.Error(), "resolve project") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("daemon called for unknown alias")
	}
}

func TestPRCreateProjectJSONWithStdinBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	var gotBody string
	ogDaemon(t, func(path string, req og.Request) og.Response {
		if req.Body != nil {
			gotBody = *req.Body
		}
		return og.Response{PR: &og.PullRequest{Index: 9, Title: "feat: x", State: "open"}}
	})
	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd(&outBuf, &errBuf)
	cmd.SetIn(strings.NewReader("## Summary\n\nBody text.\n"))
	cmd.SetArgs([]string{"pr", "create", "feat: x", "--project", "ko", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pr create: %v", err)
	}
	if gotBody != "## Summary\n\nBody text." {
		t.Fatalf("body = %q", gotBody)
	}
	var out ogPRJSON
	if err := json.Unmarshal(outBuf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, outBuf.String())
	}
	if out.Project != "ko" || out.PR.Index != 9 {
		t.Fatalf("output = %+v", out)
	}
}
