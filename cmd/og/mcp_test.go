package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

type recordingOGCaller struct {
	call func(context.Context, string, og.Request) (og.Response, error)
}

func (c recordingOGCaller) CallContext(ctx context.Context, path string, req og.Request) (og.Response, error) {
	return c.call(ctx, path, req)
}

func testOGStore(t *testing.T) *project.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "projects.toml")
	content := "[organon]\nname = \"Organon\"\n" +
		"path = \"/work/code/projects/tta-lab/organon\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return project.NewStore(path)
}

func connectOGMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "og-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestOGMCPListsCLIParityTools(t *testing.T) {
	caller := recordingOGCaller{call: func(context.Context, string, og.Request) (og.Response, error) {
		return og.Response{}, nil
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"auth_status", "clone", "pr_checks", "pr_comment", "pr_create", "pr_failures", "pr_find", "pr_get",
		"pr_log", "pr_modify", "pull", "push",
	}
	gotNames := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		gotNames = append(gotNames, tool.Name)
		assertOGToolContract(t, tool)
	}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("tools = %v, want %v", gotNames, wantNames)
	}
}

func assertOGToolContract(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	if tool.InputSchema == nil || tool.OutputSchema == nil {
		t.Fatalf("tool %q must have generated schemas", tool.Name)
	}
	assertOGToolAnnotations(t, tool)
	properties := decodeToolProperties(t, tool)
	assertOGToolSelectorSchema(t, tool, properties)
	assertOGToolPRIDSchema(t, tool, properties)
	assertRequiredInput(t, tool)
	assertOGToolDefaults(t, tool, properties)
}

func assertOGToolSelectorSchema(t *testing.T, tool *mcp.Tool, properties map[string]*jsonschema.Schema) {
	t.Helper()
	if tool.Name != "clone" && properties["project"] == nil {
		t.Fatalf("tool %q input lacks project", tool.Name)
	}
	if tool.Name == "clone" &&
		(properties["url"] == nil || properties["alias"] == nil || properties["reference"] == nil) {
		t.Fatalf("clone input properties = %v", properties)
	}
	for _, forbidden := range []string{
		"work_dir", "path", "cwd", "root", "roots", "uri", "file_uri", "token", "token_env",
	} {
		if properties[forbidden] != nil {
			t.Fatalf("tool %q exposes forbidden input %q", tool.Name, forbidden)
		}
	}
}

func assertOGToolPRIDSchema(t *testing.T, tool *mcp.Tool, properties map[string]*jsonschema.Schema) {
	t.Helper()
	if ogToolHasPRID(tool.Name) && properties["pr_id"] == nil {
		t.Fatalf("tool %q input lacks pr_id", tool.Name)
	}
	if !ogToolHasPRID(tool.Name) && properties["pr_id"] != nil {
		t.Fatalf("tool %q unexpectedly exposes pr_id", tool.Name)
	}
	if prID := properties["pr_id"]; prID != nil {
		if prID.Minimum == nil || *prID.Minimum != 1 {
			t.Fatalf("tool %q pr_id minimum = %#v, want 1", tool.Name, prID.Minimum)
		}
	}
}

func assertOGToolDefaults(t *testing.T, tool *mcp.Tool, properties map[string]*jsonschema.Schema) {
	t.Helper()
	if tool.Name == "pr_log" || tool.Name == "pr_failures" {
		assertTailSchema(t, tool.Name, properties["tail"])
	}
	if tool.Name == "push" {
		if force := properties["force"]; force == nil || string(force.Default) != "false" {
			t.Fatalf("tool %q force schema = %#v", tool.Name, force)
		}
	}
	if tool.Name == "clone" {
		if reference := properties["reference"]; reference == nil || string(reference.Default) != "false" {
			t.Fatalf("tool %q reference schema = %#v", tool.Name, reference)
		}
	}
	if tool.Name == "pr_find" {
		state := properties["state"]
		if state == nil || string(state.Default) != `"open"` ||
			!reflect.DeepEqual(state.Enum, []any{"open", "closed", "all"}) {
			t.Fatalf("tool %q state schema = %#v", tool.Name, state)
		}
	}
}

