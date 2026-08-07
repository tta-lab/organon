package gitutil

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestGitHubAppGitEnvRoutesOriginsThroughCanonicalHTTPS(t *testing.T) {
	tests := []struct {
		name   string
		remote string
	}{
		{"SSH shorthand", "git@github.com:tta-lab/organon.git"},
		{"SSH URL", "ssh://git@github.com/tta-lab/organon.git"},
		{"HTTPS", "https://github.com/tta-lab/organon.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := GitHubAppGitEnv([]string{"PATH=/bin"}, tt.remote, "tta-lab", "organon", "test-token-secret")
			configs := gitConfigPairs(t, env)
			canonical := "https://github.com/tta-lab/organon.git"
			if tt.remote != canonical && configs["url."+canonical+".insteadOf"] != tt.remote {
				t.Fatalf("configs = %#v, want rewrite from %q to %q", configs, tt.remote, canonical)
			}
			for key, value := range configs {
				if strings.Contains(key, "test-token-secret") || strings.Contains(value, "test-token-secret") {
					t.Fatalf("token leaked into git config %q=%q", key, value)
				}
			}
			if envValue(env, "GIT_TOKEN_INJECT") != "test-token-secret" {
				t.Fatalf("GIT_TOKEN_INJECT missing from %v", env)
			}
		})
	}
}

func TestGitHubAppGitEnvClearsAmbientCredentialConfiguration(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"GITHUB_TOKEN=ambient-github",
		"GH_TOKEN=ambient-gh",
		"FORGEJO_TOKEN=ambient-forgejo",
		"FORGEJO_ACCESS_TOKEN=ambient-forgejo-access",
		"GITEA_TOKEN=ambient-gitea",
		"GIT_TERMINAL_PROMPT=1",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=store",
		"GIT_TOKEN_INJECT=ambient-token",
		"GIT_ASKPASS=/tmp/leaky-helper",
		"SSH_ASKPASS=/tmp/leaky-ssh-helper",
		"GCM_INTERACTIVE=always",
	}
	env := GitHubAppGitEnv(base, "git@github.com:tta-lab/organon.git", "tta-lab", "organon", "")
	if envValue(env, "PATH") != "/bin" || envValue(env, "GIT_TERMINAL_PROMPT") != "0" {
		t.Fatalf("environment = %v", env)
	}
	if envValue(env, "GIT_TOKEN_INJECT") != "" {
		t.Fatalf("anonymous environment retained token: %v", env)
	}
	for _, name := range []string{"GIT_ASKPASS", "SSH_ASKPASS", "GCM_INTERACTIVE"} {
		if envValue(env, name) != "" {
			t.Fatalf("anonymous environment retained %s: %v", name, env)
		}
	}
	for _, name := range []string{
		"GITHUB_TOKEN", "GH_TOKEN", "FORGEJO_TOKEN", "FORGEJO_ACCESS_TOKEN", "GITEA_TOKEN",
	} {
		if envValue(env, name) != "" {
			t.Fatalf("GitHub App environment retained %s: %v", name, env)
		}
	}
	configs := gitConfigPairs(t, env)
	if got, ok := configs["credential.helper"]; !ok || got != "" {
		t.Fatalf("credential.helper = %q, present %v; want explicit clear", got, ok)
	}
	if got, ok := configs["core.askPass"]; !ok || got != "" {
		t.Fatalf("core.askPass = %q, present %v; want explicit clear", got, ok)
	}
}

func gitConfigPairs(t *testing.T, env []string) map[string]string {
	t.Helper()
	countValue := envValue(env, "GIT_CONFIG_COUNT")
	if countValue == "" {
		t.Fatal("GIT_CONFIG_COUNT is missing")
	}
	var count int
	if _, err := fmt.Sscanf(countValue, "%d", &count); err != nil {
		t.Fatalf("parse GIT_CONFIG_COUNT %q: %v", countValue, err)
	}
	pairs := make(map[string]string, count)
	for i := range count {
		key := envValue(env, fmt.Sprintf("GIT_CONFIG_KEY_%d", i))
		pairs[key] = envValue(env, fmt.Sprintf("GIT_CONFIG_VALUE_%d", i))
	}
	return pairs
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func TestGitCredEnvWithTokenUsesExplicitToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ambient-token")

	env := GitCredEnvWithToken("resolved-token")
	if len(env) != 7 {
		t.Fatalf("expected 7 env vars, got %d: %v", len(env), env)
	}
	if env[6] != "GIT_TOKEN_INJECT=resolved-token" {
		t.Fatalf("env[6] = %q, want explicit resolved token", env[6])
	}
}

