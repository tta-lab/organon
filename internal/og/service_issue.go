package og

import (
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

func (s Service) IssueList(req Request) (Response, error) { return s.issueList(req, false) }
func (s Service) IssueSearch(req Request) (Response, error) {
	if strings.TrimSpace(req.Query) == "" {
		return Response{}, fmt.Errorf("issue search query must not be blank")
	}
	return s.issueList(req, true)
}
func (s Service) issueList(req Request, search bool) (Response, error) {
	state, page, per, err := normalizeIssuePage(req.State, req.Page, req.PerPage, search)
	if err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueRead)
	if err != nil {
		return Response{}, err
	}
	items, err := p.ListIssues(ctx.Owner, ctx.Repo, req.Query, state, page, per)
	if err != nil {
		return Response{}, err
	}
	out := make([]Issue, 0, len(items.Issues))
	for _, v := range items.Issues {
		if validIssue(v, 0) {
			out = append(out, fromProviderIssue(v))
		}
	}
	return success(Response{Issues: out, HasNext: items.HasNext, Incomplete: items.Incomplete}), nil
}
func (s Service) IssueGet(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueRead)
	if err != nil {
		return Response{}, err
	}
	issue, err := p.GetIssue(ctx.Owner, ctx.Repo, req.Index)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(issue, req.Index) {
		return Response{}, issueIdentityError(issue, req.Index)
	}
	return success(Response{Issue: ptrIssue(fromProviderIssue(issue))}), nil
}
func (s Service) IssueComments(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	_, page, per, err := normalizeIssuePage("", req.Page, req.PerPage, false)
	if err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueRead)
	if err != nil {
		return Response{}, err
	}
	issue, err := p.GetIssue(ctx.Owner, ctx.Repo, req.Index)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(issue, req.Index) {
		return Response{}, issueIdentityError(issue, req.Index)
	}
	comments, err := p.ListIssueComments(ctx.Owner, ctx.Repo, req.Index, page, per)
	if err != nil {
		return Response{}, err
	}
	out := make([]IssueComment, 0, len(comments.Comments))
	for _, v := range comments.Comments {
		if v != nil {
			out = append(out, IssueComment{ID: v.ID, IssueID: req.Index, Body: v.Body,
				URL: v.HTMLURL, User: v.User, CreatedAt: v.CreatedAt})
		}
	}
	return success(Response{IssueComments: out, HasNext: comments.HasNext}), nil
}
func (s Service) IssueCreate(req Request) (Response, error) {
	if err := ValidateIssueCreate(req.Title, req.Body); err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err = requireRemoteWrite(ctx, "create issue"); err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueWrite)
	if err != nil {
		return Response{}, err
	}
	issue, err := p.CreateIssue(ctx.Owner, ctx.Repo, *req.Title, *req.Body)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(issue, 0) {
		return Response{}, fmt.Errorf("provider returned invalid issue after creation")
	}
	return success(Response{Issue: ptrIssue(fromProviderIssue(issue))}), nil
}
func (s Service) IssueUpdateTitle(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	if err := ValidateIssueTitle(req.Title); err != nil {
		return Response{}, err
	}
	return s.writeIssue(req, "update issue title", func(
		p gitprovider.IssueProvider, c *repoContext,
	) (*gitprovider.Issue, error) {
		return p.UpdateIssueTitle(c.Owner, c.Repo, req.Index, *req.Title)
	})
}
func (s Service) IssueReplaceBody(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	if req.Body == nil {
		return Response{}, fmt.Errorf("issue body is required")
	}
	return s.writeIssue(req, "replace issue body", func(
		p gitprovider.IssueProvider, c *repoContext,
	) (*gitprovider.Issue, error) {
		return p.ReplaceIssueBody(c.Owner, c.Repo, req.Index, *req.Body)
	})
}
func (s Service) IssueEditBody(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	if err := ValidateIssueEdits(req.Edits); err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err = requireRemoteWrite(ctx, "edit issue body"); err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueWrite)
	if err != nil {
		return Response{}, err
	}
	issue, err := p.GetIssue(ctx.Owner, ctx.Repo, req.Index)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(issue, req.Index) {
		return Response{}, issueIdentityError(issue, req.Index)
	}
	body, err := applyIssueEdits(issue.Body, req.Edits)
	if err != nil {
		return Response{}, err
	}
	updated, err := p.ReplaceIssueBody(ctx.Owner, ctx.Repo, req.Index, body)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(updated, req.Index) || updated.Body != body {
		return Response{}, fmt.Errorf("provider returned invalid issue body after edit")
	}
	return success(Response{Issue: ptrIssue(fromProviderIssue(updated))}), nil
}
func (s Service) IssueComment(req Request) (Response, error) {
	if err := ValidatePositiveIssueID(req.Index); err != nil {
		return Response{}, err
	}
	if err := ValidateIssueComment(req.Body); err != nil {
		return Response{}, err
	}
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err = requireRemoteWrite(ctx, "comment on issue"); err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueWrite)
	if err != nil {
		return Response{}, err
	}
	issue, err := p.GetIssue(ctx.Owner, ctx.Repo, req.Index)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(issue, req.Index) {
		return Response{}, issueIdentityError(issue, req.Index)
	}
	comment, err := p.CreateIssueComment(ctx.Owner, ctx.Repo, req.Index, *req.Body)
	if err != nil {
		return Response{}, err
	}
	if comment == nil || comment.ID <= 0 || comment.HTMLURL == "" || comment.Body != *req.Body {
		return Response{}, fmt.Errorf("provider returned invalid issue comment")
	}
	return success(Response{IssueComments: []IssueComment{{
		ID: comment.ID, IssueID: req.Index, Body: comment.Body, URL: comment.HTMLURL,
		User: comment.User, CreatedAt: comment.CreatedAt,
	}}}), nil
}

type issueWrite func(gitprovider.IssueProvider, *repoContext) (*gitprovider.Issue, error)

func (s Service) writeIssue(req Request, op string, write issueWrite) (Response, error) {
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err = requireRemoteWrite(ctx, op); err != nil {
		return Response{}, err
	}
	p, err := issueProviderFor(ctx, githubapp.PurposeIssueWrite)
	if err != nil {
		return Response{}, err
	}
	current, err := p.GetIssue(ctx.Owner, ctx.Repo, req.Index)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(current, req.Index) {
		return Response{}, issueIdentityError(current, req.Index)
	}
	updated, err := write(p, ctx)
	if err != nil {
		return Response{}, err
	}
	if !validIssue(updated, req.Index) {
		return Response{}, fmt.Errorf("provider returned invalid issue after update")
	}
	return success(Response{Issue: ptrIssue(fromProviderIssue(updated))}), nil
}
func validIssue(i *gitprovider.Issue, id int64) bool {
	return i != nil && i.Index > 0 && (id == 0 || i.Index == id) && !i.IsPullRequest && i.HTMLURL != ""
}
func issueIdentityError(i *gitprovider.Issue, id int64) error {
	if i != nil && i.IsPullRequest {
		return fmt.Errorf("#%d is a pull request, not an issue", id)
	}
	return fmt.Errorf("provider returned invalid issue #%d", id)
}
func fromProviderIssue(i *gitprovider.Issue) Issue {
	return Issue{Index: i.Index, Title: i.Title, Body: i.Body, State: i.State, URL: i.HTMLURL}
}
func ptrIssue(i Issue) *Issue { return &i }
