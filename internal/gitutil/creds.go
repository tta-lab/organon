package gitutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	credentialHelperConfig = "credential.helper"
	forgejoTokenEnv        = "FORGEJO_TOKEN"
)

// GitCredEnvWithToken returns environment variables for git network operations
// using an already-resolved token. Use this when a caller has a single
// credential source of truth and must not re-resolve from ambient env vars.
func GitCredEnvWithToken(token string) []string {
	// Always suppress interactive prompts; this prevents the hang bug
	// even when no token is configured.
	env := make([]string, 0, 7)
	env = append(env, "GIT_TERMINAL_PROMPT=0")

	if token == "" {
		return env
	}

	return append(env,
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential.helper",
		"GIT_CONFIG_VALUE_1=!f(){ echo username=x-access-token; echo password=$GIT_TOKEN_INJECT; }; f",
		"GIT_TOKEN_INJECT="+token,
	)
}

// AnonymousGitEnv returns a complete child environment with credential sources disabled.
func AnonymousGitEnv(baseEnv []string) []string {
	clean := controlledCredentialEnv(baseEnv)
	return append(clean,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=core.askPass",
		"GIT_CONFIG_VALUE_1=",
		"GIT_CONFIG_KEY_2=http.sslVerify",
		"GIT_CONFIG_VALUE_2=true",
	)
}

// ForgejoGitEnv returns a complete child environment using only the resolved token.
func ForgejoGitEnv(baseEnv []string, token string) []string {
	env := controlledCredentialEnv(baseEnv)
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=4",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=core.askPass",
		"GIT_CONFIG_VALUE_1=",
		"GIT_CONFIG_KEY_2=credential.helper",
		"GIT_CONFIG_VALUE_2=!f(){ echo username=x-access-token; echo password=$GIT_TOKEN_INJECT; }; f",
		"GIT_CONFIG_KEY_3=http.sslVerify",
		"GIT_CONFIG_VALUE_3=true",
	)
	if token != "" {
		env = append(env, "GIT_TOKEN_INJECT="+token)
	}
	return env
}

// GitHubAppGitEnv returns a complete child-process environment that routes a
// GitHub origin through canonical HTTPS and uses only the supplied App token.
func GitHubAppGitEnv(baseEnv []string, remoteURL, owner, repo, token string) []string {
	env := controlledCredentialEnv(baseEnv)
	env = append(env, "GIT_TERMINAL_PROMPT=0")

	type configPair struct{ key, value string }
	configs := []configPair{
		{key: credentialHelperConfig, value: ""},
		{key: "core.askPass", value: ""},
		{key: "http.sslVerify", value: "true"},
	}
	if proxy := gitProxyFromEnvironment(baseEnv); proxy != "" {
		configs = append(configs, configPair{key: "http.proxy", value: proxy})
	}
	if token != "" {
		configs = append(configs, configPair{
			key:   credentialHelperConfig,
			value: "!f(){ echo username=x-access-token; echo password=$GIT_TOKEN_INJECT; }; f",
		})
	}
	canonicalURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	if remoteURL != canonicalURL {
		configs = append(configs, configPair{key: "url." + canonicalURL + ".insteadOf", value: remoteURL})
	}
	env = append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(configs)))
	for i, config := range configs {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, config.key),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, config.value),
		)
	}
	if token != "" {
		env = append(env, "GIT_TOKEN_INJECT="+token)
	}
	return env
}

func gitProxyFromEnvironment(baseEnv []string) string {
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy"} {
		for i := len(baseEnv) - 1; i >= 0; i-- {
			candidateName, value, _ := strings.Cut(baseEnv[i], "=")
			if candidateName == name && value != "" {
				return value
			}
		}
	}
	return ""
}

func filterControlledGitEnv(baseEnv []string) []string {
	filtered := make([]string, 0, len(baseEnv))
	for _, entry := range baseEnv {
		name, _, _ := strings.Cut(entry, "=")
		if controlledGitEnvironmentName(name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func controlledGitEnvironmentName(name string) bool {
	return strings.HasPrefix(name, "GIT_") || name == "SSH_ASKPASS" || name == "GCM_INTERACTIVE"
}

func controlledCredentialEnv(baseEnv []string) []string {
	return append(filterCredentialEnv(baseEnv),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func filterCredentialEnv(baseEnv []string) []string {
	filtered := filterControlledGitEnv(baseEnv)
	clean := filtered[:0]
	for _, entry := range filtered {
		name, _, _ := strings.Cut(entry, "=")
		if sensitiveEnvironmentName(name) {
			continue
		}
		switch name {
		case "GITHUB_TOKEN", "GH_TOKEN", forgejoTokenEnv, "FORGEJO_ACCESS_TOKEN", "GITEA_TOKEN":
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}

func sensitiveEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{
		"TOKEN", "SECRET", "PASSWORD", "API_KEY", "ACCESS_KEY", "PRIVATE_KEY", "CREDENTIAL",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return upper == "SSH_AUTH_SOCK"
}

// RemoteURL returns the origin remote URL for the given directory.
func RemoteURL(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ForgeTokenEnv returns the first configured token environment variable for
// Forgejo/Gitea-compatible remotes.
func ForgeTokenEnv() string {
	for _, name := range []string{forgejoTokenEnv, "FORGEJO_ACCESS_TOKEN", "GITEA_TOKEN"} {
		if os.Getenv(name) != "" {
			return name
		}
	}
	return forgejoTokenEnv
}

// ForgeToken returns the configured token for Forgejo/Gitea-compatible remotes.
func ForgeToken() string {
	return os.Getenv(ForgeTokenEnv())
}
