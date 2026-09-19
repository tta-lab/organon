package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

func addIssueTools(s *mcp.Server, p *project.Store, e og.Executor) {
	mcp.AddTool(s,
		setInputSchema[ogIssuePageInput](ogTool(
			"issue_list", "List issues", "List repository issues; defaults to open.", true, false, true,
		), false),
		func(c context.Context, _ *mcp.CallToolRequest, in ogIssuePageInput) (*mcp.CallToolResult, ogIssuesOutput, error) {
			return issueListTool(c, p, in.Project, og.Request{State: in.State, Page: in.Page, PerPage: in.PerPage}, e.IssueList)
		})
	mcp.AddTool(s,
		setInputSchema[ogIssueSearchInput](ogTool(
			"issue_search", "Search issues", "Search repository issues; defaults to all.", true, false, true,
		), false),
		func(c context.Context, _ *mcp.CallToolRequest, in ogIssueSearchInput) (*mcp.CallToolResult, ogIssuesOutput, error) {
			req := og.Request{Query: in.Query, State: in.State, Page: in.Page, PerPage: in.PerPage}
			return issueListTool(c, p, in.Project, req, e.IssueSearch)
		})
	mcp.AddTool(s,
		setInputSchema[ogIssueInput](ogTool(
			"issue_get", "Get issue", "Get one explicit non-pull-request issue.", true, false, true,
		), false),
		func(c context.Context, _ *mcp.CallToolRequest, in ogIssueInput) (*mcp.CallToolResult, ogIssueOutput, error) {
			return issueTool(c, p, in.Project, og.Request{Index: in.IssueID}, e.IssueGet)
		})
	mcp.AddTool(s, setInputSchema[ogIssueCommentsInput](ogTool(
		"issue_comments", "List issue comments", "List issue comments oldest first.", true, false, true,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueCommentsInput,
	) (*mcp.CallToolResult, ogIssueCommentsOutput, error) {
		req := og.Request{Index: in.IssueID, Page: in.Page, PerPage: in.PerPage}
		resp, a, err := callProject(c, p, in.Project, req, e.IssueComments)
		if err != nil {
			return nil, ogIssueCommentsOutput{}, err
		}
		return nil, ogIssueCommentsOutput{Project: a, Comments: resp.IssueComments, HasNext: resp.HasNext}, nil
	})
	mcp.AddTool(s, setInputSchema[ogIssueCreateInput](ogTool(
		"issue_create", "Create issue", "Create an issue with required title and body.", false, true, false,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueCreateInput,
	) (*mcp.CallToolResult, ogIssueOutput, error) {
		if err := og.ValidateIssueCreate(&in.Title, in.Body); err != nil {
			return nil, ogIssueOutput{}, err
		}
		return issueTool(c, p, in.Project, og.Request{Title: &in.Title, Body: in.Body}, e.IssueCreate)
	})
	mcp.AddTool(s, setInputSchema[ogIssueTitleInput](ogTool(
		"issue_update_title", "Update issue title", "Replace only an issue title.", false, true, true,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueTitleInput,
	) (*mcp.CallToolResult, ogIssueOutput, error) {
		return issueTool(c, p, in.Project, og.Request{Index: in.IssueID, Title: &in.Title}, e.IssueUpdateTitle)
	})
	mcp.AddTool(s, setInputSchema[ogIssueBodyInput](ogTool(
		"issue_replace_body", "Replace issue body", "Replace only an issue body; empty clears it.", false, true, true,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueBodyInput,
	) (*mcp.CallToolResult, ogIssueOutput, error) {
		return issueTool(c, p, in.Project, og.Request{Index: in.IssueID, Body: &in.Body}, e.IssueReplaceBody)
	})
	mcp.AddTool(s, setInputSchema[ogIssueEditsInput](ogTool(
		"issue_edit_body", "Edit issue body", "Apply exact oldText/newText edits to one current issue body.",
		false, true, false,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueEditsInput,
	) (*mcp.CallToolResult, ogIssueOutput, error) {
		return issueTool(c, p, in.Project, og.Request{Index: in.IssueID, Edits: in.Edits}, e.IssueEditBody)
	})
	mcp.AddTool(s, setInputSchema[ogIssueCommentInput](ogTool(
		"issue_comment", "Comment on issue", "Add a comment without changing the issue.", false, true, false,
	), false), func(
		c context.Context, _ *mcp.CallToolRequest, in ogIssueCommentInput,
	) (*mcp.CallToolResult, ogIssueCommentOutput, error) {
		resp, a, err := callProject(c, p, in.Project, og.Request{Index: in.IssueID, Body: &in.Body}, e.IssueComment)
		if err != nil {
			return nil, ogIssueCommentOutput{}, err
		}
		if len(resp.IssueComments) != 1 {
			return nil, ogIssueCommentOutput{}, fmt.Errorf("og returned no issue comment")
		}
		return nil, ogIssueCommentOutput{Project: a, Comment: resp.IssueComments[0]}, nil
	})
}

type issueOperation func(og.Request) (og.Response, error)

func issueListTool(
	c context.Context, p *project.Store, a string, r og.Request, op issueOperation,
) (*mcp.CallToolResult, ogIssuesOutput, error) {
	resp, canonical, err := callProject(c, p, a, r, op)
	if err != nil {
		return nil, ogIssuesOutput{}, err
	}
	output := ogIssuesOutput{Project: canonical, Issues: resp.Issues, HasNext: resp.HasNext, Incomplete: resp.Incomplete}
	return nil, output, nil
}
func issueTool(
	c context.Context, p *project.Store, a string, r og.Request, op issueOperation,
) (*mcp.CallToolResult, ogIssueOutput, error) {
	resp, canonical, err := callProject(c, p, a, r, op)
	if err != nil {
		return nil, ogIssueOutput{}, err
	}
	if resp.Issue == nil {
		return nil, ogIssueOutput{}, fmt.Errorf("og returned no issue")
	}
	return nil, ogIssueOutput{Project: canonical, Issue: *resp.Issue}, nil
}
