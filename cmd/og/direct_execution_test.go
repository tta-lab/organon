package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

type directExecutor struct {
	gitPush    func(og.Request) (og.Response, error)
	gitPull    func(og.Request) (og.Response, error)
	gitTag     func(og.Request) (og.Response, error)
	gitClone   func(og.Request) (og.Response, error)
	prCreate   func(og.Request) (og.Response, error)
	prView     func(og.Request) (og.Response, error)
	prFind     func(og.Request) (og.Response, error)
	prGet      func(og.Request) (og.Response, error)
	prModify   func(og.Request) (og.Response, error)
	prComment  func(og.Request) (og.Response, error)
	prChecks   func(og.Request) (og.Response, error)
	prLog      func(og.Request) (og.Response, error)
	prFailures func(og.Request) (og.Response, error)
	authStatus func(og.Request) (og.Response, error)
}

func (e *directExecutor) call(
	name string,
	fn func(og.Request) (og.Response, error),
	req og.Request,
) (og.Response, error) {
	if fn == nil {
		return og.Response{}, fmt.Errorf("unexpected direct operation %s", name)
	}
	return fn(req)
}
func (e *directExecutor) GitPush(req og.Request) (og.Response, error) {
	return e.call("push", e.gitPush, req)
}
func (e *directExecutor) GitPull(req og.Request) (og.Response, error) {
	return e.call("pull", e.gitPull, req)
}
func (e *directExecutor) GitTag(req og.Request) (og.Response, error) {
	return e.call("tag", e.gitTag, req)
}
func (e *directExecutor) GitClone(req og.Request) (og.Response, error) {
	return e.call("clone", e.gitClone, req)
}
func (e *directExecutor) PRCreate(req og.Request) (og.Response, error) {
	return e.call("pr create", e.prCreate, req)
}
func (e *directExecutor) PRView(req og.Request) (og.Response, error) {
	return e.call("pr view", e.prView, req)
}
func (e *directExecutor) PRFind(req og.Request) (og.Response, error) {
	return e.call("pr find", e.prFind, req)
}
func (e *directExecutor) PRGet(req og.Request) (og.Response, error) {
	return e.call("pr get", e.prGet, req)
}
func (e *directExecutor) PRModify(req og.Request) (og.Response, error) {
	return e.call("pr modify", e.prModify, req)
}
func (e *directExecutor) PRComment(req og.Request) (og.Response, error) {
	return e.call("pr comment", e.prComment, req)
}
func (e *directExecutor) PRChecks(req og.Request) (og.Response, error) {
	return e.call("pr checks", e.prChecks, req)
}
func (e *directExecutor) PRLog(req og.Request) (og.Response, error) {
	return e.call("pr log", e.prLog, req)
}
func (e *directExecutor) PRFailures(req og.Request) (og.Response, error) {
	return e.call("pr failures", e.prFailures, req)
}
func (e *directExecutor) AuthStatus(req og.Request) (og.Response, error) {
	return e.call("auth status", e.authStatus, req)
}

func testProjectStore(t *testing.T) *project.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects.toml")
	content := "[ko]\npath = \"/work/ko\"\nremote = \"https://github.com/tta-lab/ko.git\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return project.NewStore(path)
}