func ogToolHasPRID(name string) bool {
	switch name {
	case "pr_get", "pr_modify", "pr_comment", "pr_checks", "pr_log", "pr_failures":
		return true
	default:
		return false
	}
}

func assertRequiredInput(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	schemaJSON, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		t.Fatal(err)
	}
	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}
	if tool.Name != "clone" && !required["project"] {
		t.Fatalf("tool %q required inputs = %v", tool.Name, schema.Required)
	}
	if tool.Name == "clone" && (!required["url"] || required["project"] || required["alias"] || required["reference"]) {
		t.Fatalf("clone required inputs = %v", schema.Required)
	}
	if required["pr_id"] {
		t.Fatalf("tool %q unexpectedly requires pr_id: %v", tool.Name, schema.Required)
	}
	if tool.Name == "pr_create" && !required["title"] {
		t.Fatalf("tool %q required inputs = %v, want title", tool.Name, schema.Required)
	}
}

func assertOGToolAnnotations(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	annotations := tool.Annotations
	if annotations == nil || annotations.OpenWorldHint == nil || !*annotations.OpenWorldHint {
		t.Fatalf("tool %q annotations = %#v, want open-world", tool.Name, annotations)
	}
	wantReadOnly, wantDestructive, wantIdempotent := true, false, true
	if tool.Name == "pr_modify" || tool.Name == "push" || tool.Name == "pull" {
		wantReadOnly, wantDestructive = false, true
	}
	if tool.Name == "pr_comment" || tool.Name == "pr_create" {
		wantReadOnly, wantIdempotent = false, false
	}
	if tool.Name == "clone" {
		wantReadOnly, wantDestructive, wantIdempotent = false, false, true
	}
	if annotations.ReadOnlyHint != wantReadOnly || annotations.IdempotentHint != wantIdempotent ||
		annotations.DestructiveHint == nil || *annotations.DestructiveHint != wantDestructive {
		t.Fatalf("tool %q annotations = %#v", tool.Name, annotations)
	}
}

func decodeToolProperties(t *testing.T, tool *mcp.Tool) map[string]*jsonschema.Schema {
	t.Helper()
	schemaJSON, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var inputSchema jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &inputSchema); err != nil {
		t.Fatalf("decode tool %q input schema: %v", tool.Name, err)
	}
	return inputSchema.Properties
}

func assertTailSchema(t *testing.T, name string, tail *jsonschema.Schema) {
	t.Helper()
	if tail == nil || tail.Minimum == nil || *tail.Minimum != 0 {
		t.Fatalf("tool %q tail minimum = %#v", name, tail)
	}
	if tail.Maximum == nil || *tail.Maximum != 1000 || string(tail.Default) != "50" {
		t.Fatalf("tool %q tail schema = %#v", name, tail)
	}
}

func TestOGMCPResolvesAliasAndReturnsStructuredResults(t *testing.T) {
	pr := &og.PullRequest{Index: 17, Title: "typed MCP", Body: "body", State: "open", URL: "https://forge/pr/17"}
	comment := &og.Comment{ID: 44, PRID: 17, Body: "line one\nline two", URL: "https://forge/pr/17#comment-44"}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), structuredOGCaller(t, pr, comment)))
	tests := []struct {
		name string
		args map[string]any
		key  string
	}{
		{name: "auth_status", args: map[string]any{"project": "organon"}, key: "auth"},
		{name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 17}, key: "pr"},
		{name: "pr_checks", args: map[string]any{"project": "organon", "pr_id": 17}, key: "lines"},
		{name: "pr_log", args: map[string]any{"project": "organon", "pr_id": 17}, key: "lines"},
		{name: "pr_failures", args: map[string]any{"project": "organon", "pr_id": 17}, key: "lines"},
		{name: "pr_comment", args: map[string]any{"project": "organon", "pr_id": 17, "body": comment.Body}, key: "comment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStructuredOGCall(t, session, tt.name, tt.args, tt.key)
		})
	}
}

