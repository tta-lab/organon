package og

import (
	"fmt"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func createPR(ctx *repoContext, title, body string) (*PullRequest, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	pr, err := provider.CreatePR(ctx.Owner, ctx.Repo, ctx.Branch, ctx.DefaultBase, title, body)
	if err != nil {
		return nil, err
	}
	return fromProviderPR(pr), nil
}

func findPR(ctx *repoContext, state string) (*PullRequest, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	if state == "" {
		state = "open"
	}
	pr, err := provider.FindPRByState(ctx.Owner, ctx.Repo, ctx.Branch, ctx.DefaultBase, state)
	if err != nil {
		return nil, err
	}
	return fromProviderPR(pr), nil
}

func getPR(ctx *repoContext, index int64) (*PullRequest, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	pr, err := provider.GetPR(ctx.Owner, ctx.Repo, index)
	if err != nil {
		return nil, err
	}
	return fromProviderPR(pr), nil
}

func updatePR(ctx *repoContext, index int64, title, body string) (*PullRequest, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	pr, err := provider.EditPR(ctx.Owner, ctx.Repo, index, title, body)
	if err != nil {
		return nil, err
	}
	return fromProviderPR(pr), nil
}

func commentPR(ctx *repoContext, index int64, body string) error {
	provider, err := newProvider(ctx)
	if err != nil {
		return err
	}
	_, err = provider.CreateComment(ctx.Owner, ctx.Repo, index, body)
	return err
}

func getChecks(ctx *repoContext, pr *PullRequest) ([]string, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	sha := pr.SHA
	if sha == "" {
		sha = "HEAD"
	}
	status, err := provider.GetCombinedStatus(ctx.Owner, ctx.Repo, sha)
	if err != nil {
		return nil, err
	}
	lines := []string{"combined: " + status.State}
	for _, s := range status.Statuses {
		lines = append(lines, fmt.Sprintf("%s: %s - %s", s.Context, s.State, s.Description))
	}
	return lines, nil
}

func newProvider(ctx *repoContext) (gitprovider.Provider, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	if ctx.Provider == gitprovider.ProviderGitHub {
		return gitprovider.NewGitHubProviderWithToken(ctx.Token)
	}
	return gitprovider.NewForgejoProviderWithToken(ctx.Host, ctx.Token)
}

func fromProviderPR(pr *gitprovider.PullRequest) *PullRequest {
	if pr == nil {
		return nil
	}
	return &PullRequest{
		Index:   pr.Index,
		Number:  pr.Index,
		Title:   pr.Title,
		State:   pr.State,
		Merged:  pr.Merged,
		URL:     pr.HTMLURL,
		HTMLURL: pr.HTMLURL,
		Head:    pr.Head,
		Base:    pr.Base,
		Body:    pr.Body,
		SHA:     pr.HeadSHA,
	}
}

func requireToken(ctx *repoContext) error {
	if ctx.Token == "" {
		return fmt.Errorf("missing token: set %s", ctx.TokenEnv)
	}
	return nil
}
