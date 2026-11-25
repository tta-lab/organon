package og

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	service, err := LoadService(configPath)
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

func TestForgejoStatusOmitsUnsetToken(t *testing.T) {
	data, err := json.Marshal(AuthStatus{Provider: "forgejo", AuthMode: "token", TokenEnv: "FORGEJO_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	_, hasToken := got["token_set"]
	if got["ready"] != false || hasToken || got["token_env"] != "FORGEJO_TOKEN" {
		t.Fatalf("status: %s", data)
	}
}