func TestOGMCPCloneRoutesURLWithoutProjectPathOrToken(t *testing.T) {
	want := &og.CloneResult{
		Alias: "example", Path: "/home/neil/code/projects/tta-lab/example", Host: "codeberg.org",
		Owner: "tta-lab", Repo: "example", Provider: "generic",
		Remote: "https://codeberg.org/tta-lab/example.git", Registered: true,
	}
	caller := recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		if path != "/git/clone" {
			t.Fatalf("daemon path = %q", path)
		}
		if req.URL != "https://codeberg.org/tta-lab/example.git" || req.Alias != "example" || req.Reference {
			t.Fatalf("clone request = %+v", req)
		}
		if req.WorkDir != "" {
			t.Fatalf("clone work_dir = %q", req.WorkDir)
		}
		return og.Response{OK: true, Clone: want}, nil
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clone", Arguments: map[string]any{
			"url": "https://codeberg.org/tta-lab/example.git", "alias": "example",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), want.Path) ||
		!strings.Contains(fmt.Sprint(result.StructuredContent), want.Remote) {
		t.Fatalf("clone result = %#v", result)
	}
}

func TestOGMCPReadsNewAliasWithoutRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(path, []byte("[organon]\npath = \"/work/organon\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	caller := recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		return og.Response{OK: true, Auth: &og.AuthStatus{Project: filepath.Base(req.WorkDir), Ready: true}}, nil
	}}
	session := connectOGMCP(t, newOGMCPServer(project.NewStore(path), caller))
	if err := os.WriteFile(path, []byte(
		"[organon]\npath = \"/work/organon\"\n\n[newrepo]\npath = \"/work/newrepo\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "newrepo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("hot alias result = %#v", result)
	}
}

func TestOGMCPRoutesWorktreeToolsThroughRegisteredAlias(t *testing.T) {
	pr := &og.PullRequest{
		Index: 17, Title: "typed MCP", Body: "body", State: "open", URL: "https://forge/pr/17",
		Head: "feature/mcp", Base: "main",
	}
	caller := recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		if req.WorkDir != "/work/code/projects/tta-lab/organon" {
			t.Fatalf("work_dir = %q", req.WorkDir)
		}
		switch path {
		case "/git/push":
			if !req.Force {
				t.Fatal("force = false, want true")
			}
			return og.Response{OK: true, Message: "force-pushed feature/mcp"}, nil
		case "/git/pull":
			return og.Response{OK: true, Message: "Merged PR. Pulled main. Deleted feature/mcp locally and remotely"}, nil
		case "/pr/create":
			if req.Title == nil || *req.Title != "typed MCP" || req.Body == nil || *req.Body != "body" {
				t.Fatalf("create request = %+v", req)
			}
			return og.Response{OK: true, PR: pr}, nil
		case "/pr/find":
			if req.State != "open" {
				t.Fatalf("find state = %q, want open", req.State)
			}
			return og.Response{OK: true, PR: pr}, nil
		case "/pr/view":
			if req.State != "all" {
				t.Fatalf("view state = %q, want all", req.State)
			}
			return og.Response{OK: true, PR: pr}, nil
		default:
			t.Fatalf("unexpected daemon path %q", path)
			return og.Response{}, nil
		}
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	tests := []struct {
		name string
		args map[string]any
		key  string
	}{
		{name: "push", args: map[string]any{"project": "organon", "force": true}, key: "message"},
		{name: "pull", args: map[string]any{"project": "organon"}, key: "message"},
		{name: "pr_create", args: map[string]any{"project": "organon", "title": "typed MCP", "body": "body"}, key: "pr"},
		{name: "pr_find", args: map[string]any{"project": "organon"}, key: "pr"},
		{name: "pr_get", args: map[string]any{"project": "organon"}, key: "pr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStructuredOGCall(t, session, tt.name, tt.args, tt.key)
		})
	}
}

