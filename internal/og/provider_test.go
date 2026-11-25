package og

import (
	"context"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitcredential"
	"github.com/tta-lab/organon/internal/gitprovider"
)

type brokerTokenCall struct {
	owner   string
	repo    string
	purpose gitcredential.Purpose
}

type recordingBroker struct {
	tokenCalls []brokerTokenCall
	token      string
	tokenErr   error
}

func (b *recordingBroker) Token(
	_ context.Context, owner, repo, _ string, purpose gitcredential.Purpose,
) (string, error) {
	b.tokenCalls = append(b.tokenCalls, brokerTokenCall{owner: owner, repo: repo, purpose: purpose})
	if b.tokenErr != nil {
		return "", b.tokenErr
	}
	if b.token != "" {
		return b.token, nil
	}
	return "fixture-token", nil
}

func TestNewProviderRequiresGitCredential(t *testing.T) {
	_, err := newProvider(&repoContext{
		Provider: gitprovider.ProviderGitHub,
		Owner:    "tta-lab",
		Repo:     "organon",
		TokenEnv: "GITHUB_TOKEN",
		Token:    "ambient-pat",
	})
	if err == nil || !strings.Contains(err.Error(),
		"GitHub credential unavailable") {
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
	want := brokerTokenCall{owner: "tta-lab", repo: "organon", purpose: gitcredential.PurposeAPI}
	if len(broker.tokenCalls) != 1 || broker.tokenCalls[0] != want {
		t.Fatalf("broker calls = %+v, want [%+v]", broker.tokenCalls, want)
	}
}
