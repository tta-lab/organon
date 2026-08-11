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

	"github.com/tta-lab/organon/internal/skill"
)

// testSkillServer builds a server whose per-request config loader returns cfg.
func testSkillServer(home string, cfg skill.Config) *mcp.Server {
	return newSkillMCPServer(home, func() (skill.Config, error) { return cfg, nil })
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
	session := connectSkillMCP(t, testSkillServer(home, skill.Config{}))
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

func TestSkillMCPGlobalSkillsDiscoveredAndRead(t *testing.T) {
	home := t.TempDir()
	writeSkillAt(t, home, "my-skill", "a global skill", "tool", "skill body content")
	session := connectSkillMCP(t, testSkillServer(home, skill.Config{}))

	listed := callSkillTool(t, session, "skill_list", map[string]any{})
	got := listed["skills"].([]any)
	if len(got) != 1 {
		t.Fatalf("listed skills = %#v", got)
	}
	summary := got[0].(map[string]any)
	wantSource := filepath.Join(home, ".agents", "skills")
	if summary["name"] != "my-skill" || summary["source"] != wantSource {
		t.Fatalf("skill summary = %#v, want source %q", summary, wantSource)
	}

	detail := callSkillTool(t, session, "skill_get", map[string]any{"name": "my-skill"})
	skillDetail := detail["skill"].(map[string]any)
	if skillDetail["source"] != wantSource || skillDetail["body"] != "skill body content" {
		t.Fatalf("skill detail = %#v", skillDetail)
	}
	if skillDetail["path"] != filepath.Join(wantSource, "my-skill", "SKILL.md") {
		t.Fatalf("skill path = %#v", skillDetail["path"])
	}
}

func TestSkillMCPConfiguredExtras(t *testing.T) {
	home := t.TempDir()
	extraRoot := t.TempDir()
	writeSkillAt(t, home, "default-skill", "from ~/.agents", "tool", "default body")
	writeSkillAt(t, extraRoot, "extra-skill", "from configured global dir", "tool", "extra body")
	// Same name in both dirs: the default must win on the collision.
	writeSkillAt(t, home, "shared", "default version", "tool", "default shared body")
	writeSkillAt(t, extraRoot, "shared", "extra version", "tool", "extra shared body")

	cfg := skill.Config{
		Global: []string{filepath.Join(extraRoot, ".agents", "skills")},
	}
	session := connectSkillMCP(t, testSkillServer(home, cfg))

	listed := callSkillTool(t, session, "skill_list", map[string]any{})
	got := listed["skills"].([]any)
	if len(got) != 3 {
		t.Fatalf("listed skills = %#v", got)
	}
	byName := make(map[string]map[string]any, len(got))
	for _, item := range got {
		entry := item.(map[string]any)
		byName[entry["name"].(string)] = entry
	}
	wantDefault := filepath.Join(home, ".agents", "skills")
	wantExtra := filepath.Join(extraRoot, ".agents", "skills")
	if byName["default-skill"]["source"] != wantDefault {
		t.Fatalf("default skill = %#v, want source %q", byName["default-skill"], wantDefault)
	}
	if byName["extra-skill"]["source"] != wantExtra {
		t.Fatalf("configured extra skill = %#v, want source %q", byName["extra-skill"], wantExtra)
	}
	if byName["shared"]["source"] != wantDefault {
		t.Fatalf("collision skill = %#v, want default source %q", byName["shared"], wantDefault)
	}

	detail := callSkillTool(t, session, "skill_get", map[string]any{"name": "shared"})
	if got := detail["skill"].(map[string]any); got["body"] != "default shared body" {
		t.Fatalf("collision detail = %#v, want default body", got)
	}
}

func TestSkillMCPReloadsConfigPerRequest(t *testing.T) {
	home := t.TempDir()
	extraRoot := t.TempDir()
	writeSkillAt(t, extraRoot, "reloaded-skill", "appears after config edit", "tool", "reloaded body")

	var cfg skill.Config
	session := connectSkillMCP(t, newSkillMCPServer(home, func() (skill.Config, error) {
		return cfg, nil
	}))

	listed := callSkillTool(t, session, "skill_list", map[string]any{})
	if got := listed["skills"].([]any); len(got) != 0 {
		t.Fatalf("before edit: skills = %#v, want none", got)
	}

	cfg = skill.Config{Global: []string{filepath.Join(extraRoot, ".agents", "skills")}}
	listed = callSkillTool(t, session, "skill_list", map[string]any{})
	if got := listed["skills"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "reloaded-skill" {
		t.Fatalf("after edit: skills = %#v", got)
	}
}

func TestSkillMCPFindRanksTokenizedQueryAndAppliesLimit(t *testing.T) {
	home := t.TempDir()
	writeSkillAt(t, home, "pr-review-loop", "Review pull requests in a repeated loop", "workflow", "body")
	writeSkillAt(t, home, "plan-triage", "Triage implementation plans", "workflow", "body")
	writeSkillAt(t, home, "review-notes", "Review collected notes", "workflow", "body")
	session := connectSkillMCP(t, testSkillServer(home, skill.Config{}))

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

func TestSkillMCPAllowsGlobalSymlinkSkills(t *testing.T) {
	home := t.TempDir()
	targetRoot := t.TempDir()
	writeSkillAt(t, targetRoot, "linked", "linked skill", "tool", "linked body")
	target := filepath.Join(targetRoot, ".agents", "skills", "linked")
	base := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "linked")); err != nil {
		t.Fatal(err)
	}
	session := connectSkillMCP(t, testSkillServer(home, skill.Config{}))

	global := callSkillTool(t, session, "skill_get", map[string]any{"name": "linked"})
	if got := global["skill"].(map[string]any); got["source"] != base || got["body"] != "linked body" {
		t.Fatalf("linked skill = %#v", got)
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
