package gitcredential

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tta-lab/organon/internal/gitprovider"
)

type Purpose string

const (
	PurposeAPI        Purpose = "pr-api"
	PurposeIssueRead  Purpose = "issue-read"
	PurposeIssueWrite Purpose = "issue-write"
	PurposeGitRead    Purpose = "git-read"
	PurposeGitWrite   Purpose = "git-write"
)

var ErrMissing = errors.New("GitHub credential unavailable from Git credential helper")

type Broker interface {
	Token(context.Context, string, string, string, Purpose) (string, error)
}

type Git struct{}

// Token consults only system and user Git configuration, outside any checkout.
// Git's credential protocol and all helper output are kept out of errors.
func (Git) Token(ctx context.Context, owner, repo, remote string, _ Purpose) (string, error) {
	host, path, err := Context(remote, owner, repo)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "og-git-credential-*")
	if err != nil {
		return "", fmt.Errorf("prepare Git credential lookup: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	cmd := exec.CommandContext(ctx, "git", "-c", "core.askPass=", "credential", "fill")
	cmd.Dir = dir
	cmd.Env = lookupEnvironment(os.Environ())
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never", "GIT_DIR="+filepath.Join(dir, "empty.git"))
	cmd.Stdin = strings.NewReader("url=" + remote + "\n\n")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", ErrMissing
	}
	fields := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "=")
		if ok {
			fields[key] = value
		}
	}
	returnedPath, hasPath := fields["path"]
	if fields["protocol"] != "https" || fields["host"] != host || (hasPath && returnedPath != path) {
		return "", fmt.Errorf("Git credential helper returned an unexpected destination")
	}
	if fields["password"] == "" {
		return "", ErrMissing
	}
	return fields["password"], nil
}

// Context validates a GitHub URL against its canonical repository identity and
// returns the host and path spelling Git uses for credential helpers.
func Context(remote, owner, repo string) (string, string, error) {
	info, err := gitprovider.ParseHTTPRemoteURL(remote)
	if err != nil || info.BaseURL != "https://github.com" || info.Owner != owner || info.Repo != repo ||
		strings.ContainsAny(remote, "\x00\r\n") {
		return "", "", fmt.Errorf("invalid GitHub repository identity")
	}
	u, err := url.Parse(remote)
	if err != nil || strings.ContainsAny(u.Path, "\x00\r\n") {
		return "", "", fmt.Errorf("invalid GitHub repository identity")
	}
	return u.Host, strings.Trim(u.Path, "/"), nil
}

func lookupEnvironment(env []string) []string {
	var filtered []string
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "GIT_") || upper == "SSH_ASKPASS" || upper == "GCM_INTERACTIVE" ||
			strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") ||
			strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "CREDENTIAL") ||
			strings.Contains(upper, "API_KEY") || strings.Contains(upper, "ACCESS_KEY") ||
			strings.Contains(upper, "PRIVATE_KEY") || upper == "SSH_AUTH_SOCK" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
