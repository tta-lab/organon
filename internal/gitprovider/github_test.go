package gitprovider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
)

const testGitHubBaseBranch = "main"

func TestNewGitHubProviderWithTokenDoesNotUseAmbientToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ambient-token")
	_, err := NewGitHubProviderWithToken("")
	if err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("NewGitHubProviderWithToken error = %v, want explicit token error", err)
	}
}

func TestGitHubProviderReportsConfirmedAuthFailureWithoutRetry(t *testing.T) {
	var requests, failures int
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer installation-token" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    r,
		}, nil
	})
	client, err := github.NewClient(github.WithTransport(&githubTokenTransport{
		base:          transport,
		token:         "installation-token",
		onAuthFailure: func() { failures++ },
	}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	provider := &GitHubProvider{client: client}

	_, err = provider.GetPR("tta-lab", "organon", 1)
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if requests != 1 || failures != 1 {
		t.Fatalf("requests = %d, auth failure callbacks = %d; want 1, 1", requests, failures)
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

	got := fetchLogTail(server.URL, 2)
	want := "actual failure\nexit status 1"
	if got != want {
		t.Fatalf("fetchLogTail() = %q, want %q", got, want)
	}
}
