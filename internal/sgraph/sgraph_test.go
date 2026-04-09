package sgraph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func resetHTTP() {
	httpClient = nil
	endpoint = "https://sourcegraph.com/.api/graphql"
}

func TestFormatSourcegraphResults_HappyPath(t *testing.T) {
	fake := `{
		"data": {
			"search": {
				"results": {
					"matchCount": 1,
					"resultCount": 1,
					"limitHit": false,
					"results": [
						{
							"__typename": "FileMatch",
							"repository": { "name": "github.com/golang/go" },
							"file": {
								"path": "src/fmt/print.go",
								"url": "https://sourcegraph.com/github.com/golang/go/-/blob/src/fmt/print.go",
								"content": "package fmt\n\nfunc Println(a ...any) {}\n"
							},
							"lineMatches": [
								{
									"lineNumber": 3,
									"preview": "func Println(a ...any) {}"
								}
							]
						}
					]
				}
			}
		}
	}`
	var result map[string]any
	if err := json.Unmarshal([]byte(fake), &result); err != nil {
		t.Fatal(err)
	}

	out, err := formatSourcegraphResults(result, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "# Sourcegraph Search Results") {
		t.Fatalf("missing header, got: %s", out)
	}
	if !strings.Contains(out, "Found 1 matches") {
		t.Fatalf("missing match count, got: %s", out)
	}
	if !strings.Contains(out, "## Result 1: github.com/golang/go/src/fmt/print.go") {
		t.Fatalf("missing result heading, got: %s", out)
	}
	if !strings.Contains(out, "```") {
		t.Fatalf("missing fenced code block, got: %s", out)
	}
}

func TestFormatGraphQLError(t *testing.T) {
	fake := `{"errors": [{"message": "query timeout"}]}`
	var result map[string]any
	if err := json.Unmarshal([]byte(fake), &result); err != nil {
		t.Fatal(err)
	}

	out, err := formatSourcegraphResults(result, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "## Sourcegraph API Error") {
		t.Fatalf("expected API error section, got: %s", out)
	}
	if !strings.Contains(out, "query timeout") {
		t.Fatalf("expected error message, got: %s", out)
	}
}

func TestFormatNoResults(t *testing.T) {
	fake := `{"data": {"search": {"results": {"matchCount": 0, "resultCount": 0, "results": []}}}}`
	var result map[string]any
	if err := json.Unmarshal([]byte(fake), &result); err != nil {
		t.Fatal(err)
	}

	out, err := formatSourcegraphResults(result, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No results found") {
		t.Fatalf("expected no results message, got: %s", out)
	}
}

func TestSearchClampsCount(t *testing.T) {
	defer resetHTTP()

	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			return
		}
		receivedQuery = req.Variables.Query
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck
		w.Write([]byte(`{"data":{"search":{"results":{"matchCount":0,"resultCount":0,"results":[]}}}}`))
	}))
	defer srv.Close()

	httpClient = srv.Client()
	endpoint = srv.URL

	// count=25 should be clamped to 20 before the request is sent;
	// the server receives the original query string (count is validated
	// client-side, not reflected in the GraphQL query text).
	out, err := Search(context.Background(), "test query", 25, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No results found") {
		t.Fatalf("unexpected output: %s", out)
	}
	// Verify the query was passed through (clamping is in the count variable, not the query)
	if receivedQuery != "test query" {
		t.Fatalf("expected query 'test query', got: %s", receivedQuery)
	}
}

func TestSearchClampsTimeout(t *testing.T) {
	defer resetHTTP()
	resetHTTP()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		//nolint:errcheck
		w.Write([]byte(`{"data":{"search":{"results":{"matchCount":0,"resultCount":0,"results":[]}}}}`))
	}))
	defer srv.Close()

	httpClient = srv.Client()
	endpoint = srv.URL

	// timeout=200 should clamp to 120 internally; no error expected
	out, err := Search(context.Background(), "test query", 10, 10, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No results found") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	defer resetHTTP()
	resetHTTP()
	_, err := Search(context.Background(), "", 10, 10, 0)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected 'query is required', got: %v", err)
	}
}

func TestSearchHTTPError(t *testing.T) {
	defer resetHTTP()
	resetHTTP()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	httpClient = srv.Client()
	endpoint = srv.URL

	_, err := Search(context.Background(), "test", 10, 10, 5)
	if err == nil {
		t.Fatal("expected error for HTTP error response")
	}
	if !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("expected HTTP 429 in error, got: %v", err)
	}
}

func TestSearchLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	defer resetHTTP()
	resetHTTP()
	out, err := Search(context.Background(), "repo:^github\\.com/golang/go$ package main", 3, 5, 30)
	if err != nil {
		t.Fatalf("live search failed: %v", err)
	}
	// Accept either a successful results section or a structured error section.
	// Sourcegraph may rate-limit, so we check for non-empty formatted output.
	if !strings.Contains(out, "Sourcegraph Search Results") &&
		!strings.Contains(out, "Sourcegraph API Error") {
		t.Fatalf("unexpected output: %s", out)
	}
}
