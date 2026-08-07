package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/srcview"
)

func writeSrcProjects(t *testing.T, home, alias, root string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "ttal", "projects.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	rootJSON, _ := json.Marshal(root)
	content := "[" + alias + "]\npath = " + string(rootJSON) + "\nremote = \"https://example.com/tta/" + alias + ".git\"\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func connectSrcMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "src-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callSrcTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s tool error: %#v", name, result.Content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func testSrcSession(t *testing.T, files map[string]string) *mcp.ClientSession {
	t.Helper()
	home := t.TempDir()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	registry := writeSrcProjects(t, home, "ko", root)
	service := srcview.NewProjectService(project.NewStore(registry))
	return connectSrcMCP(t, newSrcMCPServer(service))
}

func TestSrcMCPExposesOnlyReadOnlyClosedWorldTools(t *testing.T) {
	session := testSrcSession(t, nil)
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		annotations := tool.Annotations
		if annotations == nil || !annotations.ReadOnlyHint || !annotations.IdempotentHint ||
			annotations.DestructiveHint == nil || *annotations.DestructiveHint ||
			annotations.OpenWorldHint == nil || *annotations.OpenWorldHint {
			t.Fatalf("tool %q annotations = %#v", tool.Name, annotations)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q must have typed schemas", tool.Name)
		}
	}
	sort.Strings(names)
	if want := []string{"read", "symbols"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestSrcMCPSymbolsAndSymbolRead(t *testing.T) {
	source := "package sample\n\n// Greet says hello.\nfunc Greet() string { return \"hello\" }\n"
	session := testSrcSession(t, map[string]string{"sample.go": source})
	outline := callSrcTool(t, session, "symbols", map[string]any{"project": "ko", "path": "sample.go"})
	if outline["project"] != "ko" || outline["path"] != "sample.go" ||
		outline["language"] != "go" || outline["total_bytes"] != float64(len(source)) {
		t.Fatalf("outline metadata = %#v", outline)
	}
	symbols := outline["symbols"].([]any)
	if len(symbols) != 1 {
		t.Fatalf("symbols = %#v", symbols)
	}
	symbol := symbols[0].(map[string]any)
	if symbol["has_doc"] != true || symbol["targetable"] != true {
		t.Fatalf("symbol = %#v", symbol)
	}
	read := callSrcTool(t, session, "read", map[string]any{
		"project": "ko", "path": "sample.go", "symbol_id": symbol["id"],
	})
	if read["content"] != "// Greet says hello.\nfunc Greet() string { return \"hello\" }" || read["truncated"] != false {
		t.Fatalf("read = %#v", read)
	}
}

func TestSrcMCPMarkdownOutlineAndUTF8Range(t *testing.T) {
	source := "# Guide\n\n## Café\n\nText.\n"
	session := testSrcSession(t, map[string]string{"guide.md": source})
	outline := callSrcTool(t, session, "symbols", map[string]any{"project": "ko", "path": "guide.md"})
	if outline["title"] != "Guide" || outline["language"] != "markdown" {
		t.Fatalf("outline = %#v", outline)
	}
	symbols := outline["symbols"].([]any)
	if symbols[0].(map[string]any)["targetable"] != false || symbols[1].(map[string]any)["kind"] != "section" {
		t.Fatalf("symbols = %#v", symbols)
	}
	read := callSrcTool(t, session, "read", map[string]any{
		"project": "ko", "path": "guide.md", "offset": 0, "limit": 18,
	})
	if read["truncated"] != true || read["next_offset"] == nil {
		t.Fatalf("range read = %#v", read)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "read", Arguments: map[string]any{"project": "ko", "path": "guide.md", "offset": 16, "limit": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("mid-rune read = %#v, want tool error", result)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "read", Arguments: map[string]any{"project": "ko", "path": "guide.md", "offset": 15, "limit": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("too-small UTF-8 read = %#v, want tool error", result)
	}
}

func TestSrcMCPRejectsEscapesAndUnsupportedStructure(t *testing.T) {
	session := testSrcSession(t, map[string]string{"notes.txt": "plain text\n"})
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"symbols", map[string]any{"project": "ko", "path": "notes.txt"}},
		{"read", map[string]any{"project": "ko", "path": "../notes.txt"}},
		{"read", map[string]any{"project": "missing", "path": "notes.txt"}},
		{"read", map[string]any{"project": "ko", "path": "notes.txt", "limit": maximumReadLimit + 1}},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("%s(%v) = %#v, want tool error", test.name, test.args, result)
		}
	}
}

func TestSrcMCPCommandHelper(_ *testing.T) {
	if os.Getenv("GO_WANT_SRC_MCP_HELPER") != "1" {
		return
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"mcp"})
	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestSrcMCPCommandTransport(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Project\n\n## Use\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeSrcProjects(t, home, "ko", root)
	command := exec.Command(os.Args[0], "-test.run=TestSrcMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_SRC_MCP_HELPER=1", "HOME="+home)
	client := mcp.NewClient(&mcp.Implementation{Name: "src-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	output := callSrcTool(t, session, "symbols", map[string]any{"project": "ko", "path": "README.md"})
	if output["title"] != "Project" {
		t.Fatalf("output = %#v", output)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
}
