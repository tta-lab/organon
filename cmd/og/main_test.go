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

func TestRootHelpListsTopLevelCommands(t *testing.T) {
	stdout, err := runOG(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"pr",
		"push",
		"pull",
		"tag",
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
	if strings.Contains(stdout, "  git ") {
		t.Fatalf("root help should not retain the git command group:\n%s", stdout)
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

func TestPRStdinCommandHelpShowsHeredocExamples(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{command: "create", want: `cat <<'EOF' | og pr create`},
		{command: "modify", want: `cat <<'EOF' | og pr modify`},
		{command: "comment", want: `cat <<'EOF' | og pr comment`},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			stdout, err := runOG(t, "pr", tt.command, "--help")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("pr %s help missing heredoc example %q:\n%s", tt.command, tt.want, stdout)
			}
			if strings.Contains(stdout, "\n  EOF") {
				t.Fatalf(
					"pr %s help indents the heredoc terminator, so the example cannot be pasted into a shell:\n%s",
					tt.command,
					stdout,
				)
			}
			if strings.Contains(stdout, "\t") {
				t.Fatalf("pr %s help contains a tab that would alter pasted body text:\n%s", tt.command, stdout)
			}
		})
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

func TestTopLevelGitCommandHelpIncludesFlags(t *testing.T) {
	for _, tt := range []struct {
		command string
		flag    string
	}{
		{command: "push", flag: "--force"},
		{command: "tag", flag: "--bump"},
	} {
		stdout, err := runOG(t, tt.command, "--help")
		if err != nil {
			t.Fatalf("og %s --help: %v", tt.command, err)
		}
		if !strings.Contains(stdout, tt.flag) {
			t.Fatalf("og %s help missing %q:\n%s", tt.command, tt.flag, stdout)
		}
	}
}

func TestTopLevelGitCommandsAreImplemented(t *testing.T) {
	useFailingDaemon(t)
	tests := [][]string{
		{"push", "--force"},
		{"pull"},
		{"tag", "v1.2.3"},
		{"tag", "--bump", "patch"},
	}

	for _, args := range tests {
		_, err := runOG(t, args...)
		if err == nil {
			t.Fatalf("runOG(%v) expected an environment error outside a git repo", args)
		}
		if !strings.Contains(err.Error(), "test daemon unavailable") {
			t.Fatalf("runOG(%v) error = %v, want isolated daemon error", args, err)
		}
	}
}

func TestGitCommandGroupIsRemoved(t *testing.T) {
	_, err := runOG(t, "git", "pull")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("og git pull error = %v, want unknown command", err)
	}
}

func TestPRCommandsAreImplemented(t *testing.T) {
	useFailingDaemon(t)
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
		if !strings.Contains(err.Error(), "test daemon unavailable") {
			t.Fatalf("runOG(%v) error = %v, want isolated daemon error", args, err)
		}
	}

	_, err := runOGWithInput(t, "review note", "pr", "comment")
	if err == nil {
		t.Fatal("runOG([pr comment]) expected an environment error outside a git repo")
	}
	if !strings.Contains(err.Error(), "test daemon unavailable") {
		t.Fatalf("runOG([pr comment]) error = %v, want isolated daemon error", err)
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
	var got map[string]any
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

	stdout, err := runOG(t, "push", "--force")
	if err != nil {
		t.Fatalf("runOG: %v", err)
	}
	if got["work_dir"] == "" {
		t.Fatal("daemon request missing work_dir")
	}
	if got["force"] != true {
		t.Fatal("daemon request missing force=true")
	}
	if _, ok := got["token"]; ok {
		t.Fatalf("CLI leaked token field to daemon: %+v", got)
	}
	if _, ok := got["token_env"]; ok {
		t.Fatalf("CLI leaked token fields to daemon: %+v", got)
	}
	if !strings.Contains(stdout, "pushed from daemon") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPRCreateRoutesThroughDaemonWithBodyAndTitle(t *testing.T) {
	var got map[string]any
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
	if got["title"] != "feat: daemon first" || got["body"] != "body from stdin" {
		t.Fatalf("request = %+v, want title/body", got)
	}
	if _, ok := got["token"]; ok {
		t.Fatalf("CLI leaked token field to daemon: %+v", got)
	}
	if _, ok := got["token_env"]; ok {
		t.Fatalf("CLI leaked token fields to daemon: %+v", got)
	}
	if !strings.Contains(stdout, "PR #12 created") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPRViewJSONPrintsCISummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pr/view" {
			t.Fatalf("path = %s, want /pr/view", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(og.Response{
			OK: true,
			PR: &og.PullRequest{
				Index: 9,
				Title: "title",
				State: "open",
				Head:  "feature/x",
				Base:  "main",
				CI: &og.CIStatusResponse{
					OK:    true,
					State: "success",
					Statuses: []og.CIStatus{{
						Context:     "check",
						State:       "success",
						Description: "passed",
						TargetURL:   "https://ci/job/1",
					}},
				},
			},
		})
	}))
	defer server.Close()
	t.Setenv("OG_DAEMON_URL", server.URL)

	stdout, err := runOG(t, "pr", "view", "--json")
	if err != nil {
		t.Fatalf("runOG: %v", err)
	}
	var got og.PullRequest
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if got.CI == nil || got.CI.State != "success" {
		t.Fatalf("CI = %+v, want success summary", got.CI)
	}
	if len(got.CI.Statuses) != 1 || got.CI.Statuses[0].Context != "check" {
		t.Fatalf("statuses = %+v, want check", got.CI.Statuses)
	}
}

func TestPRLogRoutesToLogEndpoint(t *testing.T) {
	var got og.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pr/log" {
			t.Fatalf("path = %s, want /pr/log", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(og.Response{
			OK:    true,
			Lines: []string{"CI Status for abc12345: failed", "Failure Details:"},
		})
	}))
	defer server.Close()
	t.Setenv("OG_DAEMON_URL", server.URL)

	stdout, err := runOG(t, "pr", "log", "--tail", "12")
	if err != nil {
		t.Fatalf("runOG: %v", err)
	}
	if got.Tail != 12 {
		t.Fatalf("tail = %d, want 12", got.Tail)
	}
	if !strings.Contains(stdout, "CI Status") || !strings.Contains(stdout, "Failure Details:") {
		t.Fatalf("stdout = %q, want status and details", stdout)
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

func useFailingDaemon(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(og.Response{Error: "test daemon unavailable"})
	}))
	t.Cleanup(server.Close)
	t.Setenv("OG_DAEMON_URL", server.URL)
}