func TestForgejoGitEnvUsesOnlyResolvedToken(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"GITHUB_TOKEN=ambient-github",
		"FORGEJO_TOKEN=ambient-forgejo",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=store",
	}
	env := ForgejoGitEnv(base, "resolved-token")
	if envValue(env, "PATH") != "/bin" || envValue(env, "GIT_TOKEN_INJECT") != "resolved-token" {
		t.Fatalf("environment = %v", env)
	}
	for _, name := range []string{"GITHUB_TOKEN", "FORGEJO_TOKEN", "GIT_ASKPASS", "SSH_ASKPASS"} {
		if envValue(env, name) != "" {
			t.Fatalf("Forgejo environment retained %s: %v", name, env)
		}
	}
	configs := gitConfigPairs(t, env)
	if configs["credential.helper"] == "" || !strings.Contains(configs["credential.helper"], "GIT_TOKEN_INJECT") {
		t.Fatalf("Forgejo git configs = %#v, want explicit credential helper", configs)
	}
}

func TestAnonymousGitEnvClearsAmbientCredentials(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"GITHUB_TOKEN=github-secret",
		"GH_TOKEN=gh-secret",
		"FORGEJO_TOKEN=forgejo-secret",
		"FORGEJO_ACCESS_TOKEN=forgejo-access-secret",
		"GITEA_TOKEN=gitea-secret",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=store",
		"GIT_ASKPASS=/tmp/helper",
		"SSH_ASKPASS=/tmp/ssh-helper",
	}
	env := AnonymousGitEnv(base)
	if envValue(env, "PATH") != "/bin" || envValue(env, "GIT_TERMINAL_PROMPT") != "0" {
		t.Fatalf("environment = %v", env)
	}
	for _, name := range []string{
		"GITHUB_TOKEN", "GH_TOKEN", "FORGEJO_TOKEN", "FORGEJO_ACCESS_TOKEN", "GITEA_TOKEN",
		"GIT_ASKPASS", "SSH_ASKPASS", "GIT_TOKEN_INJECT",
	} {
		if envValue(env, name) != "" {
			t.Fatalf("anonymous environment retained %s: %v", name, env)
		}
	}
	configs := gitConfigPairs(t, env)
	if configs["credential.helper"] != "" || configs["core.askPass"] != "" {
		t.Fatalf("anonymous git configs = %#v, want credential helpers cleared", configs)
	}
}

func TestControlledGitEnvironmentsDisableAmbientTracing(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"GIT_TRACE=1",
		"GIT_TRACE_CURL=1",
		"GIT_CURL_VERBOSE=1",
		"GIT_TRACE_PACKET=/tmp/packet.log",
		"GIT_TRACE2_EVENT=/tmp/trace.json",
		"GIT_CONFIG_GLOBAL=/tmp/leaky-global-config",
		"GIT_CONFIG_SYSTEM=/tmp/leaky-system-config",
		"GIT_CONFIG_PARAMETERS='credential.helper=!echo leaked'",
		"GIT_CONFIG_NOSYSTEM=0",
		"GIT_DIR=/tmp/other.git",
		"GIT_WORK_TREE=/tmp/other-worktree",
		"GIT_COMMON_DIR=/tmp/other-common",
		"GIT_INDEX_FILE=/tmp/other-index",
		"GIT_OBJECT_DIRECTORY=/tmp/other-objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/tmp/alternate-objects",
		"GIT_EXEC_PATH=/tmp/fake-git-exec",
		"GIT_TEMPLATE_DIR=/tmp/fake-template",
		"EXA_API_KEY=unrelated-api-key",
		"CUSTOM_SECRET=unrelated-secret",
		"CUSTOM_PASSWORD=unrelated-password",
	}
	environments := map[string][]string{
		"anonymous": AnonymousGitEnv(base),
		"github":    GitHubAppGitEnv(base, "https://github.com/tta-lab/example.git", "tta-lab", "example", "secret"),
		"forgejo":   ForgejoGitEnv(base, "secret"),
	}
	for name, env := range environments {
		t.Run(name, func(t *testing.T) {
			for _, variable := range []string{
				"GIT_TRACE", "GIT_TRACE_CURL", "GIT_CURL_VERBOSE", "GIT_TRACE_PACKET", "GIT_TRACE2_EVENT",
				"GIT_CONFIG_SYSTEM", "GIT_CONFIG_PARAMETERS", "GIT_DIR", "GIT_WORK_TREE",
				"GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
				"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_EXEC_PATH", "GIT_TEMPLATE_DIR",
				"EXA_API_KEY", "CUSTOM_SECRET", "CUSTOM_PASSWORD",
			} {
				if value := envValue(env, variable); value != "" {
					t.Fatalf("environment retained %s=%q: %v", variable, value, env)
				}
			}
			if envValue(env, "GIT_CONFIG_GLOBAL") != "/dev/null" ||
				envValue(env, "GIT_CONFIG_NOSYSTEM") != "1" {
				t.Fatal("controlled Git environment retained ambient global/system configuration")
			}
			configs := gitConfigPairs(t, env)
			if configs["core.hooksPath"] != os.DevNull || configs["http.sslVerify"] != "true" {
				t.Fatalf("controlled git configs = %#v, want hooks disabled and TLS verification enabled", configs)
			}
		})
	}
}
