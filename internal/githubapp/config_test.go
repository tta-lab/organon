package githubapp

import (
	"strings"
	"testing"
)

func TestConfigValidateNormalizesAllowedOwners(t *testing.T) {
	cfg := Config{
		AppID: 12345, KeySource: "file", KeyRef: "og/github-app.pem",
		AllowedOwners: []string{"tta-lab", "GuionAI", "LamplitIsles"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
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

func TestConfigValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{"missing app id", Config{KeySource: "file", KeyRef: "key.pem", AllowedOwners: []string{"tta-lab"}}, "app_id"},
		{"missing key source", Config{AppID: 1, KeyRef: "key.pem", AllowedOwners: []string{"tta-lab"}}, "key_source"},
		{
			"unknown key source",
			Config{AppID: 1, KeySource: "keychain", KeyRef: "key.pem", AllowedOwners: []string{"tta-lab"}},
			"key_source",
		},
		{"missing key ref", Config{AppID: 1, KeySource: "file", AllowedOwners: []string{"tta-lab"}}, "key_ref"},
		{"missing owners", Config{AppID: 1, KeySource: "file", KeyRef: "key.pem"}, "allowed_owners"},
		{
			"empty owner",
			Config{AppID: 1, KeySource: "file", KeyRef: "key.pem", AllowedOwners: []string{""}},
			"allowed_owners",
		},
		{
			"duplicate owner",
			Config{AppID: 1, KeySource: "file", KeyRef: "key.pem", AllowedOwners: []string{"tta-lab", "TTA-LAB"}},
			"duplicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tt.wantErr)
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
