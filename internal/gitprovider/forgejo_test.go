package gitprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	forgejo_sdk "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
)

func TestForgejoProviderEditPRSendsEmptyBody(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
			return
		}
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/repos/o/r/pulls/7" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"index":7,"title":"new title","body":"","state":"open"}`))
	}))
	t.Cleanup(server.Close)
	provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.EditPR("o", "r", 7, "new title", ""); err != nil {
		t.Fatalf("EditPR: %v", err)
	}
	if body, ok := got["body"]; !ok || body != "" {
		t.Fatalf("request body = %#v, want explicit empty body", got)
	}
}

func TestForgejoProviderMergePRUsesSquashAndExpectedSHA(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/repos/o/r/pulls/7/merge" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.(*ForgejoProvider).MergePullRequest("o", "r", 7, "head-sha"); err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}
	if got["Do"] != "squash" || got["head_commit_id"] != "head-sha" || got["delete_branch_after_merge"] != false {
		t.Fatalf("merge request = %#v", got)
	}
}

func TestForgejoProviderGetCombinedStatusNotConfiguredWhenStatusesEmptyOrNil(t *testing.T) {
	for _, body := range []string{
		`{"state":"success","total_count":0,"statuses":[]}`,
		`{"state":"failure","total_count":0,"statuses":null}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
					_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/o/r/commits/abc123/status" {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)
			provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
			if err != nil {
				t.Fatalf("NewForgejoProviderWithToken: %v", err)
			}
			status, err := provider.GetCombinedStatus("o", "r", "abc123")
			if err != nil || status == nil || status.State != StateNotConfigured ||
				status.Statuses == nil || len(status.Statuses) != 0 {
				t.Fatalf("status = %+v, err = %v, want not_configured with empty statuses", status, err)
			}
		})
	}
}

func TestForgejoProviderGetCombinedStatusPreservesUnknownNonemptyState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/o/r/commits/abc123/status" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"state":"mystery","total_count":1,"statuses":[{"status":"mystery","context":"unknown"}]}`))
	}))
	t.Cleanup(server.Close)
	provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatalf("NewForgejoProviderWithToken: %v", err)
	}
	status, err := provider.GetCombinedStatus("o", "r", "abc123")
	if err != nil || status == nil || status.State != "mystery" || len(status.Statuses) != 1 {
		t.Fatalf("status = %+v, err = %v, want preserved unknown state", status, err)
	}
}

func TestNormalizeForgejoCombinedStatusRejectsMalformedCountsAndNilEntries(t *testing.T) {
	validStatus := &forgejo_sdk.Status{State: forgejo_sdk.StatusSuccess, Context: "check"}
	for _, tt := range []struct {
		name string
		cs   *forgejo_sdk.CombinedStatus
	}{
		{name: "nil response"},
		{name: "negative total", cs: &forgejo_sdk.CombinedStatus{TotalCount: -1}},
		{name: "positive total with no statuses", cs: &forgejo_sdk.CombinedStatus{TotalCount: 1}},
		{
			name: "blank aggregate state",
			cs: &forgejo_sdk.CombinedStatus{
				TotalCount: 1,
				Statuses:   []*forgejo_sdk.Status{validStatus},
			},
		},
		{
			name: "zero total with statuses",
			cs:   &forgejo_sdk.CombinedStatus{TotalCount: 0, Statuses: []*forgejo_sdk.Status{validStatus}},
		},
		{
			name: "contradictory positive total",
			cs:   &forgejo_sdk.CombinedStatus{TotalCount: 2, Statuses: []*forgejo_sdk.Status{validStatus}},
		},
		{name: "nil status entry", cs: &forgejo_sdk.CombinedStatus{TotalCount: 1, Statuses: []*forgejo_sdk.Status{nil}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status, err := normalizeForgejoCombinedStatus(tt.cs)
			if err == nil || status != nil {
				t.Fatalf("status = %+v, err = %v, want fail-closed error", status, err)
			}
		})
	}
}

func TestNormalizeForgejoCombinedStatusAcceptsZeroValueAsNotConfigured(t *testing.T) {
	status, err := normalizeForgejoCombinedStatus(&forgejo_sdk.CombinedStatus{})
	if err != nil || status == nil || status.State != StateNotConfigured ||
		status.Statuses == nil || len(status.Statuses) != 0 {
		t.Fatalf("status = %+v, err = %v, want not_configured with empty statuses", status, err)
	}
}

func TestForgejoProviderGetCombinedStatusReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
			return
		}
		http.Error(w, `{"message":"temporary"}`, http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
	if err != nil {
		t.Fatalf("NewForgejoProviderWithToken: %v", err)
	}
	status, err := provider.GetCombinedStatus("o", "r", "abc123")
	if err == nil || status != nil {
		t.Fatalf("status = %+v, err = %v, want API error", status, err)
	}
}

func TestForgejoProviderGetCombinedStatusHandlesNullAndEmptyObject(t *testing.T) {
	for _, tt := range []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "null", body: `null`, wantError: true},
		{name: "empty object", body: `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/version" {
					_, _ = w.Write([]byte(`{"version":"9.0.0"}`))
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/o/r/commits/abc123/status" {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			provider, err := NewForgejoProviderWithToken(context.Background(), server.URL, "token")
			if err != nil {
				t.Fatalf("NewForgejoProviderWithToken: %v", err)
			}
			status, err := provider.GetCombinedStatus("o", "r", "abc123")
			if tt.wantError {
				if err == nil || status != nil {
					t.Fatalf("status = %+v, err = %v, want malformed response error", status, err)
				}
				return
			}
			if err != nil || status == nil || status.State != StateNotConfigured ||
				status.Statuses == nil || len(status.Statuses) != 0 {
				t.Fatalf("status = %+v, err = %v, want not_configured with empty statuses", status, err)
			}
		})
	}
}

func TestNewForgejoProvider_EmptyHost(t *testing.T) {
	_, err := NewForgejoProvider(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty host")
	}
}

func TestNewForgejoProvider_MissingToken(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("FORGEJO_ACCESS_TOKEN", "")

	_, err := NewForgejoProvider(context.Background(), "git.example.com")
	if err == nil {
		t.Error("expected error for missing token")
	}
}
