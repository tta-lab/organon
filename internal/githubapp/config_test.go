package githubapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigNormalizesAllowedOwners(t *testing.T) {
	path := writeConfig(t, `
[github_app]
app_id = 12345
key_source = "file"
key_ref = "og/github-app.pem"
allowed_owners = ["tta-lab", "GuionAI", "LamplitIsles"]
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.AppID != 12345 || cfg.KeySource != "file" || cfg.KeyRef != "og/github-app.pem" {
		t.Fatalf("config = %+v", cfg)
	}
	want := []string{"tta-lab", "guionai", "lamplitisles"}
	if strings.Join(cfg.AllowedOwners, ",") != strings.Join(want, ",") {
		t.Fatalf("allowed owners = %v, want %v", cfg.AllowedOwners, want)
	}
	if err := cfg.RequireOwner("GUIonAI"); err != nil {
		t.Fatalf("RequireOwner: %v", err)
	}
}

func TestLoadConfigRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"missing app id", `key_source = "file"\nkey_ref = "key.pem"\nallowed_owners = ["tta-lab"]`, "app_id"},
		{"missing key source", `app_id = 1\nkey_ref = "key.pem"\nallowed_owners = ["tta-lab"]`, "key_source"},
		{
			"unknown key source",
			`app_id = 1\nkey_source = "keychain"\nkey_ref = "key.pem"\nallowed_owners = ["tta-lab"]`,
			"key_source",
		},
		{"missing key ref", `app_id = 1\nkey_source = "file"\nallowed_owners = ["tta-lab"]`, "key_ref"},
		{"missing owners", `app_id = 1\nkey_source = "file"\nkey_ref = "key.pem"`, "allowed_owners"},
		{"empty owner", `app_id = 1\nkey_source = "file"\nkey_ref = "key.pem"\nallowed_owners = [""]`, "allowed_owners"},
		{
			"duplicate owner",
			`app_id = 1\nkey_source = "file"\nkey_ref = "key.pem"\nallowed_owners = ["tta-lab", "TTA-LAB"]`,
			"duplicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, "[github_app]\n"+tt.body)
			_, err := LoadConfig(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadConfig error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigRejectsOwnerOutsideAllowlist(t *testing.T) {
	cfg := Config{AllowedOwners: []string{"tta-lab"}}
	err := cfg.RequireOwner("outsider")
	if err == nil || !strings.Contains(err.Error(), "outsider") || !strings.Contains(err.Error(), "allowed_owners") {
		t.Fatalf("RequireOwner error = %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	body = strings.ReplaceAll(body, `\n`, "\n")
	path := filepath.Join(t.TempDir(), "og.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
