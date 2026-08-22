package gitprovider

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	urlpkg "net/url"
)

const remoteTimeout = 10 * time.Second

const (
	httpScheme  = "http"
	httpsScheme = "https"
)

type RepoInfo struct {
	Owner         string
	Repo          string
	Provider      ProviderType
	Host          string
	BaseURL       string
	CanonicalURL  string
	DefaultBranch string
}

// baseWebURL returns the base web URL for the hosting provider.
func (r *RepoInfo) baseWebURL() string {
	if r.Provider == ProviderGitHub {
		return "https://github.com"
	}
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return "https://" + r.Host
}

// WebURL constructs the base web URL for the repository.
func (r *RepoInfo) WebURL() string {
	return fmt.Sprintf("%s/%s/%s", r.baseWebURL(), r.Owner, r.Repo)
}

// PRURL constructs the full web URL for a pull request.
func (r *RepoInfo) PRURL(prID string) string {
	prSegment := "pull"
	if r.Provider != ProviderGitHub {
		prSegment = "pulls"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", r.baseWebURL(), r.Owner, r.Repo, prSegment, prID)
}

func DetectProvider(workDir string) (*RepoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git remote URL: %w", err)
	}

	remoteURL := strings.TrimSpace(string(out))
	info, err := ParseRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}

	info.DefaultBranch = detectDefaultBranch(workDir)
	return info, nil
}

func detectDefaultBranch(workDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}

	ref := strings.TrimSpace(string(out))
	const prefix = "refs/remotes/origin/"
	if !strings.HasPrefix(ref, prefix) {
		return "main"
	}
	return ref[len(prefix):]
}

func ParseRemoteURL(remoteURL string) (*RepoInfo, error) {
	if strings.HasPrefix(remoteURL, "git@") {
		return parseSSHShorthand(remoteURL)
	}
	if strings.HasPrefix(remoteURL, "ssh://") ||
		strings.HasPrefix(remoteURL, "http://") ||
		strings.HasPrefix(remoteURL, "https://") {
		return parseURL(remoteURL)
	}
	return nil, fmt.Errorf("could not parse git remote URL: %s", remoteURL)
}

// ParseHTTPRemoteURL parses an HTTP(S) repository URL and returns its canonical
// secret-free repository identity. It accepts harmless spelling differences,
// such as hostname case, a missing .git suffix, or a default port.
func ParseHTTPRemoteURL(raw string) (*RepoInfo, error) {
	u, scheme, host, hostPort, err := parseHTTPRepositoryURL(raw)
	if err != nil {
		return nil, err
	}
	owner, repo, err := parseHTTPRepositoryPath(u)
	if err != nil {
		return nil, err
	}
	if host == "github.com" {
		owner = strings.ToLower(owner)
		repo = strings.ToLower(repo)
	}
	canonical := scheme + "://" + hostPort + "/" + owner + "/" + repo + ".git"
	return &RepoInfo{
		Owner:        owner,
		Repo:         repo,
		Provider:     detectProviderFromHost(host),
		Host:         host,
		BaseURL:      scheme + "://" + hostPort,
		CanonicalURL: canonical,
	}, nil
}

func parseHTTPRepositoryURL(raw string) (*urlpkg.URL, string, string, string, error) {
	u, err := urlpkg.Parse(raw)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("invalid repository URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != httpScheme && scheme != httpsScheme {
		return nil, "", "", "", fmt.Errorf("repository URL must use HTTP or HTTPS")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", "", "", fmt.Errorf(
			"repository URL must not contain credentials, query, or fragment",
		)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, "", "", "", fmt.Errorf("repository URL must contain a hostname")
	}
	port := u.Port()
	if (scheme == httpsScheme && port == "443") || (scheme == httpScheme && port == "80") {
		port = ""
	}
	hostPort := host
	if port != "" {
		hostPort = net.JoinHostPort(host, port)
	}
	return u, scheme, host, hostPort, nil
}

func parseHTTPRepositoryPath(u *urlpkg.URL) (string, string, error) {
	escapedPath := strings.ToLower(u.EscapedPath())
	if strings.Contains(escapedPath, "%2f") || strings.Contains(escapedPath, "%5c") {
		return "", "", fmt.Errorf("repository URL path must not contain encoded separators")
	}
	path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository path %q: expected owner/repo", path)
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if owner == "." || owner == ".." || repo == "" || repo == "." || repo == ".." {
		return "", "", fmt.Errorf("invalid repository path %q: expected owner/repo", path)
	}
	return owner, repo, nil
}

func parseSSHShorthand(url string) (*RepoInfo, error) {
	colonIdx := strings.Index(url, ":")
	if colonIdx == -1 {
		return nil, fmt.Errorf("invalid SSH shorthand URL: %s", url)
	}
	host := url[4:colonIdx]
	path := url[colonIdx+1:]

	owner, repo, err := splitPath(path)
	if err != nil {
		return nil, err
	}
	return &RepoInfo{
		Owner:    owner,
		Repo:     repo,
		Provider: detectProviderFromHost(host),
		Host:     host,
	}, nil
}

func parseURL(raw string) (*RepoInfo, error) {
	u, err := urlpkg.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if (u.Scheme == "http" || u.Scheme == "https") &&
		(u.User != nil || u.RawQuery != "" || u.Fragment != "") {
		return nil, fmt.Errorf("HTTP(S) repository URL must not contain credentials, query, or fragment")
	}
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
		return ParseHTTPRemoteURL(raw)
	}

	host := u.Hostname()
	if host == "" {
		host = u.Host
	}

	owner, repo, err := splitPath(strings.TrimPrefix(u.Path, "/"))
	if err != nil {
		return nil, err
	}
	baseURL := ""
	if u.Scheme == "http" || u.Scheme == "https" {
		baseURL = u.Scheme + "://" + u.Host
	}
	return &RepoInfo{
		Owner:    owner,
		Repo:     repo,
		Provider: detectProviderFromHost(host),
		Host:     host,
		BaseURL:  baseURL,
	}, nil
}

func splitPath(path string) (owner, repo string, err error) {
	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository path: %s (expected owner/repo)", path)
	}
	return parts[0], parts[1], nil
}

func detectProviderFromHost(host string) ProviderType {
	host = strings.ToLower(host)
	if host == "github.com" || strings.HasSuffix(host, ".github.com") {
		return ProviderGitHub
	}
	return ProviderGeneric
}

// NewProviderByNameWithToken creates a provider by name with an optional GitHub token override.
// Forgejo ignores the githubToken parameter.
func NewProviderByNameWithToken(name, githubToken, host string) (Provider, error) {
	switch ProviderType(name) {
	case ProviderForgejo:
		return NewForgejoProvider(context.Background(), host)
	case ProviderGitHub:
		return NewGitHubProviderWithToken(context.Background(), githubToken)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// NewProviderWithToken creates a provider from RepoInfo with an optional GitHub token override.
// Forgejo ignores the githubToken parameter.
func NewProviderWithToken(info *RepoInfo, githubToken string) (Provider, error) {
	switch info.Provider {
	case ProviderGitHub:
		return NewGitHubProviderWithToken(context.Background(), githubToken)
	case ProviderForgejo:
		return NewForgejoProvider(context.Background(), info.baseWebURL())
	default:
		return nil, fmt.Errorf("unsupported provider: %s", info.Provider)
	}
}
