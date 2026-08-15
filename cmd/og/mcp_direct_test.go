package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

func connectDirectMCP(t *testing.T, executor og.Executor, projectsPath *project.Store) *mcp.ClientSession {
	t.Helper()
	server := newOGMCPServer(projectsPath, executor)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "og-direct-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestOGMCPUsesDirectExecutorAndPreservesToolContracts(t *testing.T) {
	projects := testProjectStore(t)
	var requests []og.Request
	record := func(req og.Request) {
		requests = append(requests, req)
		if req.WorkDir != "/work/ko" || req.Context == nil {
			t.Fatalf("request = %+v, want registered workdir and context", req)
		}
	}
	executor := &directExecutor{
		authStatus: func(req og.Request) (og.Response, error) {
			record(req)
			return og.Response{Auth: &og.AuthStatus{Project: "ko", Provider: "github", Ready: true}}, nil
		},
		gitPush: func(req og.Request) (og.Response, error) {
			record(req)
			return og.Response{Message: "pushed directly"}, nil
		},
		prGet: func(req og.Request) (og.Response, error) {
			record(req)
			return og.Response{PR: &og.PullRequest{Index: req.Index, Title: "direct", State: "open"}}, nil
		},
		gitClone: func(req og.Request) (og.Response, error) {
			if req.Context == nil || req.URL != "https://github.com/owner/repo" {
				t.Fatalf("clone request = %+v", req)
			}
			return og.Response{Clone: &og.CloneResult{
				Path: "/work/repo", Host: "github.com", Owner: "owner", Repo: "repo",
				Provider: "github", Remote: "https://github.com/owner/repo.git",
			}}, nil
		},
	}
	session := connectDirectMCP(t, executor, projects)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		gotNames = append(gotNames, tool.Name)
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q lacks generated schemas", tool.Name)
		}
	}
	sort.Strings(gotNames)
	wantNames := []string{
		"auth_status", "clone", "pr_checks", "pr_comment", "pr_create", "pr_failures",
		"pr_find", "pr_get", "pr_log", "pr_modify", "pull", "push",
	}
	if fmt.Sprint(gotNames) != fmt.Sprint(wantNames) {
		t.Fatalf("tools = %v, want %v", gotNames, wantNames)
	}

	assertDirectMCPToolCalls(t, session, &requests)
}

