package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v88/github"
)

const (
	githubHTTPTimeout = 30 * time.Second
	tokenRefreshLead  = time.Minute
)

// Purpose identifies the minimum permission set needed by an operation.
type Purpose string

const (
	PurposeAPI      Purpose = "pr-api"
	PurposeGitRead  Purpose = "git-read"
	PurposeGitWrite Purpose = "git-write"
)

// InstallationStatus is non-secret installation metadata for one repository.
type InstallationStatus struct {
	AppID          int64
	InstallationID int64
	Repository     string
	Permissions    map[string]string
}

// CredentialBroker provides repository- and purpose-scoped App credentials.
type CredentialBroker interface {
	Token(ctx context.Context, owner, repo string, purpose Purpose) (string, error)
	Status(ctx context.Context, owner, repo string) (InstallationStatus, error)
	Invalidate(owner, repo string, purpose Purpose, failedToken string) error
}

type brokerOptions struct {
	baseURL   string
	transport http.RoundTripper
	now       func() time.Time
}

// BrokerOption configures internal broker dependencies.
type BrokerOption func(*brokerOptions)

// WithAPIBaseURL overrides GitHub's API URL for tests.
func WithAPIBaseURL(baseURL string) BrokerOption {
	return func(options *brokerOptions) { options.baseURL = baseURL }
}

// WithHTTPTransport overrides the GitHub HTTP transport for tests.
func WithHTTPTransport(transport http.RoundTripper) BrokerOption {
	return func(options *brokerOptions) { options.transport = transport }
}

type installationRecord struct {
	id int64
}

type tokenRecord struct {
	value     string
	expiresAt time.Time
}

type broker struct {
	cfg    Config
	client *github.Client
	now    func() time.Time

	mu            sync.Mutex
	installations map[string]installationRecord
	tokens        map[string]tokenRecord
}

// NewBroker builds a broker and validates the configured App private key.
func NewBroker(cfg Config, keySource KeySource, options ...BrokerOption) (CredentialBroker, error) {
	key, err := keySource.PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub App private key: %w", err)
	}
	if key == nil {
		return nil, fmt.Errorf("invalid GitHub App private key: key source returned no key")
	}

	opts := brokerOptions{
		baseURL:   "https://api.github.com/",
		transport: http.DefaultTransport,
		now:       time.Now,
	}
	for _, option := range options {
		option(&opts)
	}
	if !strings.HasSuffix(opts.baseURL, "/") {
		opts.baseURL += "/"
	}

	appTransport := ghinstallation.NewAppsTransportFromPrivateKey(opts.transport, cfg.AppID, key)
	appTransport.BaseURL = strings.TrimSuffix(opts.baseURL, "/")
	client, err := github.NewClient(
		github.WithTransport(appTransport),
		github.WithTimeout(githubHTTPTimeout),
		github.WithURLs(&opts.baseURL, &opts.baseURL),
	)
	if err != nil {
		return nil, fmt.Errorf("create GitHub App client: %w", err)
	}

	return &broker{
		cfg:           cfg,
		client:        client,
		now:           opts.now,
		installations: make(map[string]installationRecord),
		tokens:        make(map[string]tokenRecord),
	}, nil
}

