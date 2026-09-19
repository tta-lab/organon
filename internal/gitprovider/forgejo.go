package gitprovider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	forgejo_sdk "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
)

type ForgejoProvider struct {
	client *forgejo_sdk.Client
	ctx    context.Context
}

const forgejoCombinedStatusShapeHeader = "X-Organon-Forgejo-Combined-Status-Shape"

func NewForgejoProvider(ctx context.Context, host string) (Provider, error) {
	token := os.Getenv("FORGEJO_TOKEN")
	if token == "" {
		token = os.Getenv("FORGEJO_ACCESS_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GITEA_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("FORGEJO_TOKEN, FORGEJO_ACCESS_TOKEN, or GITEA_TOKEN environment variable is required")
	}

	return NewForgejoProviderWithToken(ctx, host, token)
}

func NewForgejoProviderWithToken(ctx context.Context, host, token string) (Provider, error) {
	if host == "" {
		return nil, fmt.Errorf("host is required for Forgejo provider")
	}
	if token == "" {
		return nil, fmt.Errorf("forgejo token is required")
	}

	url := host
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + host
	}

	httpClient := newContextHTTPClient(ctx, nil)
	httpClient.Transport = forgejoCombinedStatusTransport{base: httpClient.Transport}
	client, err := forgejo_sdk.NewClient(
		url,
		forgejo_sdk.SetToken(token),
		forgejo_sdk.SetHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Forgejo client: %w", err)
	}

	return &ForgejoProvider{client: client, ctx: contextOrBackground(ctx)}, nil
}

func (p *ForgejoProvider) Name() string { return "forgejo" }

