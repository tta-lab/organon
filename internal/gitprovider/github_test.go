package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
)

func TestContextTransportHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := newContextHTTPClient(ctx, roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport must not run after cancellation")
		return nil, nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Do(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("client error = %v, want context cancellation", err)
	}
}

func TestGitHubProviderHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider, err := NewGitHubProviderWithToken(ctx, "token")
	if err != nil {
		t.Fatalf("NewGitHubProviderWithToken: %v", err)
	}

	if _, err := provider.GetPR("o", "r", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetPR error = %v, want context cancellation", err)
	}
}

func TestGitHubProviderEditPRSendsEmptyBody(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/repos/o/r/pulls/7" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"number":7,"title":"new title","body":"","state":"open"}`))
	}))
	t.Cleanup(server.Close)
	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatal(err)
	}
	provider := &GitHubProvider{client: client}

	if _, err := provider.EditPR("o", "r", 7, "new title", ""); err != nil {
		t.Fatalf("EditPR: %v", err)
	}
	if body, ok := got["body"]; !ok || body != "" {
		t.Fatalf("request body = %#v, want explicit empty body", got)
	}
}

const testGitHubBaseBranch = "main"

func TestNewGitHubProviderWithTokenDoesNotUseAmbientToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ambient-token")
	_, err := NewGitHubProviderWithToken(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("NewGitHubProviderWithToken error = %v, want explicit token error", err)
	}
}

func TestGitHubProviderReportsConfirmedAuthFailureWithoutRetry(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		wantFailures int
	}{
		{name: "401", status: http.StatusUnauthorized, wantFailures: 1},
		{
			name: "403 integration permission", status: http.StatusForbidden,
			body: `{"message":"Resource not accessible by integration"}`, wantFailures: 1,
		},
		{
			name: "403 rate limit", status: http.StatusForbidden,
			body: `{"message":"API rate limit exceeded"}`, wantFailures: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests, failures int
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if got := r.Header.Get("Authorization"); got != "Bearer installation-token" {
					t.Fatalf("Authorization = %q", got)
				}
				return &http.Response{
					StatusCode: tt.status,
					Status:     http.StatusText(tt.status),
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    r,
				}, nil
			})
			client, err := github.NewClient(github.WithTransport(&githubTokenTransport{
				base: transport, token: "installation-token", onAuthFailure: func() { failures++ },
			}))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			provider := &GitHubProvider{client: client}

			_, err = provider.GetPR("tta-lab", "organon", 1)
			if err == nil {
				t.Fatal("expected API error")
			}
			if requests != 1 || failures != tt.wantFailures {
				t.Fatalf("requests = %d, callbacks = %d; want 1, %d", requests, failures, tt.wantFailures)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGitHubProviderFindPRByCommit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/commits/abc123/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("per_page"); got != "2" {
			t.Errorf("per_page = %q, want 2", got)
		}
		_, _ = w.Write([]byte(`[{
			"number": 56,
			"title": "fix pull",
			"state": "closed",
			"html_url": "https://github.com/o/r/pull/56",
			"merged_at": "2026-05-24T09:16:33Z",
			"head": {"ref": "feature/deleted-remote", "sha": "abc123"},
			"base": {"ref": "main"}
		}]`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	baseURL := server.URL + "/"
	client, err := github.NewClient(
		github.WithHTTPClient(server.Client()),
		github.WithURLs(&baseURL, &baseURL),
	)
	if err != nil {
		t.Fatalf("new GitHub client: %v", err)
	}
	provider := &GitHubProvider{client: client}

	pr, err := provider.FindPRByCommit("o", "r", "abc123")
	if err != nil {
		t.Fatalf("FindPRByCommit: %v", err)
	}
	if pr.Index != 56 || !pr.Merged || pr.Head != "feature/deleted-remote" || pr.Base != testGitHubBaseBranch {
		t.Fatalf("PR = %+v, want merged PR #56 for feature/deleted-remote -> main", pr)
	}
}

func TestFetchLogTailReadsPastFirst64KiB(t *testing.T) {
	body := strings.Repeat("setup success\n", 6000) + "actual failure\nexit status 1\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	got := fetchLogTail(context.Background(), server.URL, 2)
	want := "actual failure\nexit status 1"
	if got != want {
		t.Fatalf("fetchLogTail() = %q, want %q", got, want)
	}
}
