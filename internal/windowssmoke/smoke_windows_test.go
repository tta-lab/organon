package windowssmoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//nolint:gocyclo // This end-to-end fixture validates setup and each distinct MCP operation.
func TestWindowsExecutableStdio(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Fatal("smoke requires native Windows x64")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	home := t.TempDir()
	exe := filepath.Join(home, "og.exe")
	build := exec.Command("go", "build", "-o", exe, "./cmd/og")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, output)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	checkout := filepath.Join(home, "checkout 空间")
	if err := os.Mkdir(checkout, 0700); err != nil {
		t.Fatal(err)
	}
	git := exec.Command("git", "init", "-q", checkout)
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	git = exec.Command("git", "-C", checkout, "remote", "add", "origin", "https://github.com/owner/repo.git")
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	sub := filepath.Join(checkout, "中文")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("needle one\r\nneedle two\r\nneedle three\r\n")
	if err := os.WriteFile(filepath.Join(sub, "file.txt"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	junction := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(checkout, "escape"), outside)
	if out, err := junction.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	config := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf("[sample]\npath = %s\nremote = \"https://github.com/owner/repo.git\"\n",
		strconv.Quote(checkout))
	if err := os.WriteFile(filepath.Join(config, "projects.toml"), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "windows-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, exe, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatalf("tools: %v", err)
	}
	call := func(name string, args map[string]any, wantError bool) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError != wantError {
			t.Fatalf("%s: %#v %v", name, result, err)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if result := call("project_get", map[string]any{"project": "sample"}, false); !strings.Contains(result, "sample") {
		t.Fatal(result)
	}
	read := call("source_read", map[string]any{"project": "sample", "path": "中文/file.txt", "limit": 1}, false)
	if !strings.Contains(read, "needle one") || strings.Contains(read, "needle two") {
		t.Fatal(read)
	}
	result := call("source_list", map[string]any{"project": "sample", "path": "中文"}, false)
	if !strings.Contains(result, "file.txt") {
		t.Fatal(result)
	}
	call("source_read", map[string]any{"project": "sample", "path": "../outside.txt"}, true)
	call("source_read", map[string]any{"project": "sample", "path": "escape/secret.txt"}, true)
	call("source_list", map[string]any{"project": "sample", "path": "escape"}, true)
	result = call("source_search", map[string]any{"project": "sample", "pattern": "needle", "limit": 1}, false)
	if !strings.Contains(result, `"truncated":true`) || !strings.Contains(result, "中文/file.txt") {
		t.Fatal(result)
	}
	call("repo_status", map[string]any{"project": "sample"}, false)
}