func TestOGMCPCurrentBranchPRToolsAllowOmittedPRID(t *testing.T) {
	pr := &og.PullRequest{Index: 17, Title: "current branch", URL: "https://forge/pr/17"}
	commentBody := "current branch comment"
	comment := &og.Comment{ID: 3, PRID: 17, Body: commentBody, URL: "https://forge/pr/17#comment-3"}
	caller := recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		if req.WorkDir != "/work/code/projects/tta-lab/organon" || req.Index != 0 {
			t.Fatalf("request = %+v", req)
		}
		switch path {
		case "/pr/view":
			if req.State != "all" {
				t.Fatalf("view state = %q, want all", req.State)
			}
			return og.Response{OK: true, PR: pr}, nil
		case "/pr/modify":
			return og.Response{OK: true, PR: pr}, nil
		case "/pr/comment":
			return og.Response{OK: true, Comment: comment}, nil
		case "/pr/checks", "/pr/log", "/pr/failures":
			return og.Response{OK: true, PR: pr, Lines: []string{"line"}}, nil
		default:
			t.Fatalf("unexpected daemon path %q", path)
			return og.Response{}, nil
		}
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	tests := []struct {
		name string
		args map[string]any
		key  string
	}{
		{name: "pr_get", args: map[string]any{"project": "organon"}, key: "pr"},
		{name: "pr_modify", args: map[string]any{"project": "organon", "title": "current branch"}, key: "pr"},
		{name: "pr_comment", args: map[string]any{"project": "organon", "body": commentBody}, key: "comment"},
		{name: "pr_checks", args: map[string]any{"project": "organon"}, key: "lines"},
		{name: "pr_log", args: map[string]any{"project": "organon"}, key: "lines"},
		{name: "pr_failures", args: map[string]any{"project": "organon"}, key: "lines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStructuredOGCall(t, session, tt.name, tt.args, tt.key)
		})
	}
}

func structuredOGCaller(t *testing.T, pr *og.PullRequest, comment *og.Comment) recordingOGCaller {
	t.Helper()
	return recordingOGCaller{call: func(ctx context.Context, path string, req og.Request) (og.Response, error) {
		if req.WorkDir != "/work/code/projects/tta-lab/organon" {
			t.Fatalf("work_dir = %q", req.WorkDir)
		}
		if path != "/auth/status" && req.Index != 17 {
			t.Fatalf("index = %d for %s", req.Index, path)
		}
		switch path {
		case "/auth/status":
			return og.Response{OK: true, Auth: &og.AuthStatus{
				Project: "organon", Provider: "github", Host: "github.com",
				Owner: "tta-lab", Repo: "organon", AuthMode: "github-app", Ready: true,
			}}, nil
		case "/pr/comment":
			if req.Body == nil || *req.Body != comment.Body {
				t.Fatalf("comment body = %#v", req.Body)
			}
			return og.Response{OK: true, Comment: comment}, nil
		case "/pr/log", "/pr/failures":
			if req.Tail != 50 {
				t.Fatalf("tail = %d, want default 50", req.Tail)
			}
			return og.Response{OK: true, PR: pr, Lines: []string{"line"}}, nil
		default:
			return og.Response{OK: true, PR: pr, Lines: []string{"check"}}, nil
		}
	}}
}

func assertStructuredOGCall(
	t *testing.T,
	session *mcp.ClientSession,
	name string,
	args map[string]any,
	key string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool error: %#v", result.Content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["project"] != "organon" || got[key] == nil {
		t.Fatalf("structured content = %#v", got)
	}
}

func TestOGMCPPassesCancellationToDaemon(t *testing.T) {
	tools := []struct {
		name string
		args map[string]any
	}{
		{name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 17}},
		{name: "clone", args: map[string]any{"url": "https://codeberg.org/tta-lab/example.git"}},
	}
	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			assertOGMCPCancellation(t, tool.name, tool.args)
		})
	}
}

func assertOGMCPCancellation(t *testing.T, name string, args map[string]any) {
	t.Helper()
	started := make(chan struct{})
	canceled := make(chan struct{})
	caller := recordingOGCaller{call: func(ctx context.Context, _ string, _ og.Request) (og.Response, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return og.Response{}, ctx.Err()
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("daemon call did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("daemon call context was not canceled")
	}
	<-done
}

func TestOGMCPModifyPreservesFieldPresence(t *testing.T) {
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), modifyPresenceCaller(t)))
	for _, args := range []map[string]any{
		{"project": "organon", "pr_id": 23, "title": "new title"},
		{"project": "organon", "pr_id": 23, "body": "line one\nline two"},
		{"project": "organon", "pr_id": 23, "body": ""},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "pr_modify", Arguments: args,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("tool error: %#v", result.Content)
		}
	}
}