func assertDirectMCPToolCalls(t *testing.T, session *mcp.ClientSession, requests *[]og.Request) {
	t.Helper()
	for _, call := range []struct {
		name string
		args map[string]any
		key  string
	}{
		{name: "auth_status", args: map[string]any{"project": "ko"}, key: "auth"},
		{name: "push", args: map[string]any{"project": "ko", "force": true}, key: "message"},
		{name: "pr_get", args: map[string]any{"project": "ko", "pr_id": 17}, key: "pr"},
		{name: "clone", args: map[string]any{"url": "https://github.com/owner/repo"}, key: "clone"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil {
			t.Fatalf("%s: %v", call.name, err)
		}
		if result.IsError {
			t.Fatalf("%s result = %#v", call.name, result)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil || !strings.Contains(string(data), call.key) {
			t.Fatalf("%s structured result = %s, err = %v", call.name, data, err)
		}
	}
	if len(*requests) != 3 {
		t.Fatalf("registered operation requests = %d, want 3", len(*requests))
	}
}

func TestOGMCPDirectExecutorReceivesCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	executor := &directExecutor{prGet: func(req og.Request) (og.Response, error) {
		close(started)
		<-req.Context.Done()
		close(canceled)
		return og.Response{}, req.Context.Err()
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{
			Name: "pr_get", Arguments: map[string]any{"project": "ko", "pr_id": 17},
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("direct operation did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("direct operation context was not canceled")
	}
	<-done
}

func TestOGMCPValidatesInputsBeforeDirectExecution(t *testing.T) {
	calls := 0
	executor := &directExecutor{prGet: func(og.Request) (og.Response, error) {
		calls++
		return og.Response{PR: &og.PullRequest{Index: 1}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	for _, args := range []map[string]any{
		{"project": "missing", "pr_id": 1},
		{"project": "ko", "pr_id": 0},
		{"project": "/tmp/repo", "pr_id": 1},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pr_get", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("args %v succeeded", args)
		}
	}
	if calls != 0 {
		t.Fatalf("direct calls = %d, want 0", calls)
	}
}

func TestOGMCPResultValidationUsesDirectTerminology(t *testing.T) {
	executor := &directExecutor{authStatus: func(og.Request) (og.Response, error) {
		return og.Response{}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(result.Content)
	if !result.IsError || !strings.Contains(string(content), "og returned no authentication status") {
		t.Fatalf("content = %s", content)
	}
	if strings.Contains(string(content), "legacy service") {
		t.Fatalf("content retains removed terminology: %s", content)
	}
}

func TestOGMCPLoadsRegistryChangesOnNextCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	write := func(alias, repo string) {
		content := fmt.Sprintf(
			"[%s]\npath = \"/work/%s\"\nremote = \"https://github.com/tta-lab/%s.git\"\n",
			alias, alias, repo,
		)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ko", "ko")
	var got string
	executor := &directExecutor{authStatus: func(req og.Request) (og.Response, error) {
		got = req.WorkDir
		return og.Response{Auth: &og.AuthStatus{
			Project: strings.TrimPrefix(req.WorkDir, "/work/"), Provider: "generic", Ready: true,
		}}, nil
	}}
	session := connectDirectMCP(t, executor, project.NewStore(path))
	first, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil || first.IsError || got != "/work/ko" {
		t.Fatalf("first call = %#v, err = %v, workdir = %q", first, err, got)
	}
	write("next", "next")
	second, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "next"},
	})
	if err != nil || second.IsError || got != "/work/next" {
		t.Fatalf("next call = %#v, err = %v, workdir = %q", second, err, got)
	}
}

func TestOGMCPUsesLoadedServiceDirectly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfiguredProject(t, home, "https://example.com/owner/repo.git")
	configDir := filepath.Join(home, ".config", "ttal")
	service, err := og.LoadService(filepath.Join(configDir, "og.toml"), configDir)
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	session := connectDirectMCP(t, service, service.ProjectStore())
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), "generic") {
		t.Fatalf("result = %#v", result)
	}
	mutation, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "push", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	mutationContent, _ := json.Marshal(mutation.Content)
	if !mutation.IsError || !strings.Contains(string(mutationContent), "generic HTTPS repository is read-only") {
		t.Fatalf("mutation result = %#v", mutation)
	}
}

func TestOGMCPDefaultsOmittedLogTailWithoutChangingExplicitZero(t *testing.T) {
	var tails []int
	executor := &directExecutor{prLog: func(req og.Request) (og.Response, error) {
		tails = append(tails, req.Tail)
		return og.Response{PR: &og.PullRequest{Index: 17, Title: "logs", State: "open"}, Lines: []string{"line"}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	for _, args := range []map[string]any{
		{"project": "ko", "pr_id": 17},
		{"project": "ko", "pr_id": 17, "tail": 0},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "pr_log", Arguments: args,
		})
		if err != nil || result.IsError {
			t.Fatalf("args = %#v, result = %#v, err = %v", args, result, err)
		}
	}
	if fmt.Sprint(tails) != "[50 0]" {
		t.Fatalf("tails = %v, want [50 0]", tails)
	}
}

func TestOGMCPMissingForgejoTokenIsSecretFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "")
	const remote = "http://forgejo.example/owner/repo.git"
	repo := writeConfiguredProject(t, home, remote)
	runConfiguredGit(t, repo, "remote", "add", "origin", remote)
	writeOGConfig(t, home, `[forgejo]
allowed_base_urls = ["http://forgejo.example"]
`)
	configDir := filepath.Join(home, ".config", "ttal")
	service, err := og.LoadService(filepath.Join(configDir, "og.toml"), configDir)
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	session := connectDirectMCP(t, service, service.ProjectStore())
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "push", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(result.Content)
	if !result.IsError || !strings.Contains(string(content), "missing token: set FORGEJO_TOKEN") {
		t.Fatalf("result = %s", content)
	}
	if strings.Contains(string(content), "secret") {
		t.Fatalf("result contains secret material: %s", content)
	}
}