func (p *ForgejoProvider) CreatePR(owner, repo, head, base, title, body string) (*PullRequest, error) {
	pr, _, err := p.client.CreatePullRequest(owner, repo, forgejo_sdk.CreatePullRequestOption{
		Head:  head,
		Base:  base,
		Title: title,
		Body:  body,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return toPullRequest(pr), nil
}

func (p *ForgejoProvider) FindPR(owner, repo, head, base string) (*PullRequest, error) {
	return p.FindPRByState(owner, repo, head, base, "open")
}

func (p *ForgejoProvider) FindPRByState(owner, repo, head, base, state string) (*PullRequest, error) {
	pr, _, err := p.client.GetPullRequestByBaseAndHead(owner, repo, base, head)
	if err != nil {
		return nil, fmt.Errorf("failed to find PR for %s -> %s: %w", head, base, err)
	}
	result := toPullRequest(pr)
	if state == "" || state == "all" || result.State == state {
		return result, nil
	}
	return nil, fmt.Errorf("no %s PR found for %s -> %s", state, head, base)
}

func (p *ForgejoProvider) EditPR(owner, repo string, index int64, title, body string) (*PullRequest, error) {
	opt := forgejo_sdk.EditPullRequestOption{}
	if title != "" {
		opt.Title = title
	}
	if body != "" {
		opt.Body = body
	}

	pr, _, err := p.client.EditPullRequest(owner, repo, index, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to edit PR #%d: %w", index, err)
	}

	return toPullRequest(pr), nil
}

func (p *ForgejoProvider) GetPR(owner, repo string, index int64) (*PullRequest, error) {
	pr, _, err := p.client.GetPullRequest(owner, repo, index)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR #%d: %w", index, err)
	}

	return toPullRequest(pr), nil
}

// MergePullRequest performs a squash merge guarded by the expected head SHA.
// Organon's og service performs guarded checkout cleanup after the receipt is
// recorded, so the provider leaves branch deletion to that shared policy.
func (p *ForgejoProvider) MergePullRequest(owner, repo string, index int64, headSHA string) error {
	merged, _, err := p.client.MergePullRequest(owner, repo, index, forgejo_sdk.MergePullRequestOption{
		Style:                  forgejo_sdk.MergeStyleSquash,
		HeadCommitId:           headSHA,
		DeleteBranchAfterMerge: false,
		ForceMerge:             false,
	})
	if err != nil {
		return fmt.Errorf("failed to squash merge PR #%d: %w", index, err)
	}
	if !merged {
		return fmt.Errorf("failed to squash merge PR #%d: provider did not merge pull request", index)
	}
	return nil
}

func (p *ForgejoProvider) CreateComment(owner, repo string, index int64, body string) (*Comment, error) {
	comment, _, err := p.client.CreateIssueComment(owner, repo, index, forgejo_sdk.CreateIssueCommentOption{
		Body: body,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to comment on PR #%d: %w", index, err)
	}

	result := toComment(comment)
	result.PRID = index
	return result, nil
}

func (p *ForgejoProvider) ListComments(owner, repo string, index int64) ([]*Comment, error) {
	comments, _, err := p.client.ListIssueComments(owner, repo, index, forgejo_sdk.ListIssueCommentOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list comments on PR #%d: %w", index, err)
	}

	result := make([]*Comment, len(comments))
	for i, c := range comments {
		result[i] = toComment(c)
		result[i].PRID = index
	}
	return result, nil
}

func (p *ForgejoProvider) ListIssues(owner, repo, query, state string, page, perPage int) (*IssuePage, error) {
	options := forgejo_sdk.ListIssueOption{
		ListOptions: forgejo_sdk.ListOptions{Page: page, PageSize: perPage},
		State:       forgejo_sdk.StateType(state), Type: forgejo_sdk.IssueTypeIssue, KeyWord: query,
	}
	issues, response, err := p.client.ListRepoIssues(owner, repo, options)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	result := &IssuePage{Issues: make([]*Issue, 0, len(issues)), HasNext: response != nil && response.NextPage > 0}
	for _, i := range issues {
		result.Issues = append(result.Issues, toForgejoIssue(i))
	}
	return result, nil
}
func (p *ForgejoProvider) GetIssue(owner, repo string, index int64) (*Issue, error) {
	i, _, err := p.client.GetIssue(owner, repo, index)
	if err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", index, err)
	}
	return toForgejoIssue(i), nil
}
func (p *ForgejoProvider) CreateIssue(owner, repo, title, body string) (*Issue, error) {
	i, _, err := p.client.CreateIssue(owner, repo, forgejo_sdk.CreateIssueOption{Title: title, Body: body})
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	return toForgejoIssue(i), nil
}
func (p *ForgejoProvider) UpdateIssueTitle(owner, repo string, index int64, title string) (*Issue, error) {
	i, _, err := p.client.EditIssue(owner, repo, index, forgejo_sdk.EditIssueOption{Title: title})
	if err != nil {
		return nil, fmt.Errorf("update issue title #%d: %w", index, err)
	}
	return toForgejoIssue(i), nil
}
func (p *ForgejoProvider) ReplaceIssueBody(owner, repo string, index int64, body string) (*Issue, error) {
	i, _, err := p.client.EditIssue(owner, repo, index, forgejo_sdk.EditIssueOption{Body: &body})
	if err != nil {
		return nil, fmt.Errorf("replace issue body #%d: %w", index, err)
	}
	return toForgejoIssue(i), nil
}
func (p *ForgejoProvider) ListIssueComments(owner, repo string, index int64, page, perPage int) (*CommentPage, error) {
	options := forgejo_sdk.ListIssueCommentOptions{ListOptions: forgejo_sdk.ListOptions{Page: page, PageSize: perPage}}
	cs, r, err := p.client.ListIssueComments(owner, repo, index, options)
	if err != nil {
		return nil, fmt.Errorf("list issue comments #%d: %w", index, err)
	}
	out := &CommentPage{Comments: make([]*Comment, 0, len(cs)), HasNext: r != nil && r.NextPage > 0}
	for _, c := range cs {
		v := toComment(c)
		v.PRID = index
		out.Comments = append(out.Comments, v)
	}
	return out, nil
}
func (p *ForgejoProvider) CreateIssueComment(owner, repo string, index int64, body string) (*Comment, error) {
	return p.CreateComment(owner, repo, index, body)
}
func toForgejoIssue(i *forgejo_sdk.Issue) *Issue {
	if i == nil {
		return nil
	}
	return &Issue{Index: i.Index, Title: i.Title, Body: i.Body, State: string(i.State),
		HTMLURL: i.HTMLURL, IsPullRequest: i.PullRequest != nil}
}

func (p *ForgejoProvider) GetCombinedStatus(owner, repo, ref string) (*CombinedStatus, error) {
	cs, response, err := p.client.GetCombinedStatus(owner, repo, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit status: %w", err)
	}
	if response != nil && response.Response != nil {
		shape := response.Header.Get(forgejoCombinedStatusShapeHeader)
		if shape == "null" || shape == "empty" {
			return nil, fmt.Errorf("failed to get commit status: malformed %s response", shape)
		}
	}
	return normalizeForgejoCombinedStatus(cs)
}

func normalizeForgejoCombinedStatus(cs *forgejo_sdk.CombinedStatus) (*CombinedStatus, error) {
	if cs == nil {
		return nil, fmt.Errorf("failed to get commit status: empty response")
	}
	total := cs.TotalCount
	count := len(cs.Statuses)
	if total < 0 {
		return nil, fmt.Errorf("failed to get commit status: invalid total count %d", total)
	}
	if total != count {
		return nil, fmt.Errorf("failed to get commit status: total count %d does not match %d statuses", total, count)
	}
	if total == 0 {
		return notConfiguredCombinedStatus(), nil
	}
	if strings.TrimSpace(string(cs.State)) == "" {
		return nil, fmt.Errorf("failed to get commit status: malformed response with statuses but no state")
	}

	statuses := make([]*CommitStatus, count)
	for i, s := range cs.Statuses {
		if s == nil {
			return nil, fmt.Errorf("failed to get commit status: response contains a nil status")
		}
		statuses[i] = &CommitStatus{
			Context:     s.Context,
			State:       string(s.State),
			Description: s.Description,
			TargetURL:   s.TargetURL,
		}
	}

	return &CombinedStatus{
		State:    string(cs.State),
		Statuses: statuses,
	}, nil
}

type forgejoCombinedStatusTransport struct {
	base http.RoundTripper
}

func (t forgejoCombinedStatusTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(req)
	if err != nil || response == nil || response.StatusCode/100 != 2 ||
		!strings.Contains(req.URL.Path, "/commits/") || !strings.HasSuffix(req.URL.Path, "/status") {
		return response, err
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	if response.Header == nil {
		response.Header = make(http.Header)
	}
	if trimmed := bytes.TrimSpace(body); len(trimmed) == 0 {
		response.Header.Set(forgejoCombinedStatusShapeHeader, "empty")
	} else if bytes.Equal(trimmed, []byte("null")) {
		response.Header.Set(forgejoCombinedStatusShapeHeader, "null")
	}
	return response, nil
}

// GetCIFailureDetails fetches CI failure details via Woodpecker CI API.
// Forgejo's native Actions API does not provide useful error info,
// so we use Woodpecker's API directly for failure details and step logs.
//
// Note: this method requires WOODPECKER_URL and WOODPECKER_TOKEN to be set
// independently of the Forgejo credentials; Woodpecker is a separate service.
func (p *ForgejoProvider) GetCIFailureDetails(owner, repo, sha string, tailLines int) ([]*JobFailure, error) {
	wc, err := NewWoodpeckerClient(p.ctx)
	if err != nil {
		return nil, fmt.Errorf("woodpecker client: %w (set WOODPECKER_URL and WOODPECKER_TOKEN)", err)
	}
	return wc.GetFailureDetails(owner, repo, sha, tailLines)
}

func toPullRequest(pr *forgejo_sdk.PullRequest) *PullRequest {
	head := ""
	headSHA := ""
	base := ""
	if pr.Head != nil {
		head = pr.Head.Name
		headSHA = pr.Head.Sha
	}
	if pr.Base != nil {
		base = pr.Base.Name
	}
	return &PullRequest{
		Index:     pr.Index,
		Title:     pr.Title,
		Body:      pr.Body,
		State:     string(pr.State),
		HTMLURL:   pr.HTMLURL,
		Head:      head,
		HeadSHA:   headSHA,
		Base:      base,
		Mergeable: pr.Mergeable,
		Merged:    pr.HasMerged,
	}
}

func toComment(c *forgejo_sdk.Comment) *Comment {
	user := ""
	if c.Poster != nil {
		user = c.Poster.UserName
	}
	return &Comment{
		ID:        c.ID,
		Body:      c.Body,
		User:      user,
		CreatedAt: c.Created,
		HTMLURL:   c.HTMLURL,
	}
}
