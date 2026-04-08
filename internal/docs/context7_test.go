package docs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClient_Resolve_OK(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"results": []map[string]any{
				{
					"id":            "/reactjs/react.dev",
					"title":         "React",
					"description":   "Library",
					"trustScore":    10.0,
					"totalSnippets": 2779,
					"versions":      []string{"18.2.0", "17.0.2"},
				},
				{
					"id":            "/facebook/react",
					"title":         "React Native",
					"description":   "Mobile",
					"trustScore":    9.0,
					"totalSnippets": 1000,
					"versions":      []string{},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-key", server.URL)
	libs, err := client.Resolve(context.Background(), "react")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(libs) != 2 {
		t.Fatalf("expected 2 libraries, got %d", len(libs))
	}
	if libs[0].ID != "/reactjs/react.dev" {
		t.Errorf("expected ID /reactjs/react.dev, got %s", libs[0].ID)
	}
	if libs[0].Title != "React" {
		t.Errorf("expected title React, got %s", libs[0].Title)
	}
	if libs[0].TrustScore != 10.0 {
		t.Errorf("expected trustScore 10.0, got %f", libs[0].TrustScore)
	}
	if libs[0].TotalSnippets != 2779 {
		t.Errorf("expected totalSnippets 2779, got %d", libs[0].TotalSnippets)
	}
	if len(libs[0].Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(libs[0].Versions))
	}

	// Assert request path and query
	if capturedReq.URL.Path != "/api/v1/search" {
		t.Errorf("expected path /api/v1/search, got %s", capturedReq.URL.Path)
	}
	q := capturedReq.URL.Query()
	if q.Get("query") != "react" {
		t.Errorf("expected query=react, got %s", q.Get("query"))
	}

	// Assert Authorization header
	if auth := capturedReq.Header.Get("Authorization"); auth != "Bearer test-key" {
		t.Errorf("expected Authorization: Bearer test-key, got %s", auth)
	}
}

func TestClient_Resolve_NoAuthHeaderWhenUnset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	client := NewClientWithBaseURL("", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_Resolve_TrimsKey(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	client := NewClientWithBaseURL("  test-key  ", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if auth := capturedReq.Header.Get("Authorization"); auth != "Bearer test-key" {
		t.Errorf("expected Authorization: Bearer test-key, got %s", auth)
	}
}

func TestClient_Resolve_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	libs, err := client.Resolve(context.Background(), "nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(libs) != 0 {
		t.Errorf("expected 0 libraries, got %d", len(libs))
	}
}

func TestClient_Resolve_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json{"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "decode") && !contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention decode/invalid, got: %s", err.Error())
	}
}

func TestClient_Resolve_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "resolve") {
		t.Errorf("expected error to mention 'resolve', got: %s", errStr)
	}
	if !contains(errStr, "HTTP 404") {
		t.Errorf("expected error to mention 'HTTP 404', got: %s", errStr)
	}
	if contains(errStr, "web docs resolve") {
		t.Errorf("resolve 404 should NOT have 'web docs resolve' hint, got: %s", errStr)
	}
}

func TestClient_Resolve_HTTP429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "rate limited") {
		t.Errorf("expected error to mention 'rate limited', got: %s", errStr)
	}
	if !contains(errStr, "CONTEXT7_API_KEY") {
		t.Errorf("expected error to mention 'CONTEXT7_API_KEY', got: %s", errStr)
	}
}

func TestClient_Resolve_HTTP401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid key"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("bad-key", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "invalid CONTEXT7_API_KEY") {
		t.Errorf("expected error to mention 'invalid CONTEXT7_API_KEY', got: %s", errStr)
	}
	if !contains(errStr, "ctx7sk") {
		t.Errorf("expected error to mention 'ctx7sk', got: %s", errStr)
	}
}

func TestClient_Resolve_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error snippet"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Resolve(context.Background(), "react")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "HTTP 500") {
		t.Errorf("expected error to mention 'HTTP 500', got: %s", errStr)
	}
	if !contains(errStr, "internal error snippet") {
		t.Errorf("expected error to contain body snippet, got: %s", errStr)
	}
}

func TestClient_Docs_OK(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		_, _ = io.WriteString(w, "React hooks documentation content")
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-key", server.URL)
	out, err := client.Docs(context.Background(), "/reactjs/react.dev", "hooks", 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "React hooks documentation content" {
		t.Errorf("unexpected output: %s", out)
	}

	q := capturedReq.URL.Query()
	if q.Get("type") != "txt" {
		t.Errorf("expected type=txt, got %s", q.Get("type"))
	}
	if q.Get("topic") != "hooks" {
		t.Errorf("expected topic=hooks, got %s", q.Get("topic"))
	}
	if q.Get("tokens") != "500" {
		t.Errorf("expected tokens=500, got %s", q.Get("tokens"))
	}
}

func TestClient_Docs_StripsLeadingSlash(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		_, _ = io.WriteString(w, "docs")
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/reactjs/react.dev", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.URL.Path != "/api/v1/reactjs/react.dev" {
		t.Errorf("expected path /api/v1/reactjs/react.dev, got %s", capturedReq.URL.Path)
	}
}

func TestClient_Docs_OmitsEmptyParams(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		_, _ = io.WriteString(w, "docs")
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/reactjs/react.dev", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	q := capturedReq.URL.Query()
	if q.Has("topic") {
		t.Errorf("expected no topic param, got %s", q.Get("topic"))
	}
	if q.Has("tokens") {
		t.Errorf("expected no tokens param, got %s", q.Get("tokens"))
	}
	if q.Get("type") != "txt" {
		t.Errorf("expected type=txt, got %s", q.Get("type"))
	}
}

func TestClient_Docs_TopicEscaped(t *testing.T) {
	var capturedReq http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = *r
		_, _ = io.WriteString(w, "docs")
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/reactjs/react.dev", "how to handle errors & retries", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := url.QueryUnescape(capturedReq.URL.Query().Get("topic"))
	if err != nil {
		t.Fatalf("failed to decode topic: %v", err)
	}
	if decoded != "how to handle errors & retries" {
		t.Errorf("expected decoded topic 'how to handle errors & retries', got %s", decoded)
	}
}

func TestClient_Docs_HTTP202(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("still indexing"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/reactjs/react.dev", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "still indexing") {
		t.Errorf("expected error to mention 'still indexing', got: %s", errStr)
	}
	if !contains(errStr, "HTTP 202") {
		t.Errorf("expected error to mention 'HTTP 202', got: %s", errStr)
	}
}

func TestClient_Docs_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/nope/nope", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !contains(errStr, "docs") {
		t.Errorf("expected error to mention 'docs', got: %s", errStr)
	}
	if !contains(errStr, "HTTP 404") {
		t.Errorf("expected error to mention 'HTTP 404', got: %s", errStr)
	}
	if !contains(errStr, "web docs resolve") {
		t.Errorf("expected error to mention 'web docs resolve' hint, got: %s", errStr)
	}
}

func TestClient_Docs_HTTP429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	_, err := client.Docs(context.Background(), "/reactjs/react.dev", "", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "rate limited") {
		t.Errorf("expected error to mention 'rate limited', got: %s", err.Error())
	}
}

func TestClient_Docs_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(""))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("key", server.URL)
	out, err := client.Docs(context.Background(), "/reactjs/react.dev", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
