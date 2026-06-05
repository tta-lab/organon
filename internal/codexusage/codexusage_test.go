package codexusage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigReadsCodexOAuthFromLenosConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{
		"providers": {
			"codex": {
				"api_key": "api-key-token",
				"oauth": {
					"access_token": "oauth-access",
					"refresh_token": "oauth-refresh",
					"expires_in": 3600,
					"expires_at": "2099-01-02T03:04:05Z"
				},
				"extra_headers": {
					"ChatGPT-Account-Id": "account-123"
				}
			}
		}
	}`)

	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if config.AccessToken != "oauth-access" {
		t.Fatalf("access token = %q, want oauth-access", config.AccessToken)
	}
	if config.RefreshToken != "oauth-refresh" {
		t.Fatalf("refresh token = %q, want oauth-refresh", config.RefreshToken)
	}
	if config.AccountID != "account-123" {
		t.Fatalf("account ID = %q, want account-123", config.AccountID)
	}
}

func TestFetchWeeklyUsageSendsCodexHeadersAndMapsSecondaryWindow(t *testing.T) {
	var gotAuth string
	var gotAccount string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccount = r.Header.Get("ChatGPT-Account-Id")
		if r.URL.Path != "/api/codex/usage" {
			t.Fatalf("path = %q, want /api/codex/usage", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
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

	client := &http.Client{Timeout: time.Second}
	usage, err := FetchWeeklyUsage(context.Background(), client, server.URL, CodexConfig{
		AccessToken: "oauth-access",
		AccountID:   "account-123",
	})
	if err != nil {
		t.Fatalf("FetchWeeklyUsage returned error: %v", err)
	}

	if gotAuth != "Bearer oauth-access" {
		t.Fatalf("authorization header = %q, want bearer token", gotAuth)
	}
	if gotAccount != "account-123" {
		t.Fatalf("account header = %q, want account-123", gotAccount)
	}
	if usage.Plan != "pro" {
		t.Fatalf("plan = %q, want pro", usage.Plan)
	}
	if usage.UsedPercent != 37 {
		t.Fatalf("used percent = %d, want 37", usage.UsedPercent)
	}
	if usage.RemainingPercent != 63 {
		t.Fatalf("remaining percent = %d, want 63", usage.RemainingPercent)
	}
	if usage.RefreshAt.UTC().Format(time.RFC3339) != "2026-01-01T00:00:00Z" {
		t.Fatalf("refresh at = %s, want 2026-01-01T00:00:00Z", usage.RefreshAt.UTC().Format(time.RFC3339))
	}
}

func TestFetchWeeklyUsageUsesBackendWhamPathWhenBaseContainsBackendAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("path = %q, want /backend-api/wham/usage", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"rate_limit": {
				"secondary_window": {
					"used_percent": 0,
					"reset_at": 1767225600,
					"limit_window_seconds": 604800
				}
			}
		}`))
	}))
	defer server.Close()

	_, err := FetchWeeklyUsage(
		context.Background(),
		server.Client(),
		server.URL+"/backend-api",
		CodexConfig{AccessToken: "token"},
	)
	if err != nil {
		t.Fatalf("FetchWeeklyUsage returned error: %v", err)
	}
}

func TestFormatWeeklyUsageIncludesUsageAndRefreshDate(t *testing.T) {
	location := time.FixedZone("LOCAL", 8*60*60)
	refresh := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 12, 26, 11, 0, 0, 0, time.UTC)
	text := FormatWeeklyUsageAt(WeeklyUsage{
		Plan:             "pro",
		UsedPercent:      37,
		RemainingPercent: 63,
		RefreshAt:        refresh,
	}, now, location)

	for _, want := range []string{
		"Plan: pro",
		"Weekly usage: 37% used, 63% left",
		"Refresh date: 2026-01-01 08:00:00 (5d 13h)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted output missing %q:\n%s", want, text)
		}
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
