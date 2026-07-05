package navidrome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigSecretPrecedence(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "https://env.example")
	t.Setenv("NAVIDROME_USER", "env-user")
	t.Setenv("NAVIDROME_PASS", "env-pass")
	t.Setenv("CONFIG_PASS", "config-env-pass")

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
server = "https://config.example"
username = "config-user"
password = "config-pass"
password_env = "CONFIG_PASS"
client = "config-client"
api_version = "1.15.0"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(ConfigOptions{
		Path:       path,
		Server:     "https://flag.example",
		Username:   "flag-user",
		Password:   "flag-pass",
		Client:     "flag-client",
		APIVersion: "1.16.1",
	})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Server != "https://flag.example" {
		t.Fatalf("Server = %q", cfg.Server)
	}
	if cfg.Username != "flag-user" {
		t.Fatalf("Username = %q", cfg.Username)
	}
	if cfg.Password != "flag-pass" {
		t.Fatalf("Password = %q", cfg.Password)
	}
	if cfg.Client != "flag-client" {
		t.Fatalf("Client = %q", cfg.Client)
	}
	if cfg.APIVersion != "1.16.1" {
		t.Fatalf("APIVersion = %q", cfg.APIVersion)
	}

	cfg, err = LoadConfig(ConfigOptions{Path: path})
	if err != nil {
		t.Fatalf("LoadConfig without flags: %v", err)
	}
	if cfg.Server != "https://env.example" || cfg.Username != "env-user" || cfg.Password != "env-pass" {
		t.Fatalf("env precedence not applied: %+v", cfg)
	}

	t.Setenv("NAVIDROME_PASS", "")
	if err := os.Unsetenv("NAVIDROME_PASS"); err != nil {
		t.Fatalf("unset NAVIDROME_PASS: %v", err)
	}
	cfg, err = LoadConfig(ConfigOptions{Path: path})
	if err != nil {
		t.Fatalf("LoadConfig password_env: %v", err)
	}
	if cfg.Password != "config-pass" {
		t.Fatalf("Password = %q, want config plaintext before password_env", cfg.Password)
	}
}

func TestLoadConfigRejectsMissingAuthFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`server = "https://music.example"`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(ConfigOptions{Path: path})
	if err == nil {
		t.Fatal("LoadConfig succeeded, want missing auth error")
	}
}

func TestLoadConfigUsesPasswordEnvWhenPlaintextPasswordAbsent(t *testing.T) {
	t.Setenv("CONFIG_PASS", "env-secret")
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
server = "https://music.example"
username = "ooneil"
password_env = "CONFIG_PASS"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(ConfigOptions{Path: path})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Password != "env-secret" {
		t.Fatalf("Password = %q", cfg.Password)
	}
}