func modifyPresenceCaller(t *testing.T) recordingOGCaller {
	t.Helper()
	title := "new title"
	multiline := "line one\nline two"
	empty := ""
	wants := []struct {
		title        *string
		body         *string
		responseBody string
	}{
		{title: &title, responseBody: "old"},
		{body: &multiline, responseBody: multiline},
		{body: &empty, responseBody: empty},
	}
	var calls int
	return recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		if calls >= len(wants) {
			t.Fatalf("unexpected call %d", calls+1)
			return og.Response{}, nil
		}
		want := wants[calls]
		calls++
		assertModifyRequest(t, path, req, want.title, want.body)
		return og.Response{OK: true, PR: &og.PullRequest{
			Index: 23, Title: "new title", Body: want.responseBody, URL: "https://forge/pr/23",
		}}, nil
	}}
}

func assertModifyRequest(t *testing.T, path string, req og.Request, title, body *string) {
	t.Helper()
	if path != "/pr/modify" || req.WorkDir != "/work/code/projects/tta-lab/organon" {
		t.Fatalf("call = %s %+v", path, req)
	}
	if req.Index != 23 || !reflect.DeepEqual(req.Title, title) || !reflect.DeepEqual(req.Body, body) {
		t.Fatalf("modify request = %+v, want title %#v body %#v", req, title, body)
	}
}

func TestOGMCPRejectsInvalidInputsAndDaemonFailuresAsToolErrors(t *testing.T) {
	caller := recordingOGCaller{call: func(_ context.Context, _ string, _ og.Request) (og.Response, error) {
		return og.Response{}, fmt.Errorf("daemon unavailable")
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "pr_get", args: map[string]any{"project": "organon.child", "pr_id": 1}},
		{name: "pr_get", args: map[string]any{"project": "/work/code/projects/tta-lab/organon", "pr_id": 1}},
		{name: "pr_get", args: map[string]any{"project": "missing", "pr_id": 1}},
		{name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 0}},
		{name: "pr_modify", args: map[string]any{"project": "organon", "pr_id": -1, "title": "title"}},
		{name: "pr_comment", args: map[string]any{"project": "organon", "pr_id": -1, "body": "body"}},
		{name: "pr_checks", args: map[string]any{"project": "organon", "pr_id": -1}},
		{name: "pr_modify", args: map[string]any{"project": "organon", "pr_id": 1}},
		{name: "pr_modify", args: map[string]any{"project": "organon", "pr_id": 1, "title": "  "}},
		{name: "pr_comment", args: map[string]any{"project": "organon", "pr_id": 1, "body": " \n"}},
		{name: "pr_log", args: map[string]any{"project": "organon", "pr_id": 1, "tail": 1001}},
		{name: "pr_failures", args: map[string]any{"project": "organon", "pr_id": 1, "tail": -1}},
		{name: "pr_create", args: map[string]any{"project": "organon", "title": "  "}},
		{name: "pr_find", args: map[string]any{"project": "organon", "state": "merged"}},
		{name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 1, "work_dir": "/tmp"}},
		{name: "auth_status", args: map[string]any{"project": "organon"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+fmt.Sprint(tt.args), func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("protocol error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("result = %#v, want tool error", result)
			}
		})
	}
}

func TestOGMCPRejectsExplicitZeroOptionalPRID(t *testing.T) {
	caller := recordingOGCaller{call: func(_ context.Context, path string, req og.Request) (og.Response, error) {
		switch path {
		case "/pr/modify":
			return og.Response{OK: true, PR: &og.PullRequest{Index: 22, Title: *req.Title}}, nil
		case "/pr/comment":
			return og.Response{OK: true, Comment: &og.Comment{
				ID: 3, PRID: 22, Body: *req.Body, URL: "https://forge/pr/22#comment-3",
			}}, nil
		default:
			return og.Response{OK: true, PR: &og.PullRequest{Index: 22}}, nil
		}
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 0}},
		{name: "pr_modify", args: map[string]any{"project": "organon", "pr_id": 0, "title": "title"}},
		{name: "pr_comment", args: map[string]any{"project": "organon", "pr_id": 0, "body": "body"}},
		{name: "pr_checks", args: map[string]any{"project": "organon", "pr_id": 0}},
		{name: "pr_log", args: map[string]any{"project": "organon", "pr_id": 0}},
		{name: "pr_failures", args: map[string]any{"project": "organon", "pr_id": 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("protocol error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("result = %#v, want explicit zero PR ID tool error", result)
			}
		})
	}
}