func (b *broker) Token(ctx context.Context, owner, repo string, purpose Purpose) (string, error) {
	if err := b.cfg.RequireOwner(owner); err != nil {
		return "", err
	}
	permissions, err := permissionsForPurpose(purpose)
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	cacheKey := tokenCacheKey(owner, repo, purpose)
	if cached, ok := b.tokens[cacheKey]; ok && b.now().Add(tokenRefreshLead).Before(cached.expiresAt) {
		return cached.value, nil
	}

	installation, err := b.installationLocked(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	token, _, err := b.client.Apps.CreateInstallationToken(ctx, installation.id, &github.InstallationTokenOptions{
		Repositories: []string{repo},
		Permissions:  permissions,
	})
	if err != nil {
		return "", classifyGitHubError("mint installation token", owner, repo, err)
	}
	if token.GetToken() == "" || token.ExpiresAt == nil {
		return "", fmt.Errorf("GitHub returned an incomplete installation token for %s/%s", owner, repo)
	}
	record := tokenRecord{value: token.GetToken(), expiresAt: token.GetExpiresAt().Time}
	b.tokens[cacheKey] = record
	return record.value, nil
}

func (b *broker) Status(ctx context.Context, owner, repo string) (InstallationStatus, error) {
	if err := b.cfg.RequireOwner(owner); err != nil {
		return InstallationStatus{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	installation, err := b.fetchInstallation(ctx, owner, repo)
	if err != nil {
		return InstallationStatus{}, err
	}
	b.installations[repositoryCacheKey(owner, repo)] = installationRecord{id: installation.GetID()}
	return InstallationStatus{
		AppID:          b.cfg.AppID,
		InstallationID: installation.GetID(),
		Repository:     owner + "/" + repo,
		Permissions:    installationPermissions(installation.GetPermissions()),
	}, nil
}

func (b *broker) Invalidate(owner, repo string, purpose Purpose, failedToken string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cacheKey := tokenCacheKey(owner, repo, purpose)
	cached, ok := b.tokens[cacheKey]
	if !ok || cached.value != failedToken {
		return fmt.Errorf(
			"GitHub App authentication failed for %s/%s; failed credential was already replaced",
			owner, repo,
		)
	}
	delete(b.tokens, cacheKey)
	delete(b.installations, repositoryCacheKey(owner, repo))
	return fmt.Errorf("GitHub App authentication failed for %s/%s; cached credential invalidated", owner, repo)
}

func (b *broker) installationLocked(ctx context.Context, owner, repo string) (installationRecord, error) {
	cacheKey := repositoryCacheKey(owner, repo)
	if cached, ok := b.installations[cacheKey]; ok {
		return cached, nil
	}
	installation, err := b.fetchInstallation(ctx, owner, repo)
	if err != nil {
		return installationRecord{}, err
	}
	record := installationRecord{id: installation.GetID()}
	b.installations[cacheKey] = record
	return record, nil
}

func (b *broker) fetchInstallation(ctx context.Context, owner, repo string) (*github.Installation, error) {
	installation, _, err := b.client.Apps.GetRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		return nil, classifyGitHubError("discover installation", owner, repo, err)
	}
	if installation.GetID() <= 0 {
		return nil, fmt.Errorf("GitHub returned an invalid installation for %s/%s", owner, repo)
	}
	return installation, nil
}

func permissionsForPurpose(purpose Purpose) (*github.InstallationPermissions, error) {
	switch purpose {
	case PurposeAPI:
		return &github.InstallationPermissions{
			Actions:      github.Ptr("read"),
			Checks:       github.Ptr("read"),
			Contents:     github.Ptr("read"),
			PullRequests: github.Ptr("write"),
		}, nil
	case PurposeGitRead:
		return &github.InstallationPermissions{Contents: github.Ptr("read")}, nil
	case PurposeGitWrite:
		return &github.InstallationPermissions{
			Contents:  github.Ptr("write"),
			Workflows: github.Ptr("write"),
		}, nil
	default:
		return nil, fmt.Errorf("unknown GitHub App token purpose %q", purpose)
	}
}

func installationPermissions(permissions *github.InstallationPermissions) map[string]string {
	if permissions == nil {
		return map[string]string{}
	}
	return map[string]string{
		"actions":       permissions.GetActions(),
		"checks":        permissions.GetChecks(),
		"contents":      permissions.GetContents(),
		"pull_requests": permissions.GetPullRequests(),
		"workflows":     permissions.GetWorkflows(),
	}
}

func classifyGitHubError(operation, owner, repo string, err error) error {
	var responseError *github.ErrorResponse
	if !errors.As(err, &responseError) || responseError.Response == nil {
		return fmt.Errorf("transient GitHub failure while attempting to %s for %s/%s", operation, owner, repo)
	}
	status := responseError.Response.StatusCode
	switch {
	case operation == "discover installation" && status == http.StatusNotFound:
		return fmt.Errorf("GitHub App is not installed on or cannot access %s/%s", owner, repo)
	case status == http.StatusUnauthorized:
		return fmt.Errorf(
			"GitHub App JWT authentication failed for %s/%s (HTTP 401); check the private key and system clock",
			owner, repo,
		)
	case status == http.StatusForbidden || status == http.StatusUnprocessableEntity:
		return fmt.Errorf(
			"GitHub App installation lacks permission to %s for %s/%s (HTTP %d)",
			operation, owner, repo, status,
		)
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return fmt.Errorf(
			"transient GitHub failure while attempting to %s for %s/%s (HTTP %d)",
			operation, owner, repo, status,
		)
	default:
		return fmt.Errorf("GitHub rejected %s for %s/%s (HTTP %d)", operation, owner, repo, status)
	}
}

func repositoryCacheKey(owner, repo string) string {
	return strings.ToLower(owner) + "/" + strings.ToLower(repo)
}

func tokenCacheKey(owner, repo string, purpose Purpose) string {
	return repositoryCacheKey(owner, repo) + ":" + string(purpose)
}
