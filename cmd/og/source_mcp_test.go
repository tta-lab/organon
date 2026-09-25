package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

func sourceSession(t *testing.T) (*mcp.ClientSession, string) {
	t.Helper()
	root := t.TempDir()
	registry := filepath.Join(t.TempDir(), "projects.toml")
	encoded, _ := json.Marshal(root)
	config := "[ko]\npath = " + string(encoded) + "\nremote = \"https://github.com/example/source-repo.git\"\n"
	if err := os.WriteFile(registry, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return connectDirectMCP(t, og.Executor(&directExecutor{}), project.NewStore(registry)), root
}
func sourceCall(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	result, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		return nil, true
	}
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out, false
}

//nolint:gocyclo // One MCP session checks the selector and file security boundary.
func TestSourceMCPToolsAndProjectBoundary(t *testing.T) {
	s, root := sourceSession(t)
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("# Guide\n\n## Use\nText\n"), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range list.Tools {
		if !strings.HasPrefix(tool.Name, "source_") {
			continue
		}
		found++
		a := tool.Annotations
		if a == nil || !a.ReadOnlyHint || !a.IdempotentHint ||
			a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Fatalf("%s annotations: %+v", tool.Name, a)
		}
	}
	if found != 4 {
		t.Fatalf("found %d source tools", found)
	}
	for _, selector := range []string{"ko", filepath.Base(root), "source-repo", root} {
		listing, bad := sourceCall(t, s, "source_list", map[string]any{"project": selector})
		if bad || listing["project"] != "ko" || listing["path"] != "." ||
			len(listing["entries"].([]any)) != 1 {
			t.Fatalf("list selector %q: %v, %v", selector, listing, bad)
		}
		out, bad := sourceCall(t, s, "source_symbols", map[string]any{"project": selector, "path": "readme.md"})
		if bad || out["project"] != "ko" || out["title"] != "Guide" {
			t.Fatalf("selector %q: %v, %v", selector, out, bad)
		}
	}
	for _, selector := range []string{t.TempDir(), filepath.Join(root, "child"), filepath.Join(root, "readme.md"),
		" " + root, root + " "} {
		_, bad := sourceCall(t, s, "source_symbols", map[string]any{"project": selector, "path": "readme.md"})
		if !bad {
			t.Fatalf("accepted project %q", selector)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("# Secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	matches, bad := sourceCall(t, s, "source_search", map[string]any{
		"project": "ko", "pattern": "Secret",
	})
	if bad || len(matches["matches"].([]any)) != 0 {
		t.Fatalf("search followed escaping symlink: %v %v", matches, bad)
	}
	for _, path := range []string{"../secret.md", outside, "escape.md"} {
		_, bad := sourceCall(t, s, "source_list", map[string]any{"project": "ko", "path": path})
		if !bad {
			t.Fatalf("accepted list path %q", path)
		}
		_, bad = sourceCall(t, s, "source_read", map[string]any{"project": "ko", "path": path})
		if !bad {
			t.Fatalf("accepted path %q", path)
		}
	}
}
func TestSourceMCPReadAndSearch(t *testing.T) {
	s, root := sourceSession(t)
	source := "package sample\n\n// Greet says hello.\nfunc Greet() string { return \"needle\" }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	outline, bad := sourceCall(t, s, "source_symbols", map[string]any{"project": "ko", "path": "sample.go"})
	if bad || outline["language"] != "go" {
		t.Fatalf("outline: %v %v", outline, bad)
	}
	id := outline["symbols"].([]any)[0].(map[string]any)["id"]
	read, bad := sourceCall(t, s, "source_read", map[string]any{
		"project": "ko", "path": "sample.go", "symbol_id": id, "limit": 1,
	})
	if bad || read["next_offset"] != float64(2) {
		t.Fatalf("read: %v %v", read, bad)
	}
	read, bad = sourceCall(t, s, "source_read", map[string]any{
		"project": "ko", "path": "sample.go", "symbol_id": id, "offset": 2,
	})
	if bad || !strings.Contains(read["content"].(string), "needle") {
		t.Fatalf("continuation: %v %v", read, bad)
	}
	matches, bad := sourceCall(t, s, "source_search", map[string]any{"project": "ko", "pattern": "needle", "limit": 1})
	if bad || len(matches["matches"].([]any)) != 1 ||
		matches["matches"].([]any)[0].(map[string]any)["path"] != "sample.go" {
		t.Fatalf("matches: %v %v", matches, bad)
	}
	empty, bad := sourceCall(t, s, "source_search", map[string]any{"project": "ko", "pattern": "absent-token"})
	if bad || len(empty["matches"].([]any)) != 0 {
		t.Fatalf("empty: %v %v", empty, bad)
	}
}

