package og

import (
	"context"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

type brokerTokenCall struct {
	owner   string
	repo    string
	purpose githubapp.Purpose
}

type recordingBroker struct {
	tokenCalls      []brokerTokenCall
	token           string
	tokenErr        error
	invalidations   []brokerInvalidation
	invalidationErr error
	status          githubapp.InstallationStatus
	statusErr       error
}

type brokerInvalidation struct {
	owner, repo string
	purpose     githubapp.Purpose
	token       string
}

func (b *recordingBroker) Token(_ context.Context, owner, repo string, purpose githubapp.Purpose) (string, error) {
	b.tokenCalls = append(b.tokenCalls, brokerTokenCall{owner: owner, repo: repo, purpose: purpose})
	if b.tokenErr != nil {
		return "", b.tokenErr
	}
	if b.token != "" {
		return b.token, nil
	}
	return "app-installation-token", nil
}

func (b *recordingBroker) Status(context.Context, string, string) (githubapp.InstallationStatus, error) {
	return b.status, b.statusErr
}

func (b *recordingBroker) Invalidate(owner, repo string, purpose githubapp.Purpose, token string) error {
	b.invalidations = append(b.invalidations, brokerInvalidation{
		owner: owner, repo: repo, purpose: purpose, token: token,
	})
	return b.invalidationErr
}

func TestNewProviderRequiresGitHubAppBroker(t *testing.T) {
	_, err := newProvider(&repoContext{
		Provider: gitprovider.ProviderGitHub,
		Owner:    "tta-lab",
		Repo:     "organon",
		TokenEnv: "GITHUB_TOKEN",
		Token:    "ambient-pat",
	})
	if err == nil || !strings.Contains(err.Error(), "GitHub App authentication is not configured") {
		t.Fatalf("newProvider error = %v", err)
	}
}

func TestNewProviderUsesRepositoryScopedAPIToken(t *testing.T) {
	broker := &recordingBroker{}
	provider, err := newProvider(&repoContext{
		Provider:     gitprovider.ProviderGitHub,
		Owner:        "tta-lab",
		Repo:         "organon",
		TokenEnv:     "GITHUB_TOKEN",
		Token:        "ambient-pat",
		githubBroker: broker,
	})
	if err != nil {
		t.Fatalf("newProvider: %v", err)
	}
	if provider.Name() != "github" {
		t.Fatalf("provider = %q, want github", provider.Name())
	}
	want := brokerTokenCall{owner: "tta-lab", repo: "organon", purpose: githubapp.PurposeAPI}
	if len(broker.tokenCalls) != 1 || broker.tokenCalls[0] != want {
		t.Fatalf("broker calls = %+v, want [%+v]", broker.tokenCalls, want)
	}
}
