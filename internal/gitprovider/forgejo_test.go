package gitprovider

import (
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
	provider, err := NewForgejoProviderWithToken(server.URL, "token")
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

func TestNewForgejoProvider_EmptyHost(t *testing.T) {
	_, err := NewForgejoProvider("")
	if err == nil {
		t.Error("expected error for empty host")
	}
}

func TestNewForgejoProvider_MissingToken(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("FORGEJO_ACCESS_TOKEN", "")

	_, err := NewForgejoProvider("git.example.com")
	if err == nil {
		t.Error("expected error for missing token")
	}
}
