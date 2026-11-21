package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

type ogProjectInput struct {
	Project string `json:"project" jsonschema:"project reference: alias, checkout, or repository basename"`
}

type ogCheckoutInput struct {
	Project  string `json:"project,omitempty" jsonschema:"registered project reference; use one of project and worktree"`
	Worktree string `json:"worktree,omitempty" jsonschema:"absolute linked worktree root on the server"`
}

type ogPushInput struct {
	ogCheckoutInput
	Force bool `json:"force,omitempty" jsonschema:"use force-with-lease; rejected on the default branch"`
}

type ogCloneInput struct {
	Project   string `json:"project,omitempty" jsonschema:"project reference: alias, checkout, or repository basename"`
	URL       string `json:"url,omitempty" jsonschema:"HTTP(S) repository URL with exactly owner/repo"`
	Alias     string `json:"alias,omitempty" jsonschema:"optional exact single-layer project alias"`
	Reference bool   `json:"reference,omitempty" jsonschema:"clone under the references tree without registration"`
}

type ogPRCreateInput struct {
	ogCheckoutInput
	Title string `json:"title" jsonschema:"non-blank pull request title"`
	Body  string `json:"body,omitempty" jsonschema:"optional pull request body"`
}

type ogPRFindInput struct {
	ogCheckoutInput
	State string `json:"state,omitempty" jsonschema:"pull request state: open, closed, or all"`
}

type ogPRInput struct {
	ogCheckoutInput
	PRID *int64 `json:"pr_id,omitempty" jsonschema:"optional positive pull request ID; omitted uses the current branch"`
}

type ogPRModifyInput struct {
	ogCheckoutInput
	PRID  *int64  `json:"pr_id,omitempty" jsonschema:"positive PR ID; omitted uses the current branch"`
	Title *string `json:"title,omitempty" jsonschema:"optional replacement pull request title"`
	Body  *string `json:"body,omitempty" jsonschema:"optional replacement pull request body; an empty string clears it"`
}

type ogPREditsInput struct {
	ogCheckoutInput
	PRID  *int64        `json:"pr_id,omitempty" jsonschema:"positive PR ID; omitted uses the current branch"`
	Edits []og.BodyEdit `json:"edits" jsonschema:"exact unique body matches; empty newText deletes"`
}

type ogPRCommentInput struct {
	ogCheckoutInput
	PRID *int64 `json:"pr_id,omitempty" jsonschema:"optional positive pull request ID; omitted uses the current branch"`
	Body string `json:"body" jsonschema:"non-blank pull request comment body"`
}

