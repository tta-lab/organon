package og

import (
	"context"
	"fmt"

	"github.com/tta-lab/organon/internal/githubapp"
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
	if !validProviderPRIdentity(pr, 0) {
		return nil, fmt.Errorf("provider returned invalid PR after creating it")
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
	if ctx.Provider == gitprovider.ProviderGitHub {
		if finder, ok := provider.(commitPRFinder); ok {
			sha, err := gitOutput(ctx.WorkDir, "rev-parse", "HEAD")
			if err != nil {
				return nil, fmt.Errorf("get current HEAD SHA: %w", err)
			}
			if sha == "" {
				return nil, fmt.Errorf("get current HEAD SHA: empty result")
			}
			pr, err := finder.FindPRByCommit(ctx.Owner, ctx.Repo, sha)
			if err == nil && pr != nil && pr.Head == ctx.Branch && prMatches(pr, ctx.DefaultBase, state) {
				if !validProviderPRIdentity(pr, 0) {
					return nil, fmt.Errorf("provider returned invalid PR while finding by commit")
				}
				return fromProviderPR(pr), nil
			}
		}
	}
	pr, err := provider.FindPRByState(ctx.Owner, ctx.Repo, ctx.Branch, ctx.DefaultBase, state)
	if err != nil {
		return nil, err
	}
	if !validProviderPRIdentity(pr, 0) {
		return nil, fmt.Errorf("provider returned invalid PR while finding by branch")
	}
	return fromProviderPR(pr), nil
}

type commitPRFinder interface {
	FindPRByCommit(owner, repo, sha string) (*gitprovider.PullRequest, error)
}

func prMatches(pr *gitprovider.PullRequest, base, state string) bool {
	if pr.Base != base {
		return false
	}
	return state == "" || state == "all" || pr.State == state
}

func validProviderPRIdentity(pr *gitprovider.PullRequest, expectedIndex int64) bool {
	return pr != nil && pr.Index > 0 && (expectedIndex <= 0 || pr.Index == expectedIndex)
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
	if !validProviderPRIdentity(pr, index) {
		return nil, fmt.Errorf("provider returned invalid PR snapshot for #%d", index)
	}
	return fromProviderPR(pr), nil
}

func updatePR(ctx *repoContext, index int64, title, body *string) (*PullRequest, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	current, err := provider.GetPR(ctx.Owner, ctx.Repo, index)
	if err != nil {
		return nil, err
	}
	if !validProviderPRIdentity(current, index) {
		return nil, fmt.Errorf("provider returned invalid current PR ID for #%d", index)
	}
	desiredTitle, desiredBody := current.Title, current.Body
	if title != nil {
		desiredTitle = *title
	}
	if body != nil {
		desiredBody = *body
	}
	pr, err := provider.EditPR(ctx.Owner, ctx.Repo, index, desiredTitle, desiredBody)
	if err != nil {
		return nil, err
	}
	if !validProviderPRIdentity(pr, index) {
		return nil, fmt.Errorf("provider returned invalid PR ID after updating #%d", index)
	}
	if title != nil && pr.Title != *title {
		return nil, fmt.Errorf(
			"provider returned title %q after updating PR #%d, want %q", pr.Title, index, *title,
		)
	}
	if body != nil && pr.Body != *body {
		return nil, fmt.Errorf("provider returned body that does not match update for PR #%d", index)
	}
	return fromProviderPR(pr), nil
}

func commentPR(ctx *repoContext, index int64, body string) (*Comment, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	comment, err := provider.CreateComment(ctx.Owner, ctx.Repo, index, body)
	if err != nil {
		return nil, err
	}
	if comment == nil || comment.ID <= 0 || comment.PRID != index || comment.Body != body || comment.HTMLURL == "" {
		return nil, fmt.Errorf("provider returned invalid comment after commenting on PR #%d", index)
	}
	return &Comment{
		ID: comment.ID, PRID: index, Body: comment.Body, URL: comment.HTMLURL,
		User: comment.User, CreatedAt: comment.CreatedAt,
	}, nil
}

func getChecks(ctx *repoContext, pr *PullRequest) ([]string, error) {
	status, err := getCIStatus(ctx, pr)
	if err != nil {
		return nil, err
	}
	lines := []string{"combined: " + status.State}
	for _, s := range status.Statuses {
		lines = append(lines, fmt.Sprintf("%s: %s - %s", s.Context, s.State, s.Description))
	}
	return lines, nil
}

func getCIStatus(ctx *repoContext, pr *PullRequest) (*CIStatusResponse, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	sha := pr.SHA
	if sha == "" {
		sha = headRefName
	}
	status, err := provider.GetCombinedStatus(ctx.Owner, ctx.Repo, sha)
	if err != nil {
		return nil, err
	}
	statuses := make([]CIStatus, 0, len(status.Statuses))
	for _, s := range status.Statuses {
		statuses = append(statuses, CIStatus{
			Context:     s.Context,
			State:       s.State,
			Description: s.Description,
			TargetURL:   s.TargetURL,
		})
	}
	return &CIStatusResponse{OK: true, State: status.State, Statuses: statuses}, nil
}

func getCIFailures(ctx *repoContext, pr *PullRequest, tailLines int) ([]string, error) {
	failures, err := getCIFailureDetails(ctx, pr, tailLines)
	if err != nil {
		return nil, err
	}
	return formatCIFailureDetails(failures), nil
}

func getCIFailureDetails(ctx *repoContext, pr *PullRequest, tailLines int) ([]*gitprovider.JobFailure, error) {
	provider, err := newProvider(ctx)
	if err != nil {
		return nil, err
	}
	sha := pr.SHA
	if sha == "" {
		sha = headRefName
	}
	return provider.GetCIFailureDetails(ctx.Owner, ctx.Repo, sha, tailLines)
}

func formatCIFailureDetails(failures []*gitprovider.JobFailure) []string {
	lines := make([]string, 0, len(failures)*4)
	for _, failure := range failures {
		title := failure.JobName
		if failure.WorkflowName != "" {
			title = failure.WorkflowName + " / " + title
		}
		lines = append(lines, title)
		if failure.HTMLURL != "" {
			lines = append(lines, failure.HTMLURL)
		}
		if failure.LogTail != "" {
			lines = append(lines, failure.LogTail)
		}
	}
	return lines
}

var newProviderFunc = newProviderImpl

func newProvider(ctx *repoContext) (gitprovider.Provider, error) {
	return newProviderFunc(ctx)
}

func newProviderImpl(ctx *repoContext) (gitprovider.Provider, error) {
	if ctx.Provider == gitprovider.ProviderGitHub {
		if ctx.githubBroker == nil {
			return nil, fmt.Errorf("GitHub App authentication is not configured")
		}
		token, err := ctx.githubBroker.Token(context.Background(), ctx.Owner, ctx.Repo, githubapp.PurposeAPI)
		if err != nil {
			return nil, err
		}
		return gitprovider.NewGitHubProviderWithTokenAndAuthFailure(token, func() {
			_ = ctx.githubBroker.Invalidate(ctx.Owner, ctx.Repo, githubapp.PurposeAPI, token)
		})
	}
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	baseURL := ctx.BaseURL
	if baseURL == "" {
		baseURL = ctx.Host
	}
	return gitprovider.NewForgejoProviderWithToken(baseURL, ctx.Token)
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