func runDirectCLI(
	t *testing.T,
	executor og.Executor,
	projects *project.Store,
	input string,
	args ...string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newRootCmdWithExecutor(&stdout, &stderr, executor, projects)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestRootHelpOmitsRemovedProcessModel(t *testing.T) {
	stdout, _, err := runDirectCLI(t, nil, nil, "", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"pr", "push", "pull", "tag", "auth", "mcp"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(strings.ToLower(stdout), "legacy service") {
		t.Fatalf("help retains removed process model:\n%s", stdout)
	}
}

func TestCLIInvokesConfiguredExecutorDirectly(t *testing.T) {
	projects := testProjectStore(t)
	var got og.Request
	executor := &directExecutor{gitPush: func(req og.Request) (og.Response, error) {
		got = req
		return og.Response{Message: "direct push"}, nil
	}}
	stdout, stderr, err := runDirectCLI(t, executor, projects, "", "push", "--project", "ko", "--force", "--json")
	if err != nil {
		t.Fatalf("push: %v\nstderr: %s", err, stderr)
	}
	if got.WorkDir != "/work/ko" || !got.Force || got.Context == nil {
		t.Fatalf("request = %+v, want direct resolved request with context", got)
	}
	var result ogMessageJSON
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout)
	}
	if result.Project != "ko" || result.Message != "direct push" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCLIForwardsStdinAndCloneSelectorsToDomain(t *testing.T) {
	projects := testProjectStore(t)
	var body string
	var cloneRequest og.Request
	executor := &directExecutor{
		prCreate: func(req og.Request) (og.Response, error) {
			if req.Title == nil || *req.Title != "feat: direct" || req.Body == nil {
				return og.Response{}, errors.New("missing title or body")
			}
			body = *req.Body
			return og.Response{PR: &og.PullRequest{Index: 9, Title: *req.Title, Body: body, State: "open"}}, nil
		},
		gitClone: func(req og.Request) (og.Response, error) {
			cloneRequest = req
			return og.Response{Clone: &og.CloneResult{
				Path: "/work/references/github.com/owner/repo", Host: "github.com",
				Owner: "owner", Repo: "repo", Provider: "github",
				Remote: "https://github.com/owner/repo.git",
			}}, nil
		},
	}
	stdout, _, err := runDirectCLI(
		t, executor, projects, "raw\nbody\n", "pr", "create", "feat: direct", "--project", "ko", "--json",
	)
	if err != nil {
		t.Fatalf("pr create: %v", err)
	}
	if body != "raw\nbody\n" {
		t.Fatalf("body = %q", body)
	}
	var prResult ogPRJSON
	if err := json.Unmarshal([]byte(stdout), &prResult); err != nil || prResult.PR.Index != 9 {
		t.Fatalf("PR output = %q, err = %v", stdout, err)
	}
	stdout, _, err = runDirectCLI(
		t, executor, projects, "", "clone", "--reference", "https://github.com/owner/repo", "--json",
	)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if cloneRequest.URL != "https://github.com/owner/repo" || !cloneRequest.Reference || cloneRequest.Context == nil {
		t.Fatalf("clone request = %+v", cloneRequest)
	}
	var cloneResult ogCloneJSON
	if err := json.Unmarshal([]byte(stdout), &cloneResult); err != nil || cloneResult.Clone.Repo != "repo" {
		t.Fatalf("clone output = %q, err = %v", stdout, err)
	}
}

func TestCLIRejectsExplicitEmptyProjectBeforeDomainCall(t *testing.T) {
	called := false
	executor := &directExecutor{authStatus: func(og.Request) (og.Response, error) {
		called = true
		return og.Response{}, nil
	}}
	_, _, err := runDirectCLI(t, executor, testProjectStore(t), "", "auth", "status", "--project", "", "--json")
	if err == nil || !strings.Contains(err.Error(), "project reference must not be empty") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("domain executor called after invalid project selector")
	}
}

func TestCLIResultValidationUsesDomainTerminology(t *testing.T) {
	executor := &directExecutor{authStatus: func(og.Request) (og.Response, error) {
		return og.Response{}, nil
	}}
	_, _, err := runDirectCLI(t, executor, testProjectStore(t), "", "auth", "status", "--project", "ko")
	if err == nil || !strings.Contains(err.Error(), "og returned no authentication status") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "legacy service") {
		t.Fatalf("error retains removed terminology: %v", err)
	}
}

func TestCLIPropagatesCancellationContext(t *testing.T) {
	var got context.Context
	executor := &directExecutor{gitPull: func(req og.Request) (og.Response, error) {
		got = req.Context
		return og.Response{Message: "pulled"}, nil
	}}
	_, _, err := runDirectCLI(t, executor, testProjectStore(t), "", "pull", "--project", "ko")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if got == nil {
		t.Fatal("request did not carry command context")
	}
}
func runConfiguredGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	runGitCommand(t, append([]string{"-C", repo}, args...)...)
}

func runGitCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeOGConfig(t *testing.T, home, content string) {
	t.Helper()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "og.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeConfiguredProject(t *testing.T, home, remote string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-b", "main", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	runConfiguredGit(t, repo, "config", "user.email", "test@example.com")
	runConfiguredGit(t, repo, "config", "user.name", "Test User")
	runConfiguredGit(t, repo, "commit", "--allow-empty", "-m", "initial")
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projects := fmt.Sprintf("[ko]\npath = %q\nremote = %q\n", repo, remote)
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(projects), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestCLIUsesLoadedServiceDirectly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfiguredProject(t, home, "https://example.com/owner/repo.git")
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"auth", "status", "--project", "ko", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status: %v\nstderr: %s", err, stderr.String())
	}
	var result ogAuthJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.Project != "ko" || result.Auth.Provider != "generic" || !result.Auth.Ready {
		t.Fatalf("result = %+v", result)
	}
}
func TestCLIUsesLoadedServiceForGitAndPRPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfiguredProject(t, home, "https://example.com/owner/repo.git")
	for _, tc := range []struct {
		name  string
		input string
		args  []string
	}{
		{name: "push", args: []string{"push", "--project", "ko"}},
		{name: "pr create", input: "body", args: []string{"pr", "create", "title", "--project", "ko"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := newRootCmd(&stdout, &stderr)
			cmd.SetIn(strings.NewReader(tc.input))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "generic HTTPS repository is read-only") {
				t.Fatalf("error = %v, want direct policy error; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}

func TestCLIUsesLoadedServiceForConfiguredClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	webRoot := t.TempDir()
	bare := filepath.Join(webRoot, "owner", "repo.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, "init", "--bare", bare)
	work := filepath.Join(t.TempDir(), "work")
	runGitCommand(t, "init", "-b", "main", work)
	runConfiguredGit(t, work, "config", "user.email", "test@example.com")
	runConfiguredGit(t, work, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runConfiguredGit(t, work, "add", "README.md")
	runConfiguredGit(t, work, "commit", "-m", "fixture")
	runConfiguredGit(t, work, "remote", "add", "origin", bare)
	runConfiguredGit(t, work, "push", "origin", "main")
	runGitCommand(t, "--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCommand(t, "--git-dir", bare, "update-server-info")
	server := httptest.NewServer(http.FileServer(http.Dir(webRoot)))
	defer server.Close()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("[forgejo]\nallowed_base_urls = [%q]\n", server.URL)
	if err := os.WriteFile(filepath.Join(configDir, "og.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	remote := server.URL + "/owner/repo.git"
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"clone", "--alias", "example", remote, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clone: %v\nstderr: %s", err, stderr.String())
	}
	var result ogCloneJSON
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	wantPath := filepath.Join(home, "code", "projects", "owner", "repo")
	if result.Clone.Path != wantPath || result.Clone.Provider != "forgejo" || !result.Clone.Registered {
		t.Fatalf("clone = %+v, want configured direct clone at %s", result.Clone, wantPath)
	}
}

func TestCLITagValidatesDirectResult(t *testing.T) {
	executor := &directExecutor{gitTag: func(og.Request) (og.Response, error) {
		return og.Response{}, nil
	}}
	_, _, err := runDirectCLI(t, executor, testProjectStore(t), "", "tag", "v1.2.3", "--project", "ko")
	if err == nil || !strings.Contains(err.Error(), "og returned no operation result") {
		t.Fatalf("error = %v", err)
	}
}

func TestCLIConfigurationFailureIsSecretFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfiguredProject(t, home, "https://github.com/owner/repo.git")
	writeOGConfig(t, home, `[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/private.pem"
allowed_owners = ["owner"]
`)
	configDir := filepath.Join(home, ".config", "ttal", "og")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const secret = "private-key-material"
	if err := os.WriteFile(filepath.Join(configDir, "private.pem"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd(bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"auth", "status", "--project", "ko"})
	err := cmd.Execute()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %v, want secret-free configuration failure", err)
	}
}

func TestCLIMissingForgejoTokenIsSecretFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "")
	const remote = "http://forgejo.example/owner/repo.git"
	repo := writeConfiguredProject(t, home, remote)
	runConfiguredGit(t, repo, "remote", "add", "origin", remote)
	writeOGConfig(t, home, `[forgejo]
allowed_base_urls = ["http://forgejo.example"]
`)
	cmd := newRootCmd(bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"push", "--project", "ko"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing token: set FORGEJO_TOKEN") {
		t.Fatalf("error = %v, want missing-token failure", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error contains secret material: %v", err)
	}
}

func TestCLIResolvesAlternateProjectReferenceBeforeExecutor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	content := "[fb]\npath = \"/work/flick-backend\"\nremote = \"https://example.com/owner/flick-backend.git\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := project.NewStore(path)
	var got og.Request
	executor := &directExecutor{gitPush: func(req og.Request) (og.Response, error) {
		got = req
		return og.Response{Message: "pushed"}, nil
	}}

	stdout, _, err := runDirectCLI(t, executor, projects, "", "push", "--project", "FLICK-BACKEND", "--json")
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if got.WorkDir != "/work/flick-backend" {
		t.Fatalf("request = %+v, want resolved checkout", got)
	}
	var output ogMessageJSON
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.Project != "fb" {
		t.Fatalf("output project = %q, want canonical fb", output.Project)
	}
}

func TestCLIRejectsUnknownOGProjectBeforeExecutorWithRecovery(t *testing.T) {
	called := false
	executor := &directExecutor{gitPush: func(og.Request) (og.Response, error) {
		called = true
		return og.Response{Message: "unexpected"}, nil
	}}
	_, _, err := runDirectCLI(t, executor, testProjectStore(t), "", "push", "--project", "missing", "--json")
	if err == nil || !strings.Contains(err.Error(), "project find") || !strings.Contains(err.Error(), "project list") {
		t.Fatalf("error = %v, want shared recovery guidance", err)
	}
	if called {
		t.Fatal("executor called for unknown project")
	}
}
