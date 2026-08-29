package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/project"
)

func testProjectMCPServer(t *testing.T) *mcp.Server {
	t.Helper()
	home := t.TempDir()
	writeProjectsConfig(t, home, `
[organon]
name = "Organon"
path = "/work/code/projects/tta-lab/organon"
remote = "https://github.com/tta-lab/organon.git"

[archived.ttal]
name = "TTAL"
path = "/work/code/projects/tta-lab/ttal-cli"
remote = "https://github.com/tta-lab/ttal-cli.git"
`)
	references := filepath.Join(home, "code", "references")
	if err := os.MkdirAll(filepath.Join(references, "github.com", "tta-lab", "reference-only"), 0755); err != nil {
		t.Fatal(err)
	}
	return newProjectMCPServer(project.NewDiscoveryStore(
		filepath.Join(home, ".config", "ttal", "projects.toml"), references,
	))
}

func connectProjectMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "project-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestProjectMCPListsOnlyDiscoveryTools(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %#v, want read-only and idempotent", tool.Name, tool.Annotations)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q openWorldHint = %#v, want false", tool.Name, tool.Annotations.OpenWorldHint)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q must have generated input and output schemas", tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"project_find", "project_get", "project_list"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestProjectMCPReturnsStructuredCatalogData(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	tests := []struct {
		name string
		args map[string]any
		want map[string]any
	}{
		{
			name: "project_list",
			args: map[string]any{},
			want: map[string]any{"projects": []any{
				map[string]any{
					"alias": "organon", "name": "Organon",
					"path":   "/work/code/projects/tta-lab/organon",
					"remote": "https://github.com/tta-lab/organon.git", "archived": false,
				},
			}},
		},
		{
			name: "project_list",
			args: map[string]any{"include_archived": true},
			want: map[string]any{"projects": []any{
				map[string]any{
					"alias": "organon", "name": "Organon",
					"path":   "/work/code/projects/tta-lab/organon",
					"remote": "https://github.com/tta-lab/organon.git", "archived": false,
				},
				map[string]any{
					"alias": "ttal", "name": "TTAL", "path": "/work/code/projects/tta-lab/ttal-cli",
					"remote": "https://github.com/tta-lab/ttal-cli.git", "archived": true,
				},
			}},
		},
		{
			name: "project_get",
			args: map[string]any{"project": "organon"},
			want: map[string]any{"project": map[string]any{
				"alias": "organon", "name": "Organon", "path": "/work/code/projects/tta-lab/organon",
				"remote": "https://github.com/tta-lab/organon.git", "archived": false,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("unexpected tool error: %#v", result.Content)
			}
			gotJSON, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(gotJSON, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("structured content = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestProjectMCPReloadsRegistryOnEveryCall(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "ttal", "projects.toml")
	writeProjectsConfig(t, home, "[one]\npath = \"/projects/one\"\nremote = \"https://example.com/owner/one.git\"\n")
	session := connectProjectMCP(t, newProjectMCPServer(project.NewStore(path)))

	writeProjectsConfig(t, home, "[two]\npath = \"/projects/two\"\nremote = \"https://example.com/owner/two.git\"\n")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "project_get", Arguments: map[string]any{"project": "two"},
	})
	if err != nil {
		t.Fatalf("call project_get: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error after reload: %#v", result.Content)
	}
}

func TestProjectMCPRejectsInvalidOrUnknownAliasesAsToolErrors(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	for _, alias := range []string{"organon.child", "/work/code/projects/tta-lab/organon", "missing"} {
		t.Run(alias, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "project_get", Arguments: map[string]any{"project": alias},
			})
			if err != nil {
				t.Fatalf("protocol error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("result = %#v, want visible tool error", result)
			}
		})
	}
}

func TestProjectMCPCommandHelper(_ *testing.T) {
	if os.Getenv("GO_WANT_PROJECT_MCP_HELPER") != "1" {
		return
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"mcp"})
	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestProjectMCPCommandTransport(t *testing.T) {
	home := t.TempDir()
	writeProjectsConfig(t, home, `[organon]
path = "/work/code/projects/tta-lab/organon"
remote = "https://github.com/tta-lab/organon.git"
`)
	command := exec.Command(os.Args[0], "-test.run=TestProjectMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_PROJECT_MCP_HELPER=1", "HOME="+home)
	client := mcp.NewClient(&mcp.Implementation{Name: "project-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "project_get", Arguments: map[string]any{"project": "organon"},
	})
	if err != nil {
		t.Fatalf("call project_get: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %#v", result.Content)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func projectMCPToolSchema(t *testing.T, session *mcp.ClientSession, name string) map[string]any {
	t.Helper()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != name {
			continue
		}
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func projectMCPStructuredCall(
	t *testing.T, session *mcp.ClientSession, name string, args map[string]any,
) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("%s result = %#v, err = %v", name, result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func TestProjectMCPExposesProjectReferenceSchemas(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	getSchema := projectMCPToolSchema(t, session, "project_get")
	findSchema := projectMCPToolSchema(t, session, "project_find")
	getProperties := getSchema["properties"].(map[string]any)
	if _, exists := getProperties["project"]; !exists {
		t.Fatalf("project_get schema = %#v, want project field", getSchema)
	}
	if _, exists := getProperties["alias"]; exists {
		t.Fatalf("project_get schema retains alias field: %#v", getSchema)
	}
	findProperties := findSchema["properties"].(map[string]any)
	if _, exists := findProperties["query"]; !exists {
		t.Fatalf("project_find schema = %#v, want query field", findSchema)
	}
	if _, exists := findProperties["limit"]; !exists {
		t.Fatalf("project_find schema = %#v, want optional limit field", findSchema)
	}
}

func TestProjectMCPGetsAlternateProjectReferenceCanonically(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	output := projectMCPStructuredCall(t, session, "project_get", map[string]any{"project": "ORGANON"})
	if output["project"].(map[string]any)["alias"] != "organon" {
		t.Fatalf("get output = %#v, want canonical alias", output)
	}
}

func TestProjectMCPFindsActiveProjectsAndReturnsEmptyResult(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	output := projectMCPStructuredCall(t, session, "project_find", map[string]any{"query": "organon"})
	projects := output["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["alias"] != "organon" {
		t.Fatalf("find output = %#v", output)
	}

	output = projectMCPStructuredCall(t, session, "project_find", map[string]any{"query": "reference-only"})
	projects = output["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["reference"] != true {
		t.Fatalf("reference find output = %#v", output)
	}

	output = projectMCPStructuredCall(t, session, "project_find", map[string]any{"query": "unrelated"})
	projects = output["projects"].([]any)
	if projects == nil || len(projects) != 0 {
		t.Fatalf("empty find output = %#v", output)
	}
}

func TestProjectMCPResolutionErrorsExposeSuggestionsAndRejectAliasInput(t *testing.T) {
	session := connectProjectMCP(t, testProjectMCPServer(t))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "project_get", Arguments: map[string]any{"project": "organom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(result.Content)
	message := string(content)
	findHint := strings.Contains(message, "project find") || strings.Contains(message, "project_find")
	listHint := strings.Contains(message, "project list") || strings.Contains(message, "project_list")
	if !result.IsError || !strings.Contains(message, "organon") || !findHint || !listHint {
		t.Fatalf("suggestion error = %s", content)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "project_get", Arguments: map[string]any{"alias": "organon"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("legacy alias input result = %#v, want tool error", result)
	}
}
