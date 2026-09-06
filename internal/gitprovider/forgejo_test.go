package gitprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