func TestSourceMCPRejectsBinaryAndInvalidUTF8(t *testing.T) {
	s, root := sourceSession(t)
	for name, data := range map[string][]byte{
		"invalid.go": {0xff, 'x'}, "binary.go": []byte("%PDF-valid UTF-8"),
		"nul.go": {'a', 0, 'b'}, "image.go": []byte("GIF89a-image"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		for _, tool := range []string{"source_read", "source_symbols"} {
			_, bad := sourceCall(t, s, tool, map[string]any{"project": "ko", "path": name})
			if !bad {
				t.Fatalf("%s accepted %s", tool, name)
			}
		}
	}
}
func TestSourceMCPSearchLimitAndFailure(t *testing.T) {
	s, root := sourceSession(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("needle\nneedle\nneedle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, bad := sourceCall(t, s, "source_search", map[string]any{"project": "ko", "pattern": "needle", "limit": 2})
	if bad || result["truncated"] != true || len(result["matches"].([]any)) != 2 {
		t.Fatalf("limited search: %v %v", result, bad)
	}
	matches := result["matches"].([]any)
	if matches[0].(map[string]any)["line"] != float64(1) || matches[1].(map[string]any)["line"] != float64(2) {
		t.Fatalf("nondeterministic order: %v", matches)
	}
	config := filepath.Join(t.TempDir(), "rg.conf")
	if err := os.WriteFile(config, []byte("--glob=!a.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIPGREP_CONFIG_PATH", config)
	unaffected, bad := sourceCall(t, s, "source_search", map[string]any{"project": "ko", "pattern": "needle"})
	if bad || len(unaffected["matches"].([]any)) != 3 {
		t.Fatalf("rg config affected search: %v %v", unaffected, bad)
	}
	t.Setenv("PATH", t.TempDir())
	_, bad = sourceCall(t, s, "source_search", map[string]any{"project": "ko", "pattern": "needle"})
	if !bad {
		t.Fatal("missing rg executable should fail")
	}
}

func TestSourceMCPMarkdownReadAndInvalidRanges(t *testing.T) {
	s, root := sourceSession(t)
	content := []byte("# Guide\n\n## Setup\n\nFirst\nSecond\n")
	if err := os.WriteFile(filepath.Join(root, "guide.md"), content, 0600); err != nil {
		t.Fatal(err)
	}
	outline, bad := sourceCall(t, s, "source_symbols", map[string]any{"project": "ko", "path": "guide.md"})
	if bad || outline["language"] != "markdown" || outline["title"] != "Guide" {
		t.Fatalf("outline: %v %v", outline, bad)
	}
	id := outline["symbols"].([]any)[1].(map[string]any)["id"]
	read, bad := sourceCall(t, s, "source_read", map[string]any{"project": "ko", "path": "guide.md", "symbol_id": id})
	if bad || !strings.Contains(read["content"].(string), "First") {
		t.Fatalf("read: %v %v", read, bad)
	}
	zero, bad := sourceCall(t, s, "source_read", map[string]any{
		"project": "ko", "path": "guide.md", "limit": 0,
	})
	if bad || zero["content"] != "" || zero["next_offset"] != float64(1) {
		t.Fatalf("explicit zero limit: %v %v", zero, bad)
	}
	for _, args := range []map[string]any{
		{"project": "ko", "path": "guide.md", "offset": 0},
		{"project": "ko", "path": "guide.md", "offset": 100},
		{"project": "ko", "path": "guide.md", "limit": -1},
		{"project": "ko", "path": "guide.md", "symbol_id": "missing"},
	} {
		_, bad = sourceCall(t, s, "source_read", args)
		if !bad {
			t.Fatalf("accepted invalid read: %v", args)
		}
	}
}

//nolint:gocyclo // One generated-schema matrix verifies every source tool contract.
func TestSourceMCPGeneratedSchemas(t *testing.T) {
	session, _ := sourceSession(t)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range list.Tools {
		byName[tool.Name] = tool
	}
	cases := []struct {
		name     string
		required []string
		optional []string
	}{
		{"source_list", []string{"project"}, []string{"path"}},
		{"source_symbols", []string{"project", "path"}, nil},
		{"source_read", []string{"project", "path"}, []string{"symbol_id", "offset", "limit"}},
		{"source_search", []string{"project", "pattern"}, []string{"limit"}},
	}
	for _, tc := range cases {
		tool := byName[tc.name]
		if tool == nil {
			t.Fatalf("missing %s", tc.name)
		}
		a := tool.Annotations
		if a == nil || !a.ReadOnlyHint || !a.IdempotentHint || a.DestructiveHint == nil || *a.DestructiveHint ||
			a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Fatalf("%s annotations: %+v", tc.name, a)
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s schema: %#v", tc.name, tool.InputSchema)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s properties: %#v", tc.name, schema)
		}
		if len(props) != len(tc.required)+len(tc.optional) {
			t.Fatalf("%s properties: %#v", tc.name, props)
		}
		required := map[string]bool{}
		for _, value := range schema["required"].([]any) {
			required[value.(string)] = true
		}
		if len(required) != len(tc.required) {
			t.Fatalf("%s required: %#v", tc.name, required)
		}
		for _, name := range tc.required {
			if !required[name] || props[name] == nil {
				t.Fatalf("%s missing required %s", tc.name, name)
			}
			field := props[name].(map[string]any)
			if !schemaTypeIncludes(field["type"], "string") || field["minLength"] != float64(1) {
				t.Fatalf("%s.%s string bound: %#v", tc.name, name, field)
			}
		}
		for _, name := range tc.optional {
			if required[name] || props[name] == nil {
				t.Fatalf("%s optional %s: %#v", tc.name, name, props)
			}
			wantType := "integer"
			if name == "symbol_id" || name == "path" {
				wantType = "string"
			}
			if !schemaTypeIncludes(props[name].(map[string]any)["type"], wantType) {
				t.Fatalf("%s.%s type: %#v", tc.name, name, props[name])
			}
		}
		if props["query"] != nil || props["args"] != nil || props["flags"] != nil || props["options"] != nil {
			t.Fatalf("%s exposes raw rg arguments: %#v", tc.name, props)
		}
		if tc.name == "source_search" {
			pattern := props["pattern"].(map[string]any)
			limit := props["limit"].(map[string]any)
			if pattern["minLength"] != float64(1) || pattern["maxLength"] != float64(4096) ||
				limit["minimum"] != float64(1) || limit["maximum"] != float64(200) || limit["default"] != float64(50) {
				t.Fatalf("search bounds: %#v %#v", pattern, limit)
			}
		}
		if tc.name == "source_read" {
			offset := props["offset"].(map[string]any)
			limit := props["limit"].(map[string]any)
			if offset["minimum"] != float64(1) || offset["default"] != float64(1) || limit["minimum"] != float64(0) {
				t.Fatalf("read bounds: %#v %#v", offset, limit)
			}
		}
	}
}

func TestSourceMCPSearchRegexAndOptionBoundary(t *testing.T) {
	session, root := sourceSession(t)
	content := "foo\nbar\nfunc Resolve() {}\nTODO\nobj.Resolve(\n-needle\n--glob\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"foo|bar", `func\s+\w+`, "TODO|FIXME", `\.Resolve\(`, "-needle", "--glob"} {
		result, bad := sourceCall(t, session, "source_search", map[string]any{"project": "ko", "pattern": pattern})
		if bad || len(result["matches"].([]any)) == 0 {
			t.Fatalf("pattern %q: %v %v", pattern, result, bad)
		}
	}
	for _, args := range []map[string]any{
		{"project": "ko", "pattern": "foo", "limit": 0},
		{"project": "ko", "pattern": "foo", "limit": 201},
		{"project": "ko", "query": "foo"},
	} {
		_, bad := sourceCall(t, session, "source_search", args)
		if !bad {
			t.Fatalf("accepted invalid search input: %#v", args)
		}
	}
	for _, pattern := range []string{"[", " ", strings.Repeat("a", 4097)} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "source_search", Arguments: map[string]any{"project": "ko", "pattern": pattern},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("accepted invalid pattern %q: %#v", pattern, result)
		}
		if pattern == "[" {
			detail, ok := result.Content[0].(*mcp.TextContent)
			if !ok || !strings.Contains(detail.Text, "regex") {
				t.Fatalf("invalid regex error lacks detail: %#v", result.Content)
			}
		}
	}
}