func TestOGMCPRejectsMismatchedDaemonIdentity(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		resp og.Response
	}{
		{
			name: "auth_status", args: map[string]any{"project": "organon"},
			resp: og.Response{OK: true, Auth: &og.AuthStatus{Project: "other", Ready: true}},
		},
		{
			name: "pr_get", args: map[string]any{"project": "organon", "pr_id": 7},
			resp: og.Response{OK: true, PR: &og.PullRequest{Index: 8, URL: "https://forge/pr/8"}},
		},
		{
			name: "pr_modify", args: map[string]any{"project": "organon", "pr_id": 7, "title": "new"},
			resp: og.Response{OK: true, PR: &og.PullRequest{Index: 8, Title: "new", URL: "https://forge/pr/8"}},
		},
		{
			name: "pr_comment", args: map[string]any{"project": "organon", "pr_id": 7, "body": "body"},
			resp: og.Response{OK: true, Comment: &og.Comment{
				ID: 1, PRID: 8, Body: "body", URL: "https://forge/pr/8#comment-1",
			}},
		},
		{
			name: "clone", args: map[string]any{"url": "https://codeberg.org/tta-lab/example.git"},
			resp: og.Response{OK: true, Clone: &og.CloneResult{
				Path: "/tmp/example", Host: "codeberg.org", Owner: "tta-lab", Repo: "example",
				Provider: "unknown", Remote: "https://user:secret@codeberg.org/tta-lab/example.git",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := recordingOGCaller{call: func(context.Context, string, og.Request) (og.Response, error) {
				return tt.resp, nil
			}}
			session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: tt.name, Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("protocol error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("result = %#v, want identity mismatch tool error", result)
			}
		})
	}
}

func TestOGMCPRejectsInvalidWorktreeDaemonResults(t *testing.T) {
	tests := []struct {
		name string
		resp og.Response
	}{
		{name: "push", resp: og.Response{OK: true}},
		{name: "pull", resp: og.Response{OK: true, Message: " \n"}},
		{name: "pr_create", resp: og.Response{OK: true}},
		{name: "pr_find", resp: og.Response{OK: true, PR: &og.PullRequest{URL: "https://forge/pr/0"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := recordingOGCaller{call: func(context.Context, string, og.Request) (og.Response, error) {
				return tt.resp, nil
			}}
			session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
			args := map[string]any{"project": "organon"}
			if tt.name == "pr_create" {
				args["title"] = "title"
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: args})
			if err != nil {
				t.Fatalf("protocol error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("result = %#v, want invalid daemon result tool error", result)
			}
		})
	}
}

func TestOGMCPAcceptsWorktreePRWithoutDisplayURL(t *testing.T) {
	caller := recordingOGCaller{call: func(context.Context, string, og.Request) (og.Response, error) {
		return og.Response{OK: true, PR: &og.PullRequest{Index: 7, Title: "feature"}}, nil
	}}
	session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_get", Arguments: map[string]any{"project": "organon"},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %#v, want positive PR identity accepted", result)
	}
}

