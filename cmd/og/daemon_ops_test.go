package main

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

func TestLoadDaemonServiceWithoutGitHubAppConfig(t *testing.T) {
	configDir := t.TempDir()
	service, err := loadDaemonService(filepath.Join(configDir, "missing.toml"), configDir)
	if err != nil {
		t.Fatalf("loadDaemonService: %v", err)
	}
	if service.GitHubAppConfigured() {
		t.Fatal("service unexpectedly has GitHub App authentication")
	}
}

func TestLoadDaemonServiceRejectsInvalidGitHubAppConfig(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.WriteFile(configPath, []byte("[github_app]\napp_id = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadDaemonService(configPath, configDir)
	if err == nil || !strings.Contains(err.Error(), "app_id") {
		t.Fatalf("loadDaemonService error = %v", err)
	}
}

func TestLoadDaemonServiceRejectsInvalidGitHubAppKey(t *testing.T) {
	configDir, configPath := writeDaemonAppConfig(t, []byte("not a PEM key"))
	_, err := loadDaemonService(configPath, configDir)
	if err == nil || !strings.Contains(err.Error(), "PEM") {
		t.Fatalf("loadDaemonService error = %v", err)
	}
}

func TestLoadDaemonServiceConstructsConfiguredBroker(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	configDir, configPath := writeDaemonAppConfig(t, data)

	service, err := loadDaemonService(configPath, configDir)
	if err != nil {
		t.Fatalf("loadDaemonService: %v", err)
	}
	if !service.GitHubAppConfigured() {
		t.Fatal("service does not have GitHub App authentication")
	}
}

func writeDaemonAppConfig(t *testing.T, key []byte) (string, string) {
	t.Helper()
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "github-app.pem"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "og.toml")
	contents := `[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["tta-lab"]
`
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return configDir, configPath
}
