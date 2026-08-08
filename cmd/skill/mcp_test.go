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

func writeSkillProjects(t *testing.T, home, content string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "ttal", "projects.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func connectSkillMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "skill-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callSkillTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
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

func TestSkillMCPExposesOnlyReadOnlyClosedWorldTools(t *testing.T) {
	home := t.TempDir()
	registry := writeSkillProjects(t, home, "")
	session := connectSkillMCP(t, newSkillMCPServer(home, project.NewStore(registry)))
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
	if want := []string{"skill_find", "skill_get", "skill_list"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestSkillMCPProjectSkillsShadowGlobalSkills(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	writeSkillAt(t, home, "shared", "global description", "global-category", "global body")
	writeSkillAt(t, root, "shared", "project description", "project-category", "project body")
	writeSkillAt(t, home, "global-only", "global utility", "tool", "global utility body")
	registry := writeSkillProjects(t, home,
		"[ko]\npath = "+strconvQuote(root)+"\nremote = \"https://example.com/tta/ko.git\"\n")
	session := connectSkillMCP(t, newSkillMCPServer(home, project.NewStore(registry)))

	global := callSkillTool(t, session, "skill_get", map[string]any{"name": "shared"})
	if got := global["skill"].(map[string]any); got["scope"] != "global" ||
		got["source"] != "global:.agents" || got["body"] != "global body" {
		t.Fatalf("global skill = %#v", got)
	}
	local := callSkillTool(t, session, "skill_get", map[string]any{"project": "ko", "name": "shared"})
	if got := local["skill"].(map[string]any); got["scope"] != "project" ||
		got["source"] != "project:.agents" || got["body"] != "project body" {
		t.Fatalf("project skill = %#v", got)
	}
	listed := callSkillTool(t, session, "skill_list", map[string]any{"project": "ko"})
	if got := listed["skills"].([]any); len(got) != 2 {
		t.Fatalf("listed skills = %#v", got)
	}
}

func TestSkillMCPFindRanksTokenizedQueryAndAppliesLimit(t *testing.T) {
	home := t.TempDir()
	writeSkillAt(t, home, "pr-review-loop", "Review pull requests in a repeated loop", "workflow", "body")
	writeSkillAt(t, home, "plan-triage", "Triage implementation plans", "workflow", "body")
	writeSkillAt(t, home, "review-notes", "Review collected notes", "workflow", "body")
	registry := writeSkillProjects(t, home, "")
	session := connectSkillMCP(t, newSkillMCPServer(home, project.NewStore(registry)))

	output := callSkillTool(t, session, "skill_find", map[string]any{
		"query": "review loop triage", "limit": 2,
	})
	got := output["skills"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["name"] != "pr-review-loop" ||
		got[1].(map[string]any)["name"] != "plan-triage" {
		t.Fatalf("find result = %#v", got)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "skill_find", Arguments: map[string]any{"query": " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("blank query result = %#v, want tool error", result)
	}
}

func TestSkillMCPRejectsUnknownProject(t *testing.T) {
	home := t.TempDir()
	registry := writeSkillProjects(t, home, "")
	session := connectSkillMCP(t, newSkillMCPServer(home, project.NewStore(registry)))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "skill_list", Arguments: map[string]any{"project": "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result = %#v, want tool error", result)
	}
}

func TestSkillMCPAllowsGlobalLinksButRejectsProjectLinkEscapes(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	targetRoot := t.TempDir()
	writeSkillAt(t, targetRoot, "linked", "linked skill", "tool", "linked body")
	target := filepath.Join(targetRoot, ".agents", "skills", "linked")
	globalBase := filepath.Join(home, ".agents", "skills")
	projectBase := filepath.Join(root, ".agents", "skills")
	for _, base := range []string{globalBase, projectBase} {
		if err := os.MkdirAll(base, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(base, "linked")); err != nil {
			t.Fatal(err)
		}
	}
	registry := writeSkillProjects(t, home,
		"[ko]\npath = "+strconvQuote(root)+"\nremote = \"https://example.com/tta/ko.git\"\n")
	session := connectSkillMCP(t, newSkillMCPServer(home, project.NewStore(registry)))

	global := callSkillTool(t, session, "skill_get", map[string]any{"name": "linked"})
	if got := global["skill"].(map[string]any); got["source"] != "global:.agents" {
		t.Fatalf("global linked skill = %#v", got)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "skill_get", Arguments: map[string]any{"project": "ko", "name": "linked"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("project escape result = %#v, want tool error", result)
	}
}

func TestSkillMCPCommandHelper(_ *testing.T) {
	if os.Getenv("GO_WANT_SKILL_MCP_HELPER") != "1" {
		return
	}
	cmd := newRootCmd(os.Stdout, os.Stderr, nil)
	cmd.SetArgs([]string{"mcp"})
	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestSkillMCPCommandTransport(t *testing.T) {
	home := t.TempDir()
	writeSkillAt(t, home, "global", "global skill", "tool", "body")
	writeSkillProjects(t, home, "")
	command := exec.Command(os.Args[0], "-test.run=TestSkillMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_SKILL_MCP_HELPER=1", "HOME="+home)
	client := mcp.NewClient(&mcp.Implementation{Name: "skill-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	output := callSkillTool(t, session, "skill_list", map[string]any{})
	if got := output["skills"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "global" {
		t.Fatalf("skills = %#v", got)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
