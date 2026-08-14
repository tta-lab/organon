package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tta-lab/organon/internal/og"
)

type daemonResponseParityCase struct {
	name     string
	cliArgs  []string
	mcpTool  string
	mcpArgs  map[string]any
	response og.Response
	want     string
}

func TestDaemonResponseValidationParity(t *testing.T) {
	cases := []daemonResponseParityCase{
		{
			name:     "missing auth status",
			cliArgs:  []string{"auth", "status"},
			mcpTool:  "auth_status",
			mcpArgs:  map[string]any{"project": "organon"},
			response: og.Response{OK: true},
			want:     "no authentication status",
		},
		{
			name:     "missing current branch PR",
			cliArgs:  []string{"pr", "view"},
			mcpTool:  "pr_get",
			mcpArgs:  map[string]any{"project": "organon"},
			response: og.Response{OK: true},
			want:     "no pull request",
		},
		{
			name:     "invalid current branch PR",
			cliArgs:  []string{"pr", "view"},
			mcpTool:  "pr_get",
			mcpArgs:  map[string]any{"project": "organon"},
			response: og.Response{OK: true, PR: &og.PullRequest{Index: 0}},
			want:     "invalid PR ID 0",
		},
		{
			name:     "mismatched exact PR",
			cliArgs:  []string{"pr", "get", "7"},
			mcpTool:  "pr_get",
			mcpArgs:  map[string]any{"project": "organon", "pr_id": 7},
			response: og.Response{OK: true, PR: &og.PullRequest{Index: 8}},
			want:     "PR ID 8, want 7",
		},
		{
			name:     "blank message",
			cliArgs:  []string{"push"},
			mcpTool:  "push",
			mcpArgs:  map[string]any{"project": "organon"},
			response: og.Response{OK: true, Message: " \n"},
			want:     "no operation result",
		},
		{
			name:     "lines without PR metadata",
			cliArgs:  []string{"pr", "log"},
			mcpTool:  "pr_log",
			mcpArgs:  map[string]any{"project": "organon"},
			response: og.Response{OK: true, Lines: []string{"CI line"}},
			want:     "no pull request",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			defer server.Close()
			t.Setenv("OG_DAEMON_URL", server.URL)

			for _, jsonOutput := range []bool{false, true} {
				args := append([]string(nil), tc.cliArgs...)
				if jsonOutput {
					args = append(args, "--json")
				}
				stdout, stderr, err := runOGForResponseParity(args...)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("CLI %v error = %v, want %q", args, err, tc.want)
				}
				if stdout != "" || !strings.Contains(stderr, tc.want) {
					t.Fatalf("CLI %v stdout/stderr = %q/%q, want concise error %q only on stderr", args, stdout, stderr, tc.want)
				}
			}

			caller := recordingOGCaller{call: func(context.Context, string, og.Request) (og.Response, error) {
				return tc.response, nil
			}}
			session := connectOGMCP(t, newOGMCPServer(testOGStore(t), caller))
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: tc.mcpTool, Arguments: tc.mcpArgs,
			})
			if err != nil {
				t.Fatalf("MCP protocol error: %v", err)
			}
			content, err := json.Marshal(result.Content)
			if err != nil {
				t.Fatalf("encode MCP content: %v", err)
			}
			if !result.IsError || !strings.Contains(string(content), tc.want) {
				t.Fatalf("MCP content = %s, want error containing %q", content, tc.want)
			}
		})
	}
}

func runOGForResponseParity(args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd(&outBuf, &errBuf)
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}
