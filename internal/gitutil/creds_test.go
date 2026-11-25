package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlledGitEnvironmentsRespectSupportedGlobalHTTPVersion(t *testing.T) {
	for _, version := range []string{"HTTP/1.1", "HTTP/2"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			writeGlobalGitConfig(t, home, "[http]\n\tversion = "+version+"\n")
			base := []string{
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + home,
				"GIT_CONFIG_GLOBAL=" + filepath.Join(t.TempDir(), "ambient-global"),
				"GIT_CONFIG_SYSTEM=" + filepath.Join(t.TempDir(), "ambient-system"),
				"GIT_CONFIG_NOSYSTEM=0",
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=http.version",
				"GIT_CONFIG_VALUE_0=HTTP/3",
			}
			environments := map[string][]string{
				"anonymous": AnonymousGitEnv(base),
				"github":    GitHubGitEnv(base, nil),
				"forgejo":   ForgejoGitEnv(base, ""),
			}
			for name, env := range environments {
				t.Run(name, func(t *testing.T) {
					configs := gitConfigPairs(t, env)
					if got := configs[globalHTTPVersionKey]; got != version {
						t.Fatalf("http.version = %q, want %q; configs = %#v", got, version, configs)
					}
				})
			}
		})
	}
}

func TestControlledGitEnvironmentsIgnoreMissingAndUnsupportedGlobalHTTPVersion(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "missing"},
		{name: "unsupported", config: "[http]\n\tversion = HTTP/3\n"},
		{name: "multiple", config: "[http]\n\tversion = HTTP/1.1\n\tversion = HTTP/2\n"},
		{name: "malformed", config: "[http\nversion = HTTP/1.1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.config != "" {
				writeGlobalGitConfig(t, home, tt.config)
			}
			base := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home}
			environments := map[string][]string{
				"anonymous": AnonymousGitEnv(base),
				"github":    GitHubGitEnv(base, nil),
				"forgejo":   ForgejoGitEnv(base, ""),
			}
			for name, env := range environments {
				t.Run(name, func(t *testing.T) {
					configs := gitConfigPairs(t, env)
					if _, ok := configs[globalHTTPVersionKey]; ok {
						t.Fatalf("unsupported global http.version was injected: %#v", configs)
					}
				})
			}
		})
	}
}

func TestControlledGitEnvironmentsIgnoreUnavailableGitForGlobalHTTPVersion(t *testing.T) {
	tests := []struct {
		name         string
		unexecutable bool
	}{
		{name: "unavailable"},
		{name: "unexecutable", unexecutable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitDir := t.TempDir()
			if tt.unexecutable {
				gitPath := filepath.Join(gitDir, "git")
				if err := os.WriteFile(gitPath, []byte("#!/bin/sh\necho should-not-run\n"), 0o600); err != nil {
					t.Fatalf("write unexecutable git: %v", err)
				}
			}
			t.Setenv("PATH", gitDir)
			base := []string{"PATH=" + gitDir, "HOME=" + t.TempDir()}
			environments := map[string][]string{
				"anonymous": AnonymousGitEnv(base),
				"github":    GitHubGitEnv(base, nil),
				"forgejo":   ForgejoGitEnv(base, ""),
			}
			for name, env := range environments {
				t.Run(name, func(t *testing.T) {
					if env == nil {
						t.Fatal("controlled environment is nil")
					}
					configs := gitConfigPairs(t, env)
					if _, ok := configs[globalHTTPVersionKey]; ok {
						t.Fatalf("unavailable Git injected http.version: %#v", configs)
					}
				})
			}
		})
	}
}

func writeGlobalGitConfig(t *testing.T, home, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write global git config: %v", err)
	}
}

func TestGitHubGitEnvInjectsHTTPSProxy(t *testing.T) {
	env := GitHubGitEnv(
		[]string{"PATH=/bin", "HTTPS_PROXY=http://proxy.invalid:7890"},
		nil,
	)
	configs := gitConfigPairs(t, env)
	if got := configs["http.proxy"]; got != "http://proxy.invalid:7890" {
		t.Fatalf("http.proxy = %q, want HTTPS proxy", got)
	}
}

func TestGitHubGitEnvClearsAmbientCredentialConfiguration(t *testing.T) {
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
	env := GitHubGitEnv(base, nil)
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
	home := t.TempDir()
	writeGlobalGitConfig(t, home, "[http]\n\tversion = HTTP/1.1\n")
	proxy := "http://proxy.invalid:7890"
	base := []string{
		"PATH=/bin",
		"HOME=" + home,
		"HTTPS_PROXY=" + proxy,
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
		"github": GitHubGitEnv(base, []GitHubCredential{
			{Host: "github.com", Path: "tta-lab/example.git", Token: "secret"},
		}),
		"forgejo": ForgejoGitEnv(base, "secret"),
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
			if configs[globalHTTPVersionKey] != "HTTP/1.1" {
				t.Fatalf("controlled git configs = %#v, want supported global http.version", configs)
			}
			if _, disabled := configs["core.hooksPath"]; disabled || configs["http.sslVerify"] != "true" {
				t.Fatalf("controlled git configs = %#v, want normal hooks and TLS verification enabled", configs)
			}
			if envValue(env, "HTTPS_PROXY") != proxy {
				t.Fatalf("controlled Git environment lost explicit proxy: %v", env)
			}
			if name == "github" {
				if configs["http.proxy"] != proxy {
					t.Fatalf("GitHub controlled git configs = %#v, want explicit proxy", configs)
				}
			}
		})
	}
}

func TestGitHubCredentialHelperRestrictsRepository(t *testing.T) {
	home := t.TempDir()
	env := GitHubGitEnv(
		[]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home},
		[]GitHubCredential{{Host: "github.com", Path: "owner/repo.git", Token: "fixture-secret"}},
	)
	for _, tc := range []struct {
		protocol, host, path string
		allowed              bool
	}{
		{"https", "github.com", "owner/repo.git", true},
		{"https", "github.com", "other/repo.git", false},
		{"https", "evil.example", "owner/repo.git", false},
		{"http", "github.com", "owner/repo.git", false},
	} {
		cmd := exec.Command("git", "credential", "fill")
		cmd.Env = env
		cmd.Stdin = strings.NewReader("protocol=" + tc.protocol + "\nhost=" + tc.host + "\npath=" + tc.path + "\n\n")
		output, _ := cmd.Output()
		if got := strings.Contains(string(output), "password=fixture-secret"); got != tc.allowed {
			t.Fatalf("target %s %s %s allowed=%v", tc.protocol, tc.host, tc.path, got)
		}
	}
}

func TestGitHubCredentialHelperOverridesCheckoutHelper(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	init := exec.Command("git", "init", "-q", repo)
	init.Env = AnonymousGitEnv([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home})
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	marker := filepath.Join(home, "checkout-helper")
	localConfig := exec.Command("git", "-C", repo, "config", "--local",
		"credential.https://github.com.helper", "!touch "+marker)
	localConfig.Env = AnonymousGitEnv([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home})
	if output, err := localConfig.CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, output)
	}
	cmd := exec.Command("git", "-C", repo, "credential", "fill")
	cmd.Env = GitHubGitEnv([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home},
		[]GitHubCredential{{Host: "github.com", Path: "owner/repo.git", Token: "fixture-secret"}})
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=owner/repo.git\n\n")
	output, err := cmd.Output()
	if err != nil || !strings.Contains(string(output), "password=fixture-secret") {
		t.Fatalf("credential fill: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("checkout helper invoked: %v", err)
	}
}
