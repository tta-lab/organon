package ogconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestLoadNormalizesForgejoAllowedBaseURLs(t *testing.T) {
	path := writeConfig(t, `
[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["TTA-LAB"]

[forgejo]
allowed_base_urls = ["HTTP://Forgejo.Localhost:17480/", "https://code.example:443"]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantURLs := []string{"http://forgejo.localhost:17480", "https://code.example"}
	if !reflect.DeepEqual(cfg.Forgejo.AllowedBaseURLs, wantURLs) {
		t.Fatalf("allowed base URLs = %v, want %v", cfg.Forgejo.AllowedBaseURLs, wantURLs)
	}
	if cfg.GitHubApp == nil || cfg.GitHubApp.AllowedOwners[0] != "tta-lab" {
		t.Fatalf("GitHub App config = %+v, want normalized owner", cfg.GitHubApp)
	}
}

func TestLoadAllowsMissingForgejoAndGitHubAppSections(t *testing.T) {
	cfg, err := Load(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitHubApp != nil || len(cfg.Forgejo.AllowedBaseURLs) != 0 {
		t.Fatalf("config = %+v, want empty optional sections", cfg)
	}
}

func TestLoadRejectsInvalidForgejoAllowedBaseURLs(t *testing.T) {
	tests := []struct {
		name string
		urls string
		want string
	}{
		{"userinfo", `"https://user@forge.example"`, "userinfo"},
		{"path", `"https://forge.example/api"`, "path"},
		{"query", `"https://forge.example?token=x"`, "query"},
		{"fragment", `"https://forge.example#x"`, "fragment"},
		{"unsupported scheme", `"ssh://forge.example"`, "HTTP(S)"},
		{"missing host", `"https:///owner"`, "host"},
		{"duplicate normalized URL", `"https://FORGE.example", "https://forge.example:443/"`, "duplicate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, "[forgejo]\nallowed_base_urls = ["+tt.urls+"]\n"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestConfigClassifiesRemoteByExplicitTrustBoundary(t *testing.T) {
	cfg, err := Load(writeConfig(t, `[forgejo]
allowed_base_urls = ["http://forgejo.localhost:17480"]
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		name    string
		remote  string
		want    gitprovider.ProviderType
		wantErr string
	}{
		{"GitHub", "https://github.com/tta-lab/organon.git", gitprovider.ProviderGitHub, ""},
		{"GitHub over HTTP", "http://github.com/tta-lab/organon.git", "", "not allowed"},
		{"GitHub over SSH", "ssh://git@github.com/tta-lab/organon.git", "", "HTTP(S)"},
		{"allowed Forgejo", "http://forgejo.localhost:17480/GuionAI/flicknote.git", gitprovider.ProviderForgejo, ""},
		{"generic HTTPS", "https://codeberg.org/forgejo/forgejo.git", gitprovider.ProviderGeneric, ""},
		{"unlisted HTTP", "http://forge.example/owner/repo.git", "", "not allowed"},
		{"SSH", "ssh://git@forgejo.localhost:17480/owner/repo.git", "", "HTTP(S)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := gitprovider.ParseRemoteURL(tt.remote)
			if err != nil {
				t.Fatalf("ParseRemoteURL: %v", err)
			}
			got, err := cfg.ClassifyRemote(info)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ClassifyRemote error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ClassifyRemote = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "og.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
