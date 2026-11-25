package og

import (
	"strings"
	"testing"
)

func TestAuthStatusReportsGenericHTTPSAsAnonymousReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "must-not-leak")
	repo := testRegisteredRepo(t, home, "feature", "https://codeberg.org/forgejo/forgejo.git", false)

	resp, err := NewService(nil).AuthStatus(Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("AuthStatus: %v", err)
	}
	if resp.Auth == nil || !resp.Auth.Ready || resp.Auth.Provider != "generic" ||
		resp.Auth.AuthMode != "anonymous" || resp.Auth.TokenEnv != "" || resp.Auth.TokenSet {
		t.Fatalf("structured auth = %+v, want anonymous generic status", resp.Auth)
	}
	if !strings.Contains(resp.Message, "auth: anonymous") || !strings.Contains(resp.Message, "remote: read-only") {
		t.Fatalf("message = %q, want anonymous read-only status", resp.Message)
	}
}
