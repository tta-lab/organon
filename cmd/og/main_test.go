package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/og"
)

func runOG(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	return runOGWithInput(t, "", args...)
}

func runOGWithInput(t *testing.T, input string, args ...string) (stdout string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd(&outBuf, &errBuf)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), err
}

func TestRootHelpListsV1CommandGroups(t *testing.T) {
	stdout, err := runOG(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"pr",
		"git",
		"auth",
		"daemon",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("root help missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "policy") {
		t.Fatalf("root help should not list policy in V1:\n%s", stdout)
	}
}

func TestPRHelpListsV1CommandsWithoutMerge(t *testing.T) {
	stdout, err := runOG(t, "pr", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"create",
		"view",
		"list",
		"find",
		"get",
		"modify",
		"comment",
		"checks",
		"status",
		"failures",
		"log",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("pr help missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "merge") {
		t.Fatalf("pr help should not list merge:\n%s", stdout)
	}
}

func TestPRMergeIsNotAvailableInV1(t *testing.T) {
	_, err := runOG(t, "pr", "merge")
	if err == nil {
		t.Fatal("expected pr merge to fail")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want unknown command", err)
	}
}

func TestGitHelpListsTtalReplacementCommands(t *testing.T) {
	stdout, err := runOG(t, "git", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"push",
		"pull",
		"tag",
		"--force",
		"--bump",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("git help missing %q:\n%s", want, stdout)
		}
	}
}

func TestGitCommandsAreImplemented(t *testing.T) {
	tests := [][]string{
		{"git", "push", "--force"},
		{"git", "pull"},
		{"git", "tag", "v1.2.3"},
		{"git", "tag", "--bump", "patch"},
	}

	for _, args := range tests {
		_, err := runOG(t, args...)
		if err == nil {
			t.Fatalf("runOG(%v) expected an environment error outside a git repo", args)
		}
		if !strings.Contains(err.Error(), "daemon call") {
			t.Fatalf("runOG(%v) error = %v, want daemon routing error", args, err)
		}
	}
}

func TestPRCommandsAreImplemented(t *testing.T) {
	tests := [][]string{
		{"pr", "create", "feat: add forge CLI"},
		{"pr", "view", "--json"},
		{"pr", "list", "--json"},
		{"pr", "find", "--state", "all"},
		{"pr", "get", "38", "--json"},
		{"pr", "modify", "--title", "new title", "--pr-id", "38"},
		{"pr", "log", "--tail", "200"},
		{"pr", "checks"},
		{"pr", "status"},
		{"pr", "failures", "--tail", "200"},
	}

	for _, args := range tests {
		_, err := runOG(t, args...)
		if err == nil {
			t.Fatalf("runOG(%v) expected an environment error outside a git repo", args)
		}
		if !strings.Contains(err.Error(), "daemon call") {
			t.Fatalf("runOG(%v) error = %v, want daemon routing error", args, err)
		}
	}

	_, err := runOGWithInput(t, "review note", "pr", "comment")
	if err == nil {
		t.Fatal("runOG([pr comment]) expected an environment error outside a git repo")
	}
	if !strings.Contains(err.Error(), "daemon call") {
		t.Fatalf("runOG([pr comment]) error = %v, want daemon routing error", err)
	}
}

func TestDaemonHelpListsLifecycleCommands(t *testing.T) {
	stdout, err := runOG(t, "daemon", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"run",
		"install",
		"uninstall",
		"start",
		"stop",
		"restart",
		"status",
		"health",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("daemon help missing %q:\n%s", want, stdout)
		}
	}
}

func TestDaemonLifecycleCommandsAreImplemented(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, subcmd := range []string{"install", "uninstall", "start", "stop", "restart", "status", "health"} {
		_, _ = runOG(t, "daemon", subcmd)
	}
}

func TestGitPushRoutesThroughDaemonWithoutReadingRepoOrToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "worker-token-must-not-be-read")
	var got og.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/git/push" {
			t.Fatalf("path = %s, want /git/push", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(og.Response{OK: true, Message: "pushed from daemon"})
	}))
	defer server.Close()
	t.Setenv("OG_DAEMON_URL", server.URL)

	stdout, err := runOG(t, "git", "push", "--force")
	if err != nil {
		t.Fatalf("runOG: %v", err)
	}
	if got.WorkDir == "" {
		t.Fatal("daemon request missing work_dir")
	}
	if !got.Force {
		t.Fatal("daemon request missing force=true")
	}
	if got.Token != "" || got.TokenEnv != "" {
		t.Fatalf("CLI leaked token fields to daemon: %+v", got)
	}
	if !strings.Contains(stdout, "pushed from daemon") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPRCreateRoutesThroughDaemonWithBodyAndTitle(t *testing.T) {
	var got og.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pr/create" {
			t.Fatalf("path = %s, want /pr/create", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(og.Response{OK: true, Message: "PR #12 created"})
	}))
	defer server.Close()
	t.Setenv("OG_DAEMON_URL", server.URL)

	stdout, err := runOGWithInput(t, "body from stdin\n", "pr", "create", "feat: daemon first")
	if err != nil {
		t.Fatalf("runOG: %v", err)
	}
	if got.Title != "feat: daemon first" || got.Body != "body from stdin" {
		t.Fatalf("request = %+v, want title/body", got)
	}
	if got.Token != "" || got.TokenEnv != "" {
		t.Fatalf("CLI leaked token fields to daemon: %+v", got)
	}
	if !strings.Contains(stdout, "PR #12 created") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestDaemonRejectsUnregisteredProject(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	repo := t.TempDir()
	initGitRepo(t, repo)

	_, err := og.Service{}.GitPush(og.Request{WorkDir: repo})
	if err == nil {
		t.Fatal("expected unregistered project to be rejected")
	}
	if !strings.Contains(err.Error(), "registered project") {
		t.Fatalf("error = %v, want registered project rejection", err)
	}
}

func TestDaemonCallUsesUnixSocketByDefault(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "og.sock")
	t.Setenv("OG_DAEMON_SOCKET", socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/git/pull" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(og.Response{OK: true, Message: "unix ok"})
	})}
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()

	resp, err := daemonCall("/git/pull", og.Request{WorkDir: "/tmp/repo"})
	if err != nil {
		t.Fatalf("daemonCall: %v", err)
	}
	if resp.Message != "unix ok" {
		t.Fatalf("response = %+v", resp)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", "https://github.com/tta-lab/example.git"},
	} {
		cmd := execCommand(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

var execCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
