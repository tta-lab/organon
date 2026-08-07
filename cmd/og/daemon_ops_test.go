package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/og"
)

func TestLoadDaemonServiceUsesConfiguredForgejoAllowlist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "test-token")
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	gitCommand(t, "", "init", repo)
	gitCommand(t, repo, "remote", "add", "origin", "http://forgejo.localhost:17480/owner/repo.git")
	projects := "[test]\npath = " + strconv.Quote(repo) + "\n"
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(projects), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "og.toml")
	if err := os.WriteFile(configPath, []byte(`[forgejo]
allowed_base_urls = ["http://forgejo.localhost:17480"]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	service, err := loadDaemonService(configPath, configDir)
	if err != nil {
		t.Fatalf("loadDaemonService: %v", err)
	}
	resp, err := service.AuthStatus(og.Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("AuthStatus: %v", err)
	}
	if resp.Auth == nil || resp.Auth.Provider != "forgejo" || !resp.Auth.TokenSet {
		t.Fatalf("auth = %+v, want configured Forgejo token status", resp.Auth)
	}
}

func TestDaemonValidateLoadsConfigWithoutBindingSocket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(home, "code", "projects", "test")
	if err := os.WriteFile(
		filepath.Join(configDir, "projects.toml"),
		[]byte("[test]\npath = "+strconv.Quote(projectPath)+"\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "og.toml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(home, "validate.sock")
	t.Setenv("OG_DAEMON_SOCKET", socket)
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"daemon", "validate"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("daemon validate: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok", stdout.String())
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("validate created socket %q: %v", socket, err)
	}
}

func TestDaemonValidateRejectsInvalidProjectRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "projects.toml"),
		[]byte("[test]\npath = \"relative\"\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "og.toml"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd(&bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"daemon", "validate"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("daemon validate error = %v, want project registry validation", err)
	}
}

func gitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

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
