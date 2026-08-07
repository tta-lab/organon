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
	return newProjectMCPServer(project.NewStore(filepath.Join(home, ".config", "ttal", "projects.toml")))
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
	want := []string{"project_get", "project_list"}
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
			args: map[string]any{"alias": "organon"},
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
		Name: "project_get", Arguments: map[string]any{"alias": "two"},
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
				Name: "project_get", Arguments: map[string]any{"alias": alias},
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
		Name: "project_get", Arguments: map[string]any{"alias": "organon"},
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
