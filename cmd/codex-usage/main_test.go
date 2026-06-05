package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPrintsWeeklyUsageFromLenosConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
		"providers": {
			"codex": {
				"oauth": {"access_token": "oauth-access"},
				"extra_headers": {"ChatGPT-Account-Id": "account-123"}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Fatalf("missing bearer token")
		}
		if r.Header.Get("ChatGPT-Account-Id") != "account-123" {
			t.Fatalf("missing account ID")
		}
		_, _ = w.Write([]byte(`{
			"plan_type": "pro",
			"rate_limit": {
				"secondary_window": {
					"used_percent": 37,
					"reset_at": 1767225600,
					"limit_window_seconds": 604800
				}
			}
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err := run(&stdout, server.Client(), []string{"--config", configPath, "--base-url", server.URL})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"Plan: pro",
		"Weekly usage: 37% used, 63% left",
		"Refresh date: 2026-01-01 00:00:00",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRejectsMissingCodexConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"providers":{}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	err := run(&stdout, &http.Client{Timeout: time.Second}, []string{"--config", configPath})
	if err == nil {
		t.Fatal("run returned nil error, want missing config error")
	}
}