type ogPRMergeInput struct {
	ogCheckoutInput
	PRID           *int64 `json:"pr_id,omitempty" jsonschema:"optional positive PR ID; omitted uses current branch"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"record a mock merge without changing the forge"`
	Wait           bool   `json:"wait,omitempty" jsonschema:"wait for Impri only; none returns immediately"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" jsonschema:"wait timeout seconds; defaults to 30"`
}
type ogIssuePageInput struct {
	Project string `json:"project"`
	State   string `json:"state,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}
type ogIssueSearchInput struct {
	Project string `json:"project"`
	Query   string `json:"query"`
	State   string `json:"state,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}
type ogIssueInput struct {
	Project string `json:"project"`
	IssueID int64  `json:"issue_id"`
}
type ogIssueCommentsInput struct {
	Project string `json:"project"`
	IssueID int64  `json:"issue_id"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}
type ogIssueCreateInput struct {
	Project string  `json:"project"`
	Title   string  `json:"title"`
	Body    *string `json:"body"`
}
type ogIssueTitleInput struct {
	Project string `json:"project"`
	IssueID int64  `json:"issue_id"`
	Title   string `json:"title"`
}
type ogIssueBodyInput struct {
	Project string `json:"project"`
	IssueID int64  `json:"issue_id"`
	Body    string `json:"body"`
}
type ogIssueEditsInput struct {
	Project string        `json:"project"`
	IssueID int64         `json:"issue_id"`
	Edits   []og.BodyEdit `json:"edits"`
}
type ogIssueCommentInput struct {
	Project string `json:"project"`
	IssueID int64  `json:"issue_id"`
	Body    string `json:"body"`
}

type ogAuthOutput struct {
	Project string        `json:"project"`
	Auth    og.AuthStatus `json:"auth"`
}

type ogPROutput struct {
	Project string         `json:"project"`
	PR      og.PullRequest `json:"pr"`
}

type ogPRLinesOutput struct {
	Project string         `json:"project"`
	PR      og.PullRequest `json:"pr"`
	Lines   []string       `json:"lines"`
}

type ogCommentOutput struct {
	Project string     `json:"project"`
	Comment og.Comment `json:"comment"`
}
type ogIssueCommentOutput struct {
	Project string          `json:"project"`
	Comment og.IssueComment `json:"comment"`
}

type ogMessageOutput struct {
	Project string `json:"project"`
	Message string `json:"message"`
}

type ogCloneOutput struct {
	Clone og.CloneResult `json:"clone"`
}

type ogPRMergeOutput struct {
	Project string           `json:"project"`
	Merge   og.PRMergeResult `json:"merge"`
}
type ogIssueOutput struct {
	Project string   `json:"project"`
	Issue   og.Issue `json:"issue"`
}
type ogIssuesOutput struct {
	Project    string     `json:"project"`
	Issues     []og.Issue `json:"issues"`
	HasNext    bool       `json:"has_next"`
	Incomplete bool       `json:"incomplete"`
}
type ogIssueCommentsOutput struct {
	Project  string            `json:"project"`
	Comments []og.IssueComment `json:"comments"`
	HasNext  bool              `json:"has_next"`
}

func ogBoolPointer(value bool) *bool { return &value }

func ogTool(name, title, description string, readOnly, destructive, idempotent bool) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    readOnly,
			DestructiveHint: ogBoolPointer(destructive),
			IdempotentHint:  idempotent,
			OpenWorldHint:   ogBoolPointer(true),
		},
	}
}

func inputSchemaFor[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("infer MCP input schema for %s: %v", reflect.TypeFor[T](), err))
	}
	if prID := schema.Properties["pr_id"]; prID != nil {
		prID.Minimum = jsonschema.Ptr(1.0)
	}
	if issueID := schema.Properties["issue_id"]; issueID != nil {
		issueID.Minimum = jsonschema.Ptr(1.0)
	}
	if page := schema.Properties["page"]; page != nil {
		page.Minimum = jsonschema.Ptr(1.0)
		page.Default = json.RawMessage("1")
	}
	if per := schema.Properties["per_page"]; per != nil {
		per.Minimum = jsonschema.Ptr(1.0)
		per.Maximum = jsonschema.Ptr(float64(og.MaxIssuePerPage))
		per.Default = json.RawMessage(fmt.Sprint(og.DefaultIssuePerPage))
	}
	if force := schema.Properties["force"]; force != nil {
		force.Default = json.RawMessage("false")
	}
	if reference := schema.Properties["reference"]; reference != nil {
		reference.Default = json.RawMessage("false")
	}
	if timeout := schema.Properties["timeout_seconds"]; timeout != nil {
		timeout.Minimum = jsonschema.Ptr(0.0)
		timeout.Maximum = jsonschema.Ptr(86400.0)
		timeout.Default = json.RawMessage("30")
	}
	if schema.Properties["project"] != nil && schema.Properties["url"] != nil {
		schema.OneOf = []*jsonschema.Schema{
			{Required: []string{"project"}},
			{Required: []string{"url"}},
		}
	}
	if worktree := schema.Properties["worktree"]; worktree != nil {
		worktree.MinLength = jsonschema.Ptr(1)
		schema.Properties[sourceProjectField].MinLength = jsonschema.Ptr(1)
		schema.OneOf = []*jsonschema.Schema{
			{Required: []string{sourceProjectField}},
			{Required: []string{"worktree"}},
		}
	}
	if state := schema.Properties["state"]; state != nil {
		state.Default = json.RawMessage(`"` + og.PRStateOpen + `"`)
		state.Enum = []any{og.PRStateOpen, og.PRStateClosed, og.PRStateAll}
	}
	return schema
}

func setInputSchema[T any](tool *mcp.Tool) *mcp.Tool {
	tool.InputSchema = inputSchemaFor[T]()
	return tool
}

func validateCloneSelector(projectAlias, rawURL, alias string, reference bool) error {
	hasProject := strings.TrimSpace(projectAlias) != ""
	hasURL := strings.TrimSpace(rawURL) != ""
	if hasProject == hasURL {
		return fmt.Errorf("exactly one of project and url is required")
	}
	if hasProject && (alias != "" || reference) {
		return fmt.Errorf("project clone does not accept alias or reference")
	}
	if reference && alias != "" {
		return fmt.Errorf("reference clone does not accept alias")
	}
	return nil
}

func newOGMCPServer(projects *project.Store, executor og.Executor) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-og", Version: "1.0.0"}, nil)
	addProjectTools(server, projects)
	addSourceTools(server, projects)
	addRepoTools(server, projects)

	mcp.AddTool(server, setInputSchema[ogPREditsInput](ogTool(
		"pr_edit_body", "Edit pull request body",
		"Apply exact oldText/newText edits matched uniquely against the original PR body. "+
			"Reject empty, no-op, duplicate or overlapping edits before one update; omitted pr_id uses the current branch.",
		false, true, false,
	)), prEditBodyHandler(projects, executor))

	mcp.AddTool(server, setInputSchema[ogCloneInput](ogTool(
		"clone", "Clone repository",
		"Clone an HTTP(S) repository to its controlled project or reference path.",
		false, false, true,
	)), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogCloneInput,
	) (*mcp.CallToolResult, ogCloneOutput, error) {
		if err := validateCloneSelector(input.Project, input.URL, input.Alias, input.Reference); err != nil {
			return nil, ogCloneOutput{}, err
		}
		req := og.Request{
			Context: ctx, Project: input.Project, URL: input.URL, Alias: input.Alias, Reference: input.Reference,
		}
		resp, err := executor.GitClone(req)
		if err != nil {
			return nil, ogCloneOutput{}, fmt.Errorf("execute clone: %w", err)
		}
		if err := og.ValidateCloneResponse(resp); err != nil {
			return nil, ogCloneOutput{}, err
		}
		return nil, ogCloneOutput{Clone: *resp.Clone}, nil
	})

	mcp.AddTool(server, setInputSchema[ogProjectInput](ogTool(
		"auth_status", "Inspect forge authentication",
		"Inspect secret-free forge authentication status for one registered project.", true, false, true,
	)), authStatusHandler(projects, executor))

	mcp.AddTool(server, setInputSchema[ogPushInput](ogTool(
		"push", "Push current branch",
		"Push the selected checkout's current branch; force uses force-with-lease and is rejected on the default branch.",
		false, true, true,
	)), pushHandler(projects, executor))

	mcp.AddTool(server, setInputSchema[ogCheckoutInput](ogTool(
		"pull", "Pull current branch",
		"Fast-forward the selected branch. Guarded closed-PR branch cleanup is available in the registered checkout.",
		false, true, true,
	)), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogCheckoutInput,
	) (*mcp.CallToolResult, ogMessageOutput, error) {
		return callMessageTool(ctx, projects, input, og.Request{}, executor.GitPull)
	})

	mcp.AddTool(server, setInputSchema[ogPRCreateInput](ogTool(
		"pr_create", "Create pull request",
		"Push the selected checkout's current branch and create its pull request.", false, false, false,
	)), prCreateHandler(projects, executor))
	addIssueTools(server, projects, executor)

	mcp.AddTool(server, setInputSchema[ogPRFindInput](ogTool(
		"pr_find", "Find current branch pull request",
		"Find a pull request for the selected checkout's current branch by state.", true, false, true,
	)), prFindHandler(projects, executor))

	mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(
		"pr_get", "Get pull request",
		"Get by positive PR ID, or view the selected checkout's current branch pull request when omitted; "+
			"CI is included only for Forgejo.",
		true, false, true,
	)), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		if input.PRID == nil {
			return callPRCheckoutTool(ctx, projects, input.ogCheckoutInput, og.Request{State: og.PRStateAll}, executor.PRView)
		}
		return callPRTool(ctx, projects, input.ogCheckoutInput, *input.PRID, executor.PRGet)
	})

	mcp.AddTool(server, setInputSchema[ogPRModifyInput](ogTool(
		"pr_modify", "Modify pull request",
		"Modify title and/or body by positive PR ID or for the registered checkout's current branch when omitted.",
		false, true, true,
	)), prModifyHandler(projects, executor))

	mcp.AddTool(server, setInputSchema[ogPRCommentInput](ogTool(
		"pr_comment", "Comment on pull request",
		"Comment by positive PR ID or on the registered checkout's current branch when omitted.", false, false, false,
	)), prCommentHandler(projects, executor))

	addPRLinesTool(server, projects, "pr_checks", "Inspect pull request checks", executor.PRChecks)

	mcp.AddTool(server, setInputSchema[ogPRMergeInput](ogTool(
		"pr_merge", "Policy-controlled squash merge",
		"Forgejo only; GitHub merge is unsupported. "+
			"Honor global merge.approval: impri (default) submits an immutable snapshot to Impri; "+
			"none executes immediately without Impri or notifications; deny rejects all merges, including dry-run. "+
			"Direct results identify approval_policy=none, "+
			"omit action/inbox/receipt, and use blocked (retry_same_request), failed (none), or executed. "+
			"Pending/unverifiable CI returns retryable immediately. Wait/timeout wait only under impri. "+
			"Repeat the same selector, explicit PR ID, and mode for uncertain outcomes or cleanup-only recovery; "+
			"observed merged PRs never merge again. Under impri, surface the inbox link "+
			"and execute only after web approval; dry-run records a mock merge and "+
			"real mode performs a squash merge and removes matching head refs. It also removes a "+
			"selected linked worktree, including ignored files, or pulls the default branch in the registered checkout. "+
			"Dry-run performs no Git cleanup. Structured outcomes route pending or "+
			"wait-timeout to the inbox, approved temporary failures to repeating the same "+
			"request, temporarily unavailable Impri approval state to the same retry "+
			"without a forge call, executed to completion, terminal rejection/expiry to a "+
			"replacement approval, terminal execution failure to new approval, receipt errors "+
			"to receipt repair, and cleanup errors to "+
			"the same request without another merge. "+
			"If an execute_failed receipt cannot be recorded, the result stays approved and "+
			"the same request must be repeated to revalidate and report it.",
		false, true, true,
	)), prMergeHandler(projects, executor))

	return server
}

func pushHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogPushInput, ogMessageOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPushInput,
	) (*mcp.CallToolResult, ogMessageOutput, error) {
		return callMessageTool(ctx, projects, input.ogCheckoutInput, og.Request{Force: input.Force}, executor.GitPush)
	}
}

func prCreateHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogPRCreateInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRCreateInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		if err := og.ValidatePRTitle(input.Title); err != nil {
			return nil, ogPROutput{}, err
		}
		return callPRCheckoutTool(ctx, projects, input.ogCheckoutInput, og.Request{
			Title: &input.Title,
			Body:  &input.Body,
		}, executor.PRCreate)
	}
}

func prFindHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogPRFindInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRFindInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		state, err := og.NormalizePRState(input.State)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		return callPRCheckoutTool(ctx, projects, input.ogCheckoutInput, og.Request{State: state}, executor.PRFind)
	}
}

func callMessageTool(
	ctx context.Context,
	projects *project.Store,
	selector ogCheckoutInput,
	req og.Request,
	operation func(og.Request) (og.Response, error),
) (*mcp.CallToolResult, ogMessageOutput, error) {
	resp, canonical, err := callCheckout(ctx, projects, selector, req, operation)
	if err != nil {
		return nil, ogMessageOutput{}, err
	}
	if err := og.ValidateMessageResponse(resp); err != nil {
		return nil, ogMessageOutput{}, err
	}
	return nil, ogMessageOutput{Project: canonical, Message: resp.Message}, nil
}

func callPRCheckoutTool(
	ctx context.Context,
	projects *project.Store,
	selector ogCheckoutInput,
	req og.Request,
	operation func(og.Request) (og.Response, error),
) (*mcp.CallToolResult, ogPROutput, error) {
	resp, canonical, err := callCheckout(ctx, projects, selector, req, operation)
	if err != nil {
		return nil, ogPROutput{}, err
	}
	if err := og.ValidatePRResponse(resp, 0); err != nil {
		return nil, ogPROutput{}, err
	}
	return nil, ogPROutput{Project: canonical, PR: *resp.PR}, nil
}

func callProject(
	ctx context.Context,
	projects *project.Store,
	reference string,
	req og.Request,
	operation func(og.Request) (og.Response, error),
) (og.Response, string, error) {
	return callCheckout(ctx, projects, ogCheckoutInput{Project: reference}, req, operation)
}

func callCheckout(
	ctx context.Context,
	projects *project.Store,
	selector ogCheckoutInput,
	req og.Request,
	operation func(og.Request) (og.Response, error),
) (og.Response, string, error) {
	var entry project.Entry
	var err error
	if selector.Worktree != "" {
		req.WorkDir, entry, err = og.ResolveWorktree(ctx, selector.Worktree, projects)
	} else {
		entry, err = resolveOGProject(projects, selector.Project)
		req.WorkDir = entry.Path
	}
	if err != nil {
		return og.Response{}, "", err
	}
	req.Context = ctx
	resp, err := operation(req)
	if err != nil {
		return resp, entry.Alias, fmt.Errorf("execute OG operation: %w", err)
	}
	return resp, entry.Alias, nil
}

func authStatusHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogProjectInput, ogAuthOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogProjectInput,
	) (*mcp.CallToolResult, ogAuthOutput, error) {
		resp, canonical, err := callProject(ctx, projects, input.Project, og.Request{}, executor.AuthStatus)
		if err != nil {
			return nil, ogAuthOutput{}, err
		}
		if err := og.ValidateAuthResponse(resp, canonical); err != nil {
			return nil, ogAuthOutput{}, err
		}
		return nil, ogAuthOutput{Project: canonical, Auth: *resp.Auth}, nil
	}
}

func prModifyHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogPRModifyInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRModifyInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		prID, err := optionalMCPPRID(input.PRID)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := og.ValidatePRModifyInput(input.Title, input.Body); err != nil {
			return nil, ogPROutput{}, err
		}
		resp, canonical, err := callCheckout(ctx, projects, input.ogCheckoutInput, og.Request{
			Index: prID, Title: input.Title, Body: input.Body,
		}, executor.PRModify)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := og.ValidatePRModifyResponse(resp, prID, input.Title, input.Body); err != nil {
			return nil, ogPROutput{}, err
		}
		return nil, ogPROutput{Project: canonical, PR: *resp.PR}, nil
	}
}

func prCommentHandler(
	projects *project.Store, executor og.Executor,
) mcp.ToolHandlerFor[ogPRCommentInput, ogCommentOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRCommentInput,
	) (*mcp.CallToolResult, ogCommentOutput, error) {
		prID, err := optionalMCPPRID(input.PRID)
		if err != nil {
			return nil, ogCommentOutput{}, err
		}
		if err := og.ValidatePRCommentBody(&input.Body); err != nil {
			return nil, ogCommentOutput{}, err
		}
		resp, canonical, err := callCheckout(ctx, projects, input.ogCheckoutInput, og.Request{
			Index: prID, Body: &input.Body,
		}, executor.PRComment)
		if err != nil {
			return nil, ogCommentOutput{}, err
		}
		if err := og.ValidateCommentResponse(resp, prID, input.Body); err != nil {
			return nil, ogCommentOutput{}, err
		}
		return nil, ogCommentOutput{Project: canonical, Comment: *resp.Comment}, nil
	}
}

func prMergeHandler(
	projects *project.Store, executor og.Executor,
) mcp.ToolHandlerFor[ogPRMergeInput, ogPRMergeOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRMergeInput,
	) (*mcp.CallToolResult, ogPRMergeOutput, error) {
		prID, err := optionalMCPPRID(input.PRID)
		if err != nil {
			return nil, ogPRMergeOutput{}, err
		}
		timeout := time.Duration(0)
		if input.TimeoutSeconds != nil {
			timeout = time.Duration(*input.TimeoutSeconds) * time.Second
		}
		req := og.Request{
			Index: prID, DryRun: input.DryRun,
			Wait: input.Wait, Timeout: timeout,
		}
		if req, err = og.NormalizePRMergeRequest(req); err != nil {
			return nil, ogPRMergeOutput{}, err
		}
		resp, canonical, err := callCheckout(ctx, projects, input.ogCheckoutInput, req, executor.PRMerge)
		if err != nil {
			var retryErr *og.PRMergeRetryableError
			if errors.As(err, &retryErr) && resp.Merge != nil {
				if validateErr := og.ValidatePRMergeResponse(resp, prID); validateErr != nil {
					return nil, ogPRMergeOutput{}, validateErr
				}
				return &mcp.CallToolResult{IsError: true}, ogPRMergeOutput{
					Project: canonical, Merge: *resp.Merge,
				}, nil
			}
			return nil, ogPRMergeOutput{}, err
		}
		if err := og.ValidatePRMergeResponse(resp, prID); err != nil {
			return nil, ogPRMergeOutput{}, err
		}
		return nil, ogPRMergeOutput{Project: canonical, Merge: *resp.Merge}, nil
	}
}

func callPRTool(
	ctx context.Context,
	projects *project.Store,
	selector ogCheckoutInput,
	id int64,
	operation func(og.Request) (og.Response, error),
) (*mcp.CallToolResult, ogPROutput, error) {
	if err := og.ValidatePositivePRID(id); err != nil {
		return nil, ogPROutput{}, err
	}
	resp, canonical, err := callCheckout(ctx, projects, selector, og.Request{Index: id}, operation)
	if err != nil {
		return nil, ogPROutput{}, err
	}
	if err := og.ValidatePRResponse(resp, id); err != nil {
		return nil, ogPROutput{}, err
	}
	return nil, ogPROutput{Project: canonical, PR: *resp.PR}, nil
}

func addPRLinesTool(
	server *mcp.Server, projects *project.Store, name, title string,
	operation func(og.Request) (og.Response, error),
) {
	mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(name, title,
		"Inspect Forgejo pull request CI status by ID or current branch.", true, false, true)),
		func(ctx context.Context, _ *mcp.CallToolRequest, input ogPRInput) (*mcp.CallToolResult, ogPRLinesOutput, error) {
			id, err := optionalMCPPRID(input.PRID)
			if err != nil {
				return nil, ogPRLinesOutput{}, err
			}
			resp, canonical, err := callCheckout(ctx, projects, input.ogCheckoutInput, og.Request{Index: id}, operation)
			if err != nil {
				return nil, ogPRLinesOutput{}, err
			}
			if err := og.ValidatePRResponse(resp, id); err != nil {
				return nil, ogPRLinesOutput{}, err
			}
			return nil, ogPRLinesOutput{Project: canonical, PR: *resp.PR, Lines: resp.Lines}, nil
		})
}

func optionalMCPPRID(id *int64) (int64, error) {
	if id == nil {
		return 0, nil
	}
	if err := og.ValidatePositivePRID(*id); err != nil {
		return 0, err
	}
	return *id, nil
}

func newOGMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve typed forge tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := config.InjectDotEnvFallback(); err != nil {
				cmd.PrintErrf("warning: could not load .env: %v\n", err)
			}
			projects := discoveryStore()
			if _, err := projects.Snapshot(); err != nil {
				return err
			}
			executor := newDeferredExecutor(func() (og.Executor, error) {
				return og.LoadService(config.OGConfigPath(), config.DefaultConfigDir())
			})
			return newOGMCPServer(projects, executor).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}

func prEditBodyHandler(projects *project.Store, executor og.Executor) mcp.ToolHandlerFor[ogPREditsInput, ogPROutput] {
	return func(
		ctx context.Context, _ *mcp.CallToolRequest, input ogPREditsInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		prID, err := optionalMCPPRID(input.PRID)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := og.ValidateBodyEdits(input.Edits); err != nil {
			return nil, ogPROutput{}, err
		}
		resp, canonical, err := callCheckout(ctx, projects, input.ogCheckoutInput, og.Request{
			Index: prID, Edits: input.Edits,
		}, executor.PREditBody)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := og.ValidatePRResponse(resp, prID); err != nil {
			return nil, ogPROutput{}, err
		}
		return nil, ogPROutput{Project: canonical, PR: *resp.PR}, nil
	}
}