func TestOGMCPCommandHelper(_ *testing.T) {
	if os.Getenv("GO_WANT_OG_MCP_HELPER") != "1" {
		return
	}
	cmd := newRootCmd(os.Stdout, os.Stderr)
	cmd.SetArgs([]string{"mcp"})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestOGMCPCommandTransport(t *testing.T) {
	home := t.TempDir()
	writeOGMCPConfig(t, home)
	daemon := newOGMCPTestDaemon(t)
	defer daemon.Close()
	command := exec.Command(os.Args[0], "-test.run=TestOGMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_OG_MCP_HELPER=1", "HOME="+home, "OG_DAEMON_URL="+daemon.URL)
	client := mcp.NewClient(&mcp.Implementation{Name: "og-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	assertOGCommandSession(t, session)
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestOGMCPCommandTransportClonesAllowedForgejoHTTPAndHotRegisters(t *testing.T) {
	home := t.TempDir()
	writeOGMCPConfigContent(t, home, "")
	gitServer := newDumbGitHTTPServer(t)
	defer gitServer.Close()
	t.Setenv("HOME", home)
	store := project.NewStore(filepath.Join(home, ".config", "ttal", "projects.toml"))
	service := og.NewServiceWithConfig(nil, store, ogconfig.Config{
		Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{gitServer.URL}},
	})
	daemon := httptest.NewServer(og.NewMux(service))
	defer daemon.Close()

	command := exec.Command(os.Args[0], "-test.run=TestOGMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_OG_MCP_HELPER=1", "HOME="+home, "OG_DAEMON_URL="+daemon.URL)
	client := mcp.NewClient(&mcp.Implementation{Name: "og-clone-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	defer func() { _ = session.Close() }()

	remote := gitServer.URL + "/tta-lab/example.git"
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clone", Arguments: map[string]any{"url": remote, "alias": "example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(home, "code", "projects", "tta-lab", "example")
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), wantPath) ||
		!strings.Contains(fmt.Sprint(result.StructuredContent), "forgejo") {
		t.Fatalf("clone result = %#v", result)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), "forgejo") {
		t.Fatalf("hot registered auth result = %#v", result)
	}
}

func writeOGMCPConfig(t *testing.T, home string) {
	t.Helper()
	writeOGMCPConfigContent(t, home, "[organon]\npath = \"/work/code/projects/tta-lab/organon\"\n")
}

func writeOGMCPConfigContent(t *testing.T, home, content string) {
	t.Helper()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newDumbGitHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	webRoot := t.TempDir()
	bare := filepath.Join(webRoot, "tta-lab", "example.git")
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, "init", "--bare", bare)
	runGitFixture(t, "init", "-b", "main", work)
	runGitFixture(t, "-C", work, "config", "user.name", "Test")
	runGitFixture(t, "-C", work, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, "-C", work, "add", "README.md")
	runGitFixture(t, "-C", work, "commit", "-m", "fixture")
	runGitFixture(t, "-C", work, "remote", "add", "origin", bare)
	runGitFixture(t, "-C", work, "push", "origin", "main")
	runGitFixture(t, "--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitFixture(t, "--git-dir", bare, "update-server-info")
	return httptest.NewServer(http.FileServer(http.Dir(webRoot)))
}

func runGitFixture(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newOGMCPTestDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["work_dir"] != "/work/code/projects/tta-lab/organon" {
			t.Errorf("request = %#v", req)
		}
		for _, forbidden := range []string{"token", "token_env"} {
			if _, ok := req[forbidden]; ok {
				t.Errorf("request includes %s", forbidden)
			}
		}
		switch r.URL.Path {
		case "/pr/get":
			if req["index"] != float64(31) {
				t.Errorf("request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(og.Response{OK: true, PR: &og.PullRequest{
				Index: 31, Title: "stdio", State: "open", URL: "https://forge/pr/31",
			}})
		case "/pr/view":
			if req["state"] != "all" {
				t.Errorf("request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(og.Response{OK: true, PR: &og.PullRequest{
				Index: 32, Title: "current branch", State: "open", URL: "https://forge/pr/32",
			}})
		case "/git/push":
			if req["force"] != true {
				t.Errorf("request = %#v", req)
			}
			_ = json.NewEncoder(w).Encode(og.Response{OK: true, Message: "pushed with force-with-lease"})
		default:
			t.Errorf("path = %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
}

func assertOGCommandSession(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 12 {
		t.Fatalf("tools = %#v, err = %v", tools, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_get", Arguments: map[string]any{"project": "organon", "pr_id": 31},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), "stdio") {
		t.Fatalf("result = %#v", result)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_get", Arguments: map[string]any{"project": "organon"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), "current branch") {
		t.Fatalf("current branch result = %#v", result)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "push", Arguments: map[string]any{"project": "organon", "force": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(fmt.Sprint(result.StructuredContent), "force-with-lease") {
		t.Fatalf("push result = %#v", result)
	}
}
