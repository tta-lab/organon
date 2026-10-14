package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/og"
)

func TestMCPPREditBodyContract(t *testing.T) {
	edits := []og.BodyEdit{{OldText: "old\ntext", NewText: "new\ntext"}, {OldText: "delete", NewText: ""}}
	calls := 0
	expectedID := int64(7)
	executor := &directExecutor{prEditBody: func(req og.Request) (og.Response, error) {
		calls++
		if req.Index != expectedID || req.WorkDir != "/work/ko" || req.Context == nil ||
			!reflect.DeepEqual(req.Edits, edits) {
			t.Fatalf("request=%+v", req)
		}
		return og.Response{PR: &og.PullRequest{Index: 7, Title: "kept", Body: "updated"}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var editTool *mcp.Tool
	for _, tool := range list.Tools {
		if tool.Name == "pr_edit_body" {
			editTool = tool
		}
	}
	assertPREditToolAnnotations(t, editTool)
	for _, explicit := range []bool{true, false} {
		args := map[string]any{"project": "KO", "edits": edits}
		expectedID = 0
		if explicit {
			expectedID = 7
			args["pr_id"] = 7
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pr_edit_body", Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		assertPREditOutput(t, result)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestMCPPREditBodyRejectsInvalidInput(t *testing.T) {
	edits := []og.BodyEdit{{OldText: "old", NewText: "new"}}
	calls := 0
	executor := &directExecutor{prEditBody: func(og.Request) (og.Response, error) {
		calls++
		return og.Response{PR: &og.PullRequest{Index: 7}}, nil
	}}
	session := connectDirectMCP(t, executor, testProjectStore(t))

	for _, args := range []map[string]any{
		{"project": "ko", "pr_id": 0, "edits": edits},
		{"project": "ko", "pr_id": -1, "edits": edits},
		{"project": "ko", "edits": []og.BodyEdit{}},
		{"project": "ko", "edits": []og.BodyEdit{{NewText: "x"}}},
		{"project": "ko", "edits": []og.BodyEdit{{OldText: "x", NewText: "x"}}},
		{"project": "ko", "edits": []og.BodyEdit{{OldText: "x", NewText: "y"}, {OldText: "x", NewText: "z"}}},
		{"project": "ko"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pr_edit_body", Arguments: args})
		if err == nil && !result.IsError {
			t.Fatalf("invalid args succeeded: %+v", args)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input reached executor %d times", calls)
	}
}

func TestMCPPREditBodyRejectsInvalidIdentity(t *testing.T) {
	for _, pr := range []*og.PullRequest{nil, {Index: 0}, {Index: 99}} {
		executor := &directExecutor{prEditBody: func(og.Request) (og.Response, error) { return og.Response{PR: pr}, nil }}
		id := int64(7)
		_, _, err := prEditBodyHandler(testProjectStore(t), executor)(context.Background(), nil, ogPREditsInput{
			Project: "ko", PRID: &id, Edits: []og.BodyEdit{{OldText: "a", NewText: "b"}},
		})
		if err == nil {
			t.Fatalf("accepted PR %+v", pr)
		}
	}
}

func assertPREditToolAnnotations(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	if tool == nil || tool.Annotations == nil {
		t.Fatalf("tool=%+v", tool)
	}
	a := tool.Annotations
	if a.ReadOnlyHint || a.IdempotentHint || a.DestructiveHint == nil || !*a.DestructiveHint {
		t.Fatalf("annotations=%+v", a)
	}
}

func assertPREditOutput(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output ogPROutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.Project != "ko" || output.PR.Index != 7 || output.PR.Body != "updated" {
		t.Fatalf("output=%+v", output)
	}
}
