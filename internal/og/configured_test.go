package og

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadServiceComposesForgejoConfigurationAndHotRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "test-token")
	configDir := filepath.Join(home, ".config", "ttal")
	repo := testRegisteredRepo(t, home, branchMain, "http://forgejo.localhost:17480/owner/repo.git", false)
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.WriteFile(configPath, []byte(
		"[forgejo]\nallowed_base_urls = [\"http://forgejo.localhost:17480\"]\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	service, err := LoadService(configPath, configDir)
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	resp, err := service.AuthStatus(Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("AuthStatus: %v", err)
	}
	if resp.Auth == nil || resp.Auth.Provider != "forgejo" || !resp.Auth.TokenSet {
		t.Fatalf("auth = %+v", resp.Auth)
	}
	if _, err := service.ProjectStore().Get("test"); err != nil {
		t.Fatalf("ProjectStore: %v", err)
	}
}

func TestLoadServiceRejectsInvalidGitHubAppKey(t *testing.T) {
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "github-app.pem"), []byte("not a PEM key"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.WriteFile(configPath, []byte(`[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["tta-lab"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadService(configPath, configDir)
	if err == nil || !strings.Contains(err.Error(), "PEM") {
		t.Fatalf("LoadService error = %v, want PEM error", err)
	}
}

func TestLoadServiceBuildsGitHubAppBroker(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyData := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(filepath.Join(keyDir, "github-app.pem"), keyData, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.WriteFile(configPath, []byte(`[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["tta-lab"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := LoadService(configPath, configDir)
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	if !service.GitHubAppConfigured() {
		t.Fatal("service does not have GitHub App credentials")
	}
}
