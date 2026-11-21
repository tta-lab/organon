package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

func testWorktreeCheckout(t *testing.T) (*project.Store, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	checkout := filepath.Join(root, "registered")
	worktree := filepath.Join(root, "feature checkout")
	runRepoMCPGit(t, root, "init", "-qb", "main", checkout)
	runRepoMCPGit(t, checkout, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit", "--allow-empty", "-qm", "initial")
	runRepoMCPGit(t, checkout, "worktree", "add", "-qb", "feature", worktree)
	registry := filepath.Join(root, "projects.toml")
	config := fmt.Sprintf("[ko]\npath = %q\nremote = \"https://example.com/team/repo.git\"\n", checkout)
	if err := os.WriteFile(registry, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return project.NewStore(registry), checkout, worktree
}

func TestCLIWorktreeSelection(t *testing.T) {
	projects, checkout, worktree := testWorktreeCheckout(t)
	var calledWith string
	gitOperation := func(req og.Request) (og.Response, error) {
		calledWith = req.WorkDir
		return og.Response{Message: "updated branch"}, nil
	}
	executor := &directExecutor{
		gitPush: gitOperation,
		gitPull: gitOperation,
		prFind: func(req og.Request) (og.Response, error) {
			calledWith = req.WorkDir
			return og.Response{PR: &og.PullRequest{Index: 7, State: "open"}}, nil
		},
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "explicit push", args: []string{"--worktree", worktree, "push"}, want: worktree},
		{name: "explicit pull", args: []string{"pull", "--worktree", worktree}, want: worktree},
		{name: "explicit PR", args: []string{"pr", "find", "--worktree", worktree}, want: worktree},
		{name: "registered checkout", args: []string{"--project", "ko", "push"}, want: checkout},
		{name: "both selectors", args: []string{"push", "--project", "ko", "--worktree", worktree}},
		{name: "empty", args: []string{"pull", "--worktree", ""}},
		{name: "relative", args: []string{"push", "--worktree", "feature"}},
		{name: "registered path", args: []string{"push", "--worktree", checkout}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calledWith = ""
			stdout, _, err := runDirectCLI(t, executor, projects, "", append(tc.args, "--json")...)
			if tc.want == "" {
				if err == nil || calledWith != "" {
					t.Fatalf("invalid selector reached %q, error = %v", calledWith, err)
				}
				return
			}
			if err != nil || calledWith != tc.want {
				t.Fatalf("called with %q, want %q, error = %v", calledWith, tc.want, err)
			}
			var result struct{ Project string }
			if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.Project != "ko" {
				t.Fatalf("CLI result = %s, error = %v", stdout, err)
			}
		})
	}
}

func TestMCPWorktreeSelection(t *testing.T) {
	projects, _, worktree := testWorktreeCheckout(t)
	record := func(req og.Request) error {
		if req.WorkDir != worktree || req.Context == nil {
			return fmt.Errorf("unexpected worktree request: %+v", req)
		}
		return nil
	}
	executor := &directExecutor{
		gitPush: func(req og.Request) (og.Response, error) {
			return og.Response{Message: "pushed"}, record(req)
		},
		gitPull: func(req og.Request) (og.Response, error) {
			return og.Response{Message: "pulled"}, record(req)
		},
		prCreate: func(req og.Request) (og.Response, error) {
			return og.Response{PR: &og.PullRequest{Index: 7, State: "open"}}, record(req)
		},
		prGet: func(req og.Request) (og.Response, error) {
			return og.Response{PR: &og.PullRequest{Index: req.Index, State: "open"}}, record(req)
		},
		prMerge: func(req og.Request) (og.Response, error) {
			return og.Response{Merge: &og.PRMergeResult{
				ApprovalPolicy: "none", Status: og.PRMergeStatusExecuted,
				Snapshot:   og.PRMergeSnapshot{PRNumber: req.Index, PRURL: "https://example.com/team/repo/pulls/7"},
				NextAction: og.PRMergeNextNone, Completion: "cleanup complete",
			}}, record(req)
		},
	}
	session := connectDirectMCP(t, executor, projects)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"push", map[string]any{"worktree": worktree}},
		{"pull", map[string]any{"worktree": worktree}},
		{"pr_create", map[string]any{"worktree": worktree, "title": "feature"}},
		{"pr_get", map[string]any{"worktree": worktree, "pr_id": 7}},
		{"pr_merge", map[string]any{"worktree": worktree, "pr_id": 7}},
	} {
		out, bad := sourceCall(t, session, tc.name, tc.args)
		if bad || out["project"] != "ko" {
			t.Fatalf("%s result = %v, tool error = %v", tc.name, out, bad)
		}
	}
	for _, args := range []map[string]any{
		{}, {"project": "ko", "worktree": worktree}, {"worktree": ""}, {"worktree": "feature"},
	} {
		if out, bad := sourceCall(t, session, "push", args); !bad {
			t.Fatalf("push accepted selector %v: %v", args, out)
		}
	}
}
