package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/search"
	webcore "github.com/tta-lab/organon/internal/web"
)

type recordingWebService struct {
	search      func(context.Context, string) (webcore.SearchResult, error)
	fetch       func(context.Context, webcore.FetchInput) (webcore.FetchResult, error)
	docsResolve func(context.Context, string) (webcore.DocsResolveResult, error)
	docsFetch   func(context.Context, webcore.DocsFetchInput) (webcore.DocsFetchResult, error)
	sgraph      func(context.Context, webcore.SGraphInput) (webcore.SGraphResult, error)
}

func (s recordingWebService) Search(ctx context.Context, query string) (webcore.SearchResult, error) {
	return s.search(ctx, query)
}

func (s recordingWebService) Fetch(ctx context.Context, input webcore.FetchInput) (webcore.FetchResult, error) {
	return s.fetch(ctx, input)
}

func (s recordingWebService) DocsResolve(ctx context.Context, query string) (webcore.DocsResolveResult, error) {
	return s.docsResolve(ctx, query)
}

func (s recordingWebService) DocsFetch(
	ctx context.Context,
	input webcore.DocsFetchInput,
) (webcore.DocsFetchResult, error) {
	return s.docsFetch(ctx, input)
}

func (s recordingWebService) SGraphSearch(
	ctx context.Context,
	input webcore.SGraphInput,
) (webcore.SGraphResult, error) {
	return s.sgraph(ctx, input)
}

func testWebService(t *testing.T) recordingWebService {
	t.Helper()
	return recordingWebService{
		search: func(_ context.Context, query string) (webcore.SearchResult, error) {
			if query != "typed mcp" {
				t.Fatalf("search query = %q", query)
			}
			return search.Response{Provider: "Brave", Results: []search.SearchResult{{
				Title: "MCP", Link: "https://example.com/mcp", Snippet: "Typed tools", Position: 1,
			}}}, nil
		},
		fetch: func(_ context.Context, input webcore.FetchInput) (webcore.FetchResult, error) {
			if input.URL != "https://example.com/page" || !input.ShowTree || input.TreeThreshold != 5000 {
				t.Fatalf("fetch input = %+v", input)
			}
			return webcore.FetchResult{URL: input.URL, Mode: "tree", Content: "# Page"}, nil
		},
		docsResolve: func(_ context.Context, query string) (webcore.DocsResolveResult, error) {
			if query != "effect" {
				t.Fatalf("docs query = %q", query)
			}
			return webcore.DocsResolveResult{Query: query, Libraries: []docs.Library{{
				ID: "/effect-ts/effect", Title: "Effect", TrustScore: 9.8, TotalSnippets: 42,
			}}}, nil
		},
		docsFetch: func(_ context.Context, input webcore.DocsFetchInput) (webcore.DocsFetchResult, error) {
			if input.LibraryID != "effect-ts/effect" || input.Topic != "schema" || input.Tokens != 1200 {
				t.Fatalf("docs fetch input = %+v", input)
			}
			return webcore.DocsFetchResult{
				LibraryID: "/effect-ts/effect", Topic: input.Topic, Content: "Effect docs",
			}, nil
		},
		sgraph: func(_ context.Context, input webcore.SGraphInput) (webcore.SGraphResult, error) {
			if input.Query != "repo:tta-lab" || input.Count != 10 || input.ContextWindow != 10 || input.Timeout != 0 {
				t.Fatalf("sgraph input = %+v", input)
			}
			return webcore.SGraphResult{Content: "# Sourcegraph results"}, nil
		},
	}
}

func connectWebMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "web-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestWebMCPListsOnlyTypedOpenWorldTools(t *testing.T) {
	session := connectWebMCP(t, newWebMCPServer(testWebService(t)))
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q must have generated schemas", tool.Name)
		}
		annotations := tool.Annotations
		if annotations == nil || !annotations.ReadOnlyHint || !annotations.IdempotentHint ||
			annotations.OpenWorldHint == nil || !*annotations.OpenWorldHint {
			t.Fatalf("tool %q annotations = %#v", tool.Name, annotations)
		}
	}
	sort.Strings(names)
	want := []string{"docs_fetch", "docs_resolve", "fetch", "search", "sgraph_search"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
	assertWebMCPDefaults(t, result.Tools)
}

