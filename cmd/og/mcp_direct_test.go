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
		"auth_status", "clone", "issue_comment", "issue_comments", "issue_create",
		"issue_edit_body", "issue_get", "issue_list", "issue_replace_body",
		"issue_search", "issue_update_title", "pr_checks", "pr_comment", "pr_create",
		"pr_failures", "pr_find", "pr_get", "pr_log", "pr_merge", "pr_modify",
		"project_find", "project_get", "project_list", "pull", "push",
		"repo_diff", "source_read", "source_search", "source_symbols",
	}
	if fmt.Sprint(gotNames) != fmt.Sprint(wantNames) {
		t.Fatalf("tools = %v, want %v", gotNames, wantNames)
	}

	assertDirectMCPToolCalls(t, session, &requests)
}

//nolint:gocyclo // One focused session covers the retained project MCP contracts.
func TestOGMCPProjectToolsPreserveDiscoveryContracts(t *testing.T) {
	home := t.TempDir()
	projectsPath := filepath.Join(home, "projects.toml")
	if err := os.WriteFile(projectsPath, []byte(`[fb]
name = "FlickNote Backend"
path = "/work/flick-backend"
remote = "https://github.com/tta-lab/flick-backend.git"

[demo]
path = "/work/demo"
remote = "https://github.com/tta-lab/demo.git"

[archived.old]
path = "/work/old"
remote = "https://github.com/tta-lab/old.git"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	references := filepath.Join(home, "references")
	if err := os.MkdirAll(filepath.Join(references, "github.com", "other", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	referenceOnly := filepath.Join(references, "github.com", "other", "reference-only")
	if err := os.MkdirAll(referenceOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	session := connectDirectMCP(t, &directExecutor{}, project.NewDiscoveryStore(projectsPath, references))
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if !strings.HasPrefix(tool.Name, "project_") {
			continue
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil ||
			!tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint ||
			tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("project tool contract = %#v", tool)
		}
	}
	byName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	assertProjectSchema := func(name string, required []string, types map[string]string) {
		t.Helper()
		schema, ok := byName[name].InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s schema = %#v", name, byName[name].InputSchema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || properties["alias"] != nil {
			t.Fatalf("%s properties = %#v", name, properties)
		}
		for field, wantType := range types {
			property, ok := properties[field].(map[string]any)
			if !ok || !schemaTypeIncludes(property["type"], wantType) {
				t.Fatalf("%s.%s = %#v, want type %q", name, field, property, wantType)
			}
		}
		for _, field := range required {
			found := false
			for _, value := range schema["required"].([]any) {
				found = found || value == field
			}
			if !found {
				t.Fatalf("%s required = %#v, missing %q", name, schema["required"], field)
			}
		}
	}
	assertProjectSchema("project_get", []string{"project"}, map[string]string{"project": "string"})
	assertProjectSchema("project_find", []string{"query"}, map[string]string{"query": "string", "limit": "integer"})
	assertProjectSchema("project_list", nil, map[string]string{"include_archived": "boolean"})
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if callErr != nil || result.IsError {
			t.Fatalf("%s result = %#v, err = %v", name, result, callErr)
		}
		encoded, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var output map[string]any
		if unmarshalErr := json.Unmarshal(encoded, &output); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		return output
	}
	list := call("project_list", map[string]any{"include_archived": true})
	if projects := list["projects"].([]any); len(projects) != 3 || projects[2].(map[string]any)["archived"] != true {
		t.Fatalf("archive list = %#v", list)
	}
	find := call("project_find", map[string]any{"query": "demo"})
	if projects := find["projects"].([]any); len(projects) != 1 || projects[0].(map[string]any)["path"] != "/work/demo" {
		t.Fatalf("registered precedence = %#v", find)
	}
	find = call("project_find", map[string]any{"query": "reference-only"})
	if projects := find["projects"].([]any); len(projects) != 1 || projects[0].(map[string]any)["path"] != referenceOnly ||
		projects[0].(map[string]any)["reference"] != true {
		t.Fatalf("reference find = %#v", find)
	}
	get := call("project_get", map[string]any{"project": "FLICK-BACKEND"})
	if get["project"].(map[string]any)["alias"] != "fb" {
		t.Fatalf("canonical get = %#v", get)
	}
}

func schemaTypeIncludes(value any, want string) bool {
	if actual, ok := value.(string); ok {
		return actual == want
	}
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, candidate := range values {
		if candidate == want {
			return true
		}
	}
	return false
}

func TestOGMCPDefersUnavailableForgeConfiguration(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projectsPath := filepath.Join(configDir, "projects.toml")
	if err := os.WriteFile(projectsPath, []byte(`[ko]
path = "/work/ko"
remote = "https://github.com/tta-lab/ko.git"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.Mkdir(filepath.Join(configDir, "og"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`[github_app]
app_id = 1
key_source = "file"
key_ref = "og/unavailable.pem"
allowed_owners = ["tta-lab"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := newDeferredExecutor(func() (og.Executor, error) {
		return og.LoadService(configPath, configDir)
	})
	session := connectDirectMCP(t, executor, project.NewDiscoveryStore(projectsPath, filepath.Join(home, "references")))
	projectResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "project_get", Arguments: map[string]any{"project": "ko"},
	})
	if err != nil || projectResult.IsError {
		t.Fatalf("project result = %#v, err = %v", projectResult, err)
	}
	forgeResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "auth_status", Arguments: map[string]any{"project": "ko"},
	})
	text, ok := forgeResult.Content[0].(*mcp.TextContent)
	content := ""
	if ok {
		content = text.Text
	}
	if err != nil || !forgeResult.IsError || !strings.Contains(content, "unavailable.pem") {
		t.Fatalf("forge result = %#v, content = %q, err = %v", forgeResult, content, err)
	}
}

func TestDeferredExecutorLoadsOnceAfterProjectCalls(t *testing.T) {
	loads := 0
	var operations []string
	var requests []og.Request
	executor := newDeferredExecutor(func() (og.Executor, error) {
		loads++
		return &directExecutor{
			authStatus: func(request og.Request) (og.Response, error) {
				operations = append(operations, "auth_status")
				requests = append(requests, request)
				return og.Response{Auth: &og.AuthStatus{Project: "ko", Ready: true}}, nil
			},
			gitPush: func(request og.Request) (og.Response, error) {
				operations = append(operations, "push")
				requests = append(requests, request)
				return og.Response{Message: "pushed"}, nil
			},
		}, nil
	})
	session := connectDirectMCP(t, executor, testProjectStore(t))
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{name: "project_get", args: map[string]any{"project": "ko"}},
		{name: "auth_status", args: map[string]any{"project": "ko"}},
		{name: "push", args: map[string]any{"project": "ko"}},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError {
			t.Fatalf("%s result = %#v, err = %v", call.name, result, err)
		}
	}
	if loads != 1 || fmt.Sprint(operations) != "[auth_status push]" || len(requests) != 2 ||
		requests[0].WorkDir != "/work/ko" || requests[1].WorkDir != "/work/ko" {
		t.Fatalf("loads = %d, operations = %v, requests = %#v", loads, operations, requests)
	}
}

//nolint:gocyclo // One MCP call sequence asserts required and explicit-empty body semantics.
func TestIssueMCPMetadataAndRequiredCreateBody(t *testing.T) {
	calls := 0
	executor := &directExecutor{issueCreate: func(req og.Request) (og.Response, error) {
		calls++
		if req.Body == nil || *req.Body != "" {
			t.Fatalf("body = %#v", req.Body)
		}
		return og.Response{Issue: &og.Issue{Index: 7, Title: "title", URL: "https://example/7"}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range list.Tools {
		tools[tool.Name] = tool
	}
	create := tools["issue_create"]
	if create == nil || create.InputSchema == nil {
		t.Fatalf("issue_create schema = %#v", create)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "issue_create", Arguments: map[string]any{"project": "ko", "title": "title"},
	})
	if err == nil && !result.IsError {
		t.Fatal("issue_create accepted missing required body")
	}
	if calls != 0 {
		t.Fatalf("missing body calls = %d", calls)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "issue_create", Arguments: map[string]any{"project": "ko", "title": "title", "body": ""},
	})
	if err != nil || result.IsError || calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, calls)
	}
	edit := tools["issue_edit_body"]
	if edit == nil || edit.Annotations == nil || edit.Annotations.IdempotentHint {
		t.Fatalf("issue_edit_body annotations = %#v", edit)
	}
}

func TestOGMCPPRChecksExposeNotConfiguredStateAndPolicyMessage(t *testing.T) {
	response := og.Response{
		PR: &og.PullRequest{
			Index: 7,
			CI:    &og.CIStatusResponse{State: "not_configured", Statuses: []og.CIStatus{}},
		},
		Lines: []string{
			"combined: not_configured",
			"CI is not configured for this commit; merge policy allows proceeding without checks",
		},
	}
	executor := &directExecutor{prChecks: func(req og.Request) (og.Response, error) {
		return response, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_checks", Arguments: map[string]any{"project": "ko", "pr_id": 7},
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("MCP checks result = %#v, err = %v", result, err)
	}
	data, marshalErr := json.Marshal(result.StructuredContent)
	if marshalErr != nil || !strings.Contains(string(data), `"state":"not_configured"`) ||
		!strings.Contains(string(data),
			"CI is not configured for this commit; merge policy allows proceeding without checks") {
		t.Fatalf("MCP checks structured result = %s, err = %v", data, marshalErr)
	}
}

func TestOGMCPPRMergeIsDestructiveAndKeepsActionIDOutputOnly(t *testing.T) { //nolint:gocyclo
	executor := &directExecutor{prMerge: func(req og.Request) (og.Response, error) {
		if req.Index != 7 || !req.DryRun {
			t.Fatalf("merge request = %+v", req)
		}
		return og.Response{Merge: &og.PRMergeResult{
			ActionID: "act-1", Status: og.PRMergeStatusPending,
			InboxURL: "http://impri.example/actions",
			Snapshot: og.PRMergeSnapshot{
				PRNumber: 7, PRURL: "https://github.com/tta-lab/ko/pull/7", ExecutionMode: og.PRMergeModeDryRun,
			},
			NextAction: og.PRMergeNextWait,
			Completion: "surface the inbox URL and repeat the same project, PR, and mode " +
				"request after Impri records approved or rejected",
		}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var mergeTool *mcp.Tool
	for _, tool := range list.Tools {
		if tool.Name == "pr_merge" {
			mergeTool = tool
			break
		}
	}
	if mergeTool == nil || mergeTool.Annotations == nil || mergeTool.Annotations.DestructiveHint == nil ||
		!*mergeTool.Annotations.DestructiveHint || mergeTool.Annotations.ReadOnlyHint {
		t.Fatalf("pr_merge tool annotations = %+v", mergeTool)
	}
	schema, ok := mergeTool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema type = %T", mergeTool.InputSchema)
	}
	properties, _ := schema["properties"].(map[string]any)
	if _, exists := properties["api_key"]; exists {
		t.Fatal("pr_merge schema accepts an API key")
	}
	if _, exists := properties["action_id"]; exists {
		t.Fatal("pr_merge schema accepts an action ID input")
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_merge", Arguments: map[string]any{
			"project": "ko", "pr_id": 7, "dry_run": true,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("pr_merge result = %#v, err = %v", result, err)
	}
	data, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(data), "act-1") || !strings.Contains(string(data), "inbox_url") ||
		!strings.Contains(string(data), "wait_for_approval") || !strings.Contains(string(data), "completion") {
		t.Fatalf("structured result = %s", data)
	}
}

func TestOGMCPPRMergeRetryableErrorKeepsStructuredRecoveryOutcome(t *testing.T) {
	merge := og.PRMergeResult{
		ActionID: "act-retry", Status: og.PRMergeStatusApproved,
		InboxURL: "http://impri.example/actions", Retryable: true,
		NextAction: og.PRMergeNextRetry,
		Completion: "repeat the same project, PR, and mode request after the temporary " +
			"failure; execution completes only when status is executed",
		Snapshot: og.PRMergeSnapshot{
			PRNumber: 7, PRURL: "https://github.com/tta-lab/ko/pull/7", ExecutionMode: og.PRMergeModeReal,
		},
		Detail: "impri action remains approved; repeat the same request: provider unavailable",
	}
	executor := &directExecutor{prMerge: func(req og.Request) (og.Response, error) {
		return og.Response{Error: merge.Detail, Merge: &merge}, &og.PRMergeRetryableError{Result: merge}
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_merge", Arguments: map[string]any{"project": "ko", "pr_id": 7},
	})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("retryable MCP result = %#v, err = %v", result, err)
	}
	data, marshalErr := json.Marshal(result.StructuredContent)
	if marshalErr != nil || !strings.Contains(string(data), `"status":"approved"`) ||
		!strings.Contains(string(data), `"retryable":true`) ||
		!strings.Contains(string(data), `"next_action":"retry_same_request"`) ||
		!strings.Contains(string(data), "act-retry") {
		t.Fatalf("retryable structured result = %s, err = %v", data, marshalErr)
	}
}

func TestOGMCPPRMergeUsesSharedTimeoutNormalization(t *testing.T) {
	var got og.Request
	calls := 0
	executor := &directExecutor{prMerge: func(req og.Request) (og.Response, error) {
		calls++
		got = req
		return og.Response{Merge: &og.PRMergeResult{
			ActionID: "act-timeout", Status: og.PRMergeStatusPending,
			InboxURL:   "http://impri.example/actions",
			Snapshot:   og.PRMergeSnapshot{PRNumber: 7, PRURL: "https://github.com/tta-lab/ko/pull/7"},
			NextAction: og.PRMergeNextWait,
			Completion: "surface the inbox URL and repeat the same project, PR, and mode " +
				"request after Impri records approved or rejected",
		}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_merge", Arguments: map[string]any{
			"project": "ko", "pr_id": 7, "wait": true, "timeout_seconds": 0,
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("zero timeout result = %#v, err = %v", result, err)
	}
	if got.Timeout != og.DefaultPRMergeTimeout {
		t.Fatalf("MCP timeout = %s, want %s", got.Timeout, og.DefaultPRMergeTimeout)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_merge", Arguments: map[string]any{
			"project": "ko", "pr_id": 7, "wait": true, "timeout_seconds": -1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || calls != 1 {
		t.Fatalf("negative timeout result = %#v, calls = %d, want schema error without execution", result, calls)
	}
}

func TestOGMCPPRMergeUnavailableOutcomesKeepStructuredRecoveryContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot og.PRMergeSnapshot
	}{
		{name: "approval read"},
		{name: "wait poll", snapshot: og.PRMergeSnapshot{
			PRNumber: 7, PRURL: "https://github.com/tta-lab/ko/pull/7",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merge := og.PRMergeResult{
				ActionID: "act-unavailable", Status: og.PRMergeStatusUnavailable,
				InboxURL: "http://impri.example/actions", Snapshot: tc.snapshot,
				Retryable: true, NextAction: og.PRMergeNextRetry,
				Completion: "repeat the same project, PR, and mode request when Impri approval " +
					"state is available; completion requires a known Impri status",
				Detail: "Impri approval state is temporarily unavailable; no forge call was made; repeat the same request",
			}
			executor := &directExecutor{prMerge: func(req og.Request) (og.Response, error) {
				return og.Response{Error: merge.Detail, Merge: &merge}, &og.PRMergeRetryableError{Result: merge}
			}}
			session := connectDirectMCP(t, executor, testProjectStore(t))
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "pr_merge", Arguments: map[string]any{
					"project": "ko", "pr_id": 7, "wait": tc.name == "wait poll",
				},
			})
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("result = %#v, err = %v, want structured MCP error", result, err)
			}
			data, marshalErr := json.Marshal(result.StructuredContent)
			if marshalErr != nil || !strings.Contains(string(data), `"status":"unavailable"`) ||
				!strings.Contains(string(data), `"retryable":true`) ||
				!strings.Contains(string(data), `"next_action":"retry_same_request"`) ||
				!strings.Contains(string(data), "no forge call was made") {
				t.Fatalf("structured result = %s, err = %v", data, marshalErr)
			}
		})
	}
}

func TestOGMCPPRMergeFailedReceiptKeepsApprovedStructuredOutcome(t *testing.T) {
	merge := og.PRMergeResult{
		ActionID: "act-failed-receipt", Status: og.PRMergeStatusApproved,
		InboxURL: "http://impri.example/actions", Retryable: true,
		NextAction: og.PRMergeNextRetry,
		Completion: "repeat the same request to revalidate and record the deterministic " +
			"execute_failed result; completion is confirmed when Impri reports execute_failed",
		Snapshot: og.PRMergeSnapshot{PRNumber: 7, PRURL: "https://github.com/tta-lab/ko/pull/7"},
		Detail: "impri action act-failed-receipt remains approved; repeat the same request: " +
			"deterministic execution failure could not be recorded in Impri; " +
			"repeat the same request to revalidate and report it",
	}
	executor := &directExecutor{prMerge: func(req og.Request) (og.Response, error) {
		return og.Response{Error: merge.Detail, Merge: &merge}, &og.PRMergeRetryableError{Result: merge}
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pr_merge", Arguments: map[string]any{
			"project": "ko", "pr_id": 7,
		},
	})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("result = %#v, err = %v, want structured receipt-retry error", result, err)
	}
	data, marshalErr := json.Marshal(result.StructuredContent)
	if marshalErr != nil || !strings.Contains(string(data), `"status":"approved"`) ||
		!strings.Contains(string(data), `"next_action":"retry_same_request"`) ||
		!strings.Contains(string(data), "could not be recorded") ||
		strings.Contains(string(data), "repair_receipt") || strings.Contains(string(data), "receipt_error") {
		t.Fatalf("structured result = %s, err = %v", data, marshalErr)
	}
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

func TestOGMCPResolvesAlternateProjectReferenceAndReturnsCanonicalAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	content := "[fb]\npath = \"/work/flick-backend\"\nremote = \"https://example.com/owner/flick-backend.git\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var got og.Request
	executor := &directExecutor{
		gitPush: func(req og.Request) (og.Response, error) {
			got = req
			return og.Response{Message: "pushed"}, nil
		},
		gitClone: func(req og.Request) (og.Response, error) {
			got = req
			return og.Response{Clone: &og.CloneResult{
				Alias: "fb", Path: "/work/flick-backend", Host: "example.com", Owner: "owner", Repo: "flick-backend",
				Provider: "generic", Remote: "https://example.com/owner/flick-backend.git", Registered: true,
			}}, nil
		},
	}
	session := connectDirectMCP(t, executor, project.NewStore(path))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "push", Arguments: map[string]any{"project": "FLICK-BACKEND"},
	})
	if err != nil || result.IsError {
		t.Fatalf("alternate push = %#v, err = %v", result, err)
	}
	if got.WorkDir != "/work/flick-backend" {
		t.Fatalf("push request = %+v, want resolved checkout", got)
	}
	var output ogMessageOutput
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if output.Project != "fb" {
		t.Fatalf("push project = %q, want canonical fb", output.Project)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clone", Arguments: map[string]any{"project": "flick-backend"},
	})
	if err != nil || result.IsError {
		t.Fatalf("alternate clone = %#v, err = %v", result, err)
	}
	if got.Project != "flick-backend" {
		t.Fatalf("clone request = %+v, want caller reference forwarded to the domain", got)
	}
	var cloneOutput ogCloneOutput
	encoded, err = json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &cloneOutput); err != nil {
		t.Fatal(err)
	}
	if cloneOutput.Clone.Alias != "fb" {
		t.Fatalf("clone output alias = %q, want canonical fb", cloneOutput.Clone.Alias)
	}
}

func TestOGMCPRejectsUnknownProjectBeforeExecutorWithRecovery(t *testing.T) {
	called := false
	executor := &directExecutor{gitPush: func(og.Request) (og.Response, error) {
		called = true
		return og.Response{Message: "unexpected"}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "push", Arguments: map[string]any{"project": "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(result.Content)
	if !result.IsError ||
		!strings.Contains(string(content), "og project find") ||
		!strings.Contains(string(content), "og project list") {
		t.Fatalf("result = %#v, want shared recovery tool error", result)
	}
	if called {
		t.Fatal("executor called for unknown project")
	}
}

func TestOGProjectTargetingSeamNormalizesEveryRegisteredOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.toml")
	content := "[fb]\npath = \"/work/flick-backend\"\nremote = \"https://example.com/owner/flick-backend.git\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := project.NewStore(path)
	record := func(req og.Request) (og.Response, error) {
		if req.WorkDir != "/work/flick-backend" || req.Context == nil {
			t.Fatalf("request = %+v, want resolved checkout and context", req)
		}
		return og.Response{}, nil
	}
	executor := &directExecutor{
		gitPush: record, gitPull: record, gitTag: record,
		prCreate: record, prView: record, prFind: record, prGet: record,
		prModify: record, prComment: record, prChecks: record, prLog: record,
		prFailures: record, authStatus: record,
	}
	operations := []struct {
		name string
		op   func(og.Request) (og.Response, error)
	}{
		{name: "push", op: executor.GitPush}, {name: "pull", op: executor.GitPull},
		{name: "tag", op: executor.GitTag}, {name: "pr create", op: executor.PRCreate},
		{name: "pr view", op: executor.PRView}, {name: "pr find", op: executor.PRFind},
		{name: "pr get", op: executor.PRGet}, {name: "pr modify", op: executor.PRModify},
		{name: "pr comment", op: executor.PRComment}, {name: "pr checks", op: executor.PRChecks},
		{name: "pr log", op: executor.PRLog}, {name: "pr failures", op: executor.PRFailures},
		{name: "auth status", op: executor.AuthStatus},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			_, canonical, err := callProject(context.Background(), projects, "flick-backend", og.Request{}, operation.op)
			if err != nil {
				t.Fatal(err)
			}
			if canonical != "fb" {
				t.Fatalf("canonical alias = %q, want fb", canonical)
			}
		})
	}
}
