package og

import (
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
)

func TestAuthStatusReportsGitHubAppPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature")
	gitRun(t, repo, "checkout", "--detach")
	broker := &recordingBroker{status: githubapp.InstallationStatus{
		AppID:          12345,
		InstallationID: 41,
		Repository:     "tta-lab/example",
		Permissions: map[string]string{
			"contents": "write", "pull_requests": "write", "checks": "read",
			"actions": "read", "workflows": "write",
		},
	}}

	resp, err := NewService(broker).AuthStatus(Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("AuthStatus: %v", err)
	}
	if resp.Auth == nil || !resp.Auth.Ready || resp.Auth.Project != "test" || resp.Auth.AuthMode != "github-app" {
		t.Fatalf("structured auth = %+v", resp.Auth)
	}
	for _, want := range []string{
		"provider: github", "repo: tta-lab/example", "auth: github-app",
		"app_id: 12345", "installation: ready", "repository_scope: tta-lab/example", "key_source: file",
		"contents:write: ready", "pull_requests:write: ready", "checks:read: ready",
		"actions:read: ready", "workflows:write: ready",
	} {
		if !strings.Contains(resp.Message, want) {
			t.Fatalf("message missing %q:\n%s", want, resp.Message)
		}
	}
}

func TestAuthStatusNamesMissingGitHubAppPermission(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature")
	broker := &recordingBroker{status: githubapp.InstallationStatus{
		AppID:          12345,
		InstallationID: 41,
		Repository:     "tta-lab/example",
		Permissions: map[string]string{
			"contents": "write", "pull_requests": "write", "checks": "",
			"actions": "read", "workflows": "write",
		},
	}}

	_, err := NewService(broker).AuthStatus(Request{WorkDir: repo})
	if err == nil || !strings.Contains(err.Error(), "checks:read: missing") ||
		!strings.Contains(err.Error(), "installation owner must approve") {
		t.Fatalf("AuthStatus error = %v", err)
	}
}

func TestAuthStatusReportsGitHubAppNotConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature")

	_, err := NewService(nil).AuthStatus(Request{WorkDir: repo})
	if err == nil || !strings.Contains(err.Error(), "GitHub App authentication is not configured") {
		t.Fatalf("AuthStatus error = %v", err)
	}
}