func assertWebMCPDefaults(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	byName := make(map[string]*mcp.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	wants := map[string]map[string]string{
		"fetch":         {"tree_threshold": "5000"},
		"docs_fetch":    {"tokens": "0"},
		"sgraph_search": {"count": "10", "context": "10", "timeout": "0"},
	}
	for toolName, fields := range wants {
		properties := webToolProperties(t, byName[toolName])
		for field, want := range fields {
			got := properties[field]
			if got == nil || string(got.Default) != want {
				t.Fatalf("tool %q field %q default = %#v, want %s", toolName, field, got, want)
			}
		}
	}
}

func webToolProperties(t *testing.T, tool *mcp.Tool) map[string]*jsonschema.Schema {
	t.Helper()
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Properties
}

func TestWebMCPMapsInputsAndReturnsStructuredResults(t *testing.T) {
	session := connectWebMCP(t, newWebMCPServer(testWebService(t)))
	tests := []struct {
		name string
		args map[string]any
		key  string
		want any
	}{
		{name: "search", args: map[string]any{"query": "typed mcp"}, key: "provider", want: "Brave"},
		{name: "fetch", args: map[string]any{"url": "https://example.com/page", "tree": true}, key: "mode", want: "tree"},
		{name: "docs_resolve", args: map[string]any{"query": "effect"}, key: "query", want: "effect"},
		{name: "docs_fetch", args: map[string]any{
			"library_id": "effect-ts/effect", "topic": "schema", "tokens": 1200,
		}, key: "content", want: "Effect docs"},
		{name: "sgraph_search", args: map[string]any{"query": "repo:tta-lab"}, key: "content", want: "# Sourcegraph results"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
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
			if !reflect.DeepEqual(got[tt.key], tt.want) {
				t.Fatalf("structured content = %#v, want %s=%#v", got, tt.key, tt.want)
			}
		})
	}
}

func TestWebMCPReturnsServiceFailuresAsToolErrors(t *testing.T) {
	service := testWebService(t)
	service.search = func(context.Context, string) (webcore.SearchResult, error) {
		return webcore.SearchResult{}, errors.New("search offline")
	}
	session := connectWebMCP(t, newWebMCPServer(service))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search", Arguments: map[string]any{"query": "typed mcp"},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("result = %#v, want visible tool error", result)
	}
}

func TestWebMCPPassesCancellationToService(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	service := testWebService(t)
	service.search = func(ctx context.Context, _ string) (webcore.SearchResult, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return webcore.SearchResult{}, ctx.Err()
	}
	session := connectWebMCP(t, newWebMCPServer(service))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": "wait"}})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("service call did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("service context was not canceled")
	}
	<-done
}

func TestWebMCPCommandPassesProvider(t *testing.T) {
	var gotProvider string
	wantErr := errors.New("stop before serving")
	cmd := newWebMCPCmdWithFactory(func(provider string) (webService, error) {
		gotProvider = provider
		return nil, wantErr
	})
	cmd.SetArgs([]string{"--provider", "brave"})

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if gotProvider != "brave" {
		t.Fatalf("provider = %q, want brave", gotProvider)
	}
}

func TestWebMCPCommandLeavesProviderEmptyForFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "web.toml"), []byte("[search\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotProvider string
	wantErr := errors.New("stop before serving")
	cmd := newWebMCPCmdWithFactory(func(provider string) (webService, error) {
		gotProvider = provider
		return nil, wantErr
	})

	if err := cmd.Execute(); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if gotProvider != "" {
		t.Fatalf("provider = %q, want empty fallback selection", gotProvider)
	}
}

func TestWebMCPCommandHelper(_ *testing.T) {
	if os.Getenv("GO_WANT_WEB_MCP_HELPER") != "1" {
		return
	}
	cmd := newWebMCPCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestWebMCPCommandTransport(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/extract" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"title":"stdio","content":"typed MCP"}`))
	}))
	defer gateway.Close()
	command := exec.Command(os.Args[0], "-test.run=TestWebMCPCommandHelper")
	command.Env = append(os.Environ(), "GO_WANT_WEB_MCP_HELPER=1", "BROWSER_GATEWAY_URL="+gateway.URL)
	client := mcp.NewClient(&mcp.Implementation{Name: "web-command-test", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect command transport: %v", err)
	}
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatalf("tools = %#v, err = %v", tools, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "fetch", Arguments: map[string]any{"url": "https://example.com/stdio", "full": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !reflect.DeepEqual(result.StructuredContent.(map[string]any)["content"], "# stdio\n\ntyped MCP") {
		t.Fatalf("result = %#v", result)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
}
