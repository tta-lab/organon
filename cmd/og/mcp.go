package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

type ogDaemonCaller interface {
	CallContext(context.Context, string, og.Request) (og.Response, error)
}

type ogProjectInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
}

type ogPushInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	Force   bool   `json:"force,omitempty" jsonschema:"use force-with-lease; rejected on the default branch"`
}

type ogCloneInput struct {
	Project   string `json:"project,omitempty" jsonschema:"exact registered single-layer project alias"`
	URL       string `json:"url,omitempty" jsonschema:"HTTP(S) repository URL with exactly owner/repo"`
	Alias     string `json:"alias,omitempty" jsonschema:"optional exact single-layer project alias"`
	Reference bool   `json:"reference,omitempty" jsonschema:"clone under the references tree without registration"`
}

type ogPRCreateInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	Title   string `json:"title" jsonschema:"non-blank pull request title"`
	Body    string `json:"body,omitempty" jsonschema:"optional pull request body"`
}

type ogPRFindInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	State   string `json:"state,omitempty" jsonschema:"pull request state: open, closed, or all"`
}

type ogPRInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    *int64 `json:"pr_id,omitempty" jsonschema:"optional positive pull request ID; omitted uses the current branch"`
}

type ogPRModifyInput struct {
	Project string  `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    *int64  `json:"pr_id,omitempty" jsonschema:"positive PR ID; omitted uses the current branch"`
	Title   *string `json:"title,omitempty" jsonschema:"optional replacement pull request title"`
	Body    *string `json:"body,omitempty" jsonschema:"optional replacement pull request body; an empty string clears it"`
}

type ogPRCommentInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    *int64 `json:"pr_id,omitempty" jsonschema:"optional positive pull request ID; omitted uses the current branch"`
	Body    string `json:"body" jsonschema:"non-blank pull request comment body"`
}

type ogPRTailInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    *int64 `json:"pr_id,omitempty" jsonschema:"optional positive pull request ID; omitted uses the current branch"`
	Tail    int    `json:"tail,omitempty" jsonschema:"optional number of log tail lines; defaults to 50"`
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

type ogMessageOutput struct {
	Project string `json:"project"`
	Message string `json:"message"`
}

type ogCloneOutput struct {
	Clone og.CloneResult `json:"clone"`
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

func inputSchemaFor[T any](tail bool) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("infer MCP input schema for %s: %v", reflect.TypeFor[T](), err))
	}
	if prID := schema.Properties["pr_id"]; prID != nil {
		prID.Minimum = jsonschema.Ptr(1.0)
	}
	if tail {
		tailSchema := schema.Properties["tail"]
		tailSchema.Minimum = jsonschema.Ptr(0.0)
		tailSchema.Maximum = jsonschema.Ptr(float64(og.MaxPRLogTail))
		tailSchema.Default = json.RawMessage(fmt.Sprint(og.DefaultPRLogTail))
	}
	if force := schema.Properties["force"]; force != nil {
		force.Default = json.RawMessage("false")
	}
	if reference := schema.Properties["reference"]; reference != nil {
		reference.Default = json.RawMessage("false")
	}
	if schema.Properties["project"] != nil && schema.Properties["url"] != nil {
		schema.OneOf = []*jsonschema.Schema{
			{Required: []string{"project"}},
			{Required: []string{"url"}},
		}
	}
	if state := schema.Properties["state"]; state != nil {
		state.Default = json.RawMessage(`"` + og.PRStateOpen + `"`)
		state.Enum = []any{og.PRStateOpen, og.PRStateClosed, og.PRStateAll}
	}
	return schema
}

func setInputSchema[T any](tool *mcp.Tool, tail bool) *mcp.Tool {
	tool.InputSchema = inputSchemaFor[T](tail)
	return tool
}

func callOGDaemon(
	ctx context.Context,
	projects *project.Store,
	caller ogDaemonCaller,
	alias, path string,
	req og.Request,
) (og.Response, error) {
	entry, err := projects.Get(alias)
	if err != nil {
		return og.Response{}, fmt.Errorf("resolve project: %w", err)
	}
	req.WorkDir = entry.Path
	resp, err := caller.CallContext(ctx, path, req)
	if err != nil {
		return og.Response{}, fmt.Errorf("call og daemon: %w", err)
	}
	return resp, nil
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

func newOGMCPServer(projects *project.Store, caller ogDaemonCaller) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-og", Version: "1.0.0"}, nil)

	mcp.AddTool(server, setInputSchema[ogCloneInput](ogTool(
		"clone", "Clone repository",
		"Clone an HTTP(S) repository to its daemon-derived project or reference path.",
		false, false, true,
	), false), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogCloneInput,
	) (*mcp.CallToolResult, ogCloneOutput, error) {
		if err := validateCloneSelector(input.Project, input.URL, input.Alias, input.Reference); err != nil {
			return nil, ogCloneOutput{}, err
		}
		resp, err := caller.CallContext(ctx, "/git/clone", og.Request{
			Project: input.Project, URL: input.URL, Alias: input.Alias, Reference: input.Reference,
		})
		if err != nil {
			return nil, ogCloneOutput{}, fmt.Errorf("call og daemon: %w", err)
		}
		if err := og.ValidateCloneResponse(resp); err != nil {
			return nil, ogCloneOutput{}, err
		}
		return nil, ogCloneOutput{Clone: *resp.Clone}, nil
	})

	mcp.AddTool(server, setInputSchema[ogProjectInput](ogTool(
		"auth_status", "Inspect forge authentication",
		"Inspect secret-free forge authentication status for one registered project.", true, false, true,
	), false), authStatusHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPushInput](ogTool(
		"push", "Push current branch",
		"Push the registered checkout's current branch; force uses force-with-lease and is rejected on the default branch.",
		false, true, true,
	), false), pushHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogProjectInput](ogTool(
		"pull", "Pull current branch",
		"Run the CLI-equivalent pull workflow, including fast-forward-only pulls and guarded closed-PR branch cleanup.",
		false, true, true,
	), false), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogProjectInput,
	) (*mcp.CallToolResult, ogMessageOutput, error) {
		return callMessageTool(ctx, projects, caller, input.Project, "/git/pull", og.Request{})
	})

	mcp.AddTool(server, setInputSchema[ogPRCreateInput](ogTool(
		"pr_create", "Create pull request",
		"Push the registered checkout's current branch and create its pull request.", false, false, false,
	), false), prCreateHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPRFindInput](ogTool(
		"pr_find", "Find current branch pull request",
		"Find a pull request for the registered checkout's current branch by state.", true, false, true,
	), false), prFindHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(
		"pr_get", "Get pull request",
		"Get by positive PR ID, or view the registered checkout's current branch pull request and CI when omitted.",
		true, false, true,
	), false), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		if input.PRID == nil {
			return callWorktreePRTool(
				ctx, projects, caller, input.Project, "/pr/view", og.Request{State: og.PRStateAll},
			)
		}
		return callPRTool(ctx, projects, caller, input.Project, "/pr/get", *input.PRID)
	})

	mcp.AddTool(server, setInputSchema[ogPRModifyInput](ogTool(
		"pr_modify", "Modify pull request",
		"Modify title and/or body by positive PR ID or for the registered checkout's current branch when omitted.",
		false, true, true,
	), false), prModifyHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPRCommentInput](ogTool(
		"pr_comment", "Comment on pull request",
		"Comment by positive PR ID or on the registered checkout's current branch when omitted.", false, false, false,
	), false), prCommentHandler(projects, caller))

	addPRLinesTool(server, projects, caller, "pr_checks", "Inspect pull request checks", "/pr/checks", false)
	addPRLinesTool(server, projects, caller, "pr_log", "Inspect pull request CI log", "/pr/log", true)
	addPRLinesTool(server, projects, caller, "pr_failures", "Inspect pull request failures", "/pr/failures", true)

	return server
}

func pushHandler(
	projects *project.Store,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPushInput, ogMessageOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPushInput,
	) (*mcp.CallToolResult, ogMessageOutput, error) {
		return callMessageTool(ctx, projects, caller, input.Project, "/git/push", og.Request{Force: input.Force})
	}
}

func prCreateHandler(
	projects *project.Store,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPRCreateInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRCreateInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		if err := og.ValidatePRTitle(input.Title); err != nil {
			return nil, ogPROutput{}, err
		}
		return callWorktreePRTool(ctx, projects, caller, input.Project, "/pr/create", og.Request{
			Title: &input.Title,
			Body:  &input.Body,
		})
	}
}

func prFindHandler(
	projects *project.Store,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPRFindInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRFindInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		state, err := og.NormalizePRState(input.State)
		if err != nil {
			return nil, ogPROutput{}, err
		}
		return callWorktreePRTool(ctx, projects, caller, input.Project, "/pr/find", og.Request{State: state})
	}
}

func callMessageTool(
	ctx context.Context,
	projects *project.Store,
	caller ogDaemonCaller,
	alias, path string,
	req og.Request,
) (*mcp.CallToolResult, ogMessageOutput, error) {
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, req)
	if err != nil {
		return nil, ogMessageOutput{}, err
	}
	if err := og.ValidateMessageResponse(resp); err != nil {
		return nil, ogMessageOutput{}, err
	}
	return nil, ogMessageOutput{Project: alias, Message: resp.Message}, nil
}

func callWorktreePRTool(
	ctx context.Context,
	projects *project.Store,
	caller ogDaemonCaller,
	alias, path string,
	req og.Request,
) (*mcp.CallToolResult, ogPROutput, error) {
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, req)
	if err != nil {
		return nil, ogPROutput{}, err
	}
	if err := og.ValidateWorktreePR(resp); err != nil {
		return nil, ogPROutput{}, err
	}
	return nil, ogPROutput{Project: alias, PR: *resp.PR}, nil
}

func authStatusHandler(
	projects *project.Store,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogProjectInput, ogAuthOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogProjectInput,
	) (*mcp.CallToolResult, ogAuthOutput, error) {
		resp, err := callOGDaemon(ctx, projects, caller, input.Project, "/auth/status", og.Request{})
		if err != nil {
			return nil, ogAuthOutput{}, err
		}
		if err := og.ValidateAuthResponse(resp, input.Project); err != nil {
			return nil, ogAuthOutput{}, err
		}
		return nil, ogAuthOutput{Project: input.Project, Auth: *resp.Auth}, nil
	}
}

func prModifyHandler(
	projects *project.Store,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPRModifyInput, ogPROutput] {
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
		resp, err := callOGDaemon(ctx, projects, caller, input.Project, "/pr/modify", og.Request{
			Index: prID, Title: input.Title, Body: input.Body,
		})
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := og.ValidatePRModifyResponse(resp, prID, input.Title, input.Body); err != nil {
			return nil, ogPROutput{}, err
		}
		return nil, ogPROutput{Project: input.Project, PR: *resp.PR}, nil
	}
}

func prCommentHandler(
	projects *project.Store,
	caller ogDaemonCaller,
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
		resp, err := callOGDaemon(ctx, projects, caller, input.Project, "/pr/comment", og.Request{
			Index: prID, Body: &input.Body,
		})
		if err != nil {
			return nil, ogCommentOutput{}, err
		}
		if err := og.ValidateCommentResponse(resp, prID, input.Body); err != nil {
			return nil, ogCommentOutput{}, err
		}
		return nil, ogCommentOutput{Project: input.Project, Comment: *resp.Comment}, nil
	}
}

func callPRTool(
	ctx context.Context,
	projects *project.Store,
	caller ogDaemonCaller,
	alias, path string,
	id int64,
) (*mcp.CallToolResult, ogPROutput, error) {
	if err := og.ValidatePositivePRID(id); err != nil {
		return nil, ogPROutput{}, err
	}
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, og.Request{Index: id})
	if err != nil {
		return nil, ogPROutput{}, err
	}
	if err := og.ValidatePRResponse(resp, id); err != nil {
		return nil, ogPROutput{}, err
	}
	return nil, ogPROutput{Project: alias, PR: *resp.PR}, nil
}

func addPRLinesTool(
	server *mcp.Server,
	projects *project.Store,
	caller ogDaemonCaller,
	name, title, path string,
	withTail bool,
) {
	if !withTail {
		mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(
			name, title,
			"Inspect pull request state by positive PR ID or for the registered checkout's current branch when omitted.",
			true, false, true,
		), false), func(
			ctx context.Context,
			_ *mcp.CallToolRequest,
			input ogPRInput,
		) (*mcp.CallToolResult, ogPRLinesOutput, error) {
			return callPRLinesTool(ctx, projects, caller, input.Project, path, input.PRID, 0)
		})
		return
	}
	mcp.AddTool(server, setInputSchema[ogPRTailInput](ogTool(
		name, title,
		"Inspect pull request CI state and logs by positive PR ID or for the registered checkout's "+
			"current branch when omitted.",
		true, false, true,
	), true), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRTailInput,
	) (*mcp.CallToolResult, ogPRLinesOutput, error) {
		return callPRLinesTool(ctx, projects, caller, input.Project, path, input.PRID, input.Tail)
	})
}

func callPRLinesTool(
	ctx context.Context,
	projects *project.Store,
	caller ogDaemonCaller,
	alias, path string,
	idInput *int64,
	tail int,
) (*mcp.CallToolResult, ogPRLinesOutput, error) {
	id, err := optionalMCPPRID(idInput)
	if err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	if err := og.ValidatePRLogTail(tail); err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, og.Request{Index: id, Tail: tail})
	if err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	if err := og.ValidatePRResponse(resp, id); err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	return nil, ogPRLinesOutput{Project: alias, PR: *resp.PR, Lines: resp.Lines}, nil
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
		Short: "Serve alias-only forge tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projects := project.NewStore(config.ProjectsPath())
			if _, err := projects.Snapshot(); err != nil {
				return err
			}
			client := og.NewClientFromEnv()
			return newOGMCPServer(projects, client).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
