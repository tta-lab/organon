package gitprovider

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantHost  string
		wantErr   bool
	}{
		{
			name:      "SSH shorthand",
			url:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "github.com",
			wantErr:   false,
		},
		{
			name:      "SSH shorthand without .git",
			url:       "git@git.guion.io:clawteam/myproject",
			wantOwner: "clawteam",
			wantRepo:  "myproject",
			wantHost:  "git.guion.io",
			wantErr:   false,
		},
		{
			name:      "SSH protocol",
			url:       "ssh://git@github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "github.com",
			wantErr:   false,
		},
		{
			name:      "SSH protocol with port",
			url:       "ssh://git@git.example.com:2222/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "git.example.com",
			wantErr:   false,
		},
		{
			name:      "HTTPS URL",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "github.com",
			wantErr:   false,
		},
		{
			name:      "HTTPS URL without .git",
			url:       "https://git.guion.io/clawteam/project",
			wantOwner: "clawteam",
			wantRepo:  "project",
			wantHost:  "git.guion.io",
			wantErr:   false,
		},
		{
			name:      "HTTP URL",
			url:       "http://git.example.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "git.example.com",
			wantErr:   false,
		},
		{
			name:    "malformed - no slash",
			url:     "git@github.com:justrepo",
			wantErr: true,
		},
		{
			name:    "malformed - empty owner",
			url:     "git@github.com:/repo.git",
			wantErr: true,
		},
		{
			name:    "malformed - empty repo",
			url:     "git@github.com:owner/.git",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRemoteURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRemoteURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("ParseRemoteURL().Owner = %v, want %v", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("ParseRemoteURL().Repo = %v, want %v", got.Repo, tt.wantRepo)
			}
			if got.Host != tt.wantHost {
				t.Errorf("ParseRemoteURL().Host = %v, want %v", got.Host, tt.wantHost)
			}
		})
	}
}

func TestDetectProviderFromHost(t *testing.T) {
	tests := []struct {
		host         string
		wantProvider ProviderType
	}{
		{"github.com", ProviderGitHub},
		{"GitHub.com", ProviderGitHub},
		{"GITHUB.COM", ProviderGitHub},
		{"git.guion.io", ProviderForgejo},
		{"git.example.com", ProviderForgejo},
		{"codeberg.org", ProviderForgejo},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := detectProviderFromHost(tt.host)
			if got != tt.wantProvider {
				t.Errorf("detectProviderFromHost(%q) = %v, want %v", tt.host, got, tt.wantProvider)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path      string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"owner/repo", "owner", "repo", false},
		{"owner/repo.git", "owner", "repo", false},
		{"clawteam/my-project", "clawteam", "my-project", false},
		{"", "", "", true},
		{"nogitconfig", "", "", true},
		{"/repo", "", "", true},
		{"owner/", "", "", true},
		{"/repo.git", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			owner, repo, err := splitPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("splitPath() = (%v, %v), want (%v, %v)", owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

func TestWebURL(t *testing.T) {
	baseCases := []struct {
		name    string
		repo    *RepoInfo
		wantURL string
	}{
		{
			name:    "GitHub",
			repo:    &RepoInfo{Owner: "tta-lab", Repo: "ttal-cli", Provider: ProviderGitHub, Host: "github.com"},
			wantURL: "https://github.com/tta-lab/ttal-cli",
		},
		{
			name:    "Forgejo without FORGEJO_URL",
			repo:    &RepoInfo{Owner: "clawteam", Repo: "myproject", Provider: ProviderForgejo, Host: "git.guion.io"},
			wantURL: "https://git.guion.io/clawteam/myproject",
		},
		{
			name:    "Forgejo HTTP remote with port",
			repo:    mustParseRemoteURL(t, "http://forgejo.localhost:17480/GuionAI/flick-backend.git"),
			wantURL: "http://forgejo.localhost:17480/GuionAI/flick-backend",
		},
		{
			name:    "Forgejo SSH remote",
			repo:    mustParseRemoteURL(t, "ssh://git@git.example.com:2222/owner/repo.git"),
			wantURL: "https://git.example.com/owner/repo",
		},
	}

	for _, tt := range baseCases {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.repo.WebURL()
			if got != tt.wantURL {
				t.Errorf("WebURL() = %v, want %v", got, tt.wantURL)
			}
		})
	}
}

func TestNewProviderWithTokenUsesRemoteBaseURL(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("request path = %q, want /api/v1/version", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"11.0.0"}`))
	}))
	defer server.Close()

	repo := mustParseRemoteURL(t, server.URL+"/GuionAI/flick-backend.git")
	provider, err := NewProviderWithToken(repo, "unused")
	if err != nil {
		t.Fatalf("NewProviderWithToken(): %v", err)
	}
	if provider.Name() != string(ProviderForgejo) {
		t.Fatalf("provider.Name() = %q, want %q", provider.Name(), ProviderForgejo)
	}
}

func mustParseRemoteURL(t *testing.T, remote string) *RepoInfo {
	t.Helper()
	repo, err := ParseRemoteURL(remote)
	if err != nil {
		t.Fatalf("ParseRemoteURL(%q): %v", remote, err)
	}
	return repo
}
