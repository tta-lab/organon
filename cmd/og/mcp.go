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

const (
	defaultLogTail = 50
	maximumLogTail = 1000
)

type ogDaemonCaller interface {
	CallContext(context.Context, string, og.Request) (og.Response, error)
}

type ogProjectInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
}

type ogPRInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    int64  `json:"pr_id" jsonschema:"positive pull request ID"`
}

type ogPRModifyInput struct {
	Project string  `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    int64   `json:"pr_id" jsonschema:"positive pull request ID"`
	Title   *string `json:"title,omitempty" jsonschema:"optional replacement pull request title"`
	Body    *string `json:"body,omitempty" jsonschema:"optional replacement pull request body; an empty string clears it"`
}

type ogPRCommentInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    int64  `json:"pr_id" jsonschema:"positive pull request ID"`
	Body    string `json:"body" jsonschema:"non-blank pull request comment body"`
}

type ogPRTailInput struct {
	Project string `json:"project" jsonschema:"exact registered single-layer project alias"`
	PRID    int64  `json:"pr_id" jsonschema:"positive pull request ID"`
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
		tailSchema.Maximum = jsonschema.Ptr(float64(maximumLogTail))
		tailSchema.Default = json.RawMessage(fmt.Sprint(defaultLogTail))
	}
	return schema
}

func setInputSchema[T any](tool *mcp.Tool, tail bool) *mcp.Tool {
	tool.InputSchema = inputSchemaFor[T](tail)
	return tool
}

func callOGDaemon(
	ctx context.Context,
	projects *project.Catalog,
	caller ogDaemonCaller,
	alias, path string,
	req og.Request,
) (og.Response, error) {
	entry, err := projects.GetExact(alias)
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

func validatePositivePRID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("PR ID must be positive")
	}
	return nil
}

func newOGMCPServer(projects *project.Catalog, caller ogDaemonCaller) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-og", Version: "1.0.0"}, nil)

	mcp.AddTool(server, setInputSchema[ogProjectInput](ogTool(
		"auth_status", "Inspect forge authentication",
		"Inspect secret-free forge authentication status for one registered project.", true, false, true,
	), false), authStatusHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(
		"pr_get", "Get pull request", "Get one pull request by explicit positive ID.", true, false, true,
	), false), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		return callPRTool(ctx, projects, caller, input.Project, "/pr/get", input.PRID)
	})

	mcp.AddTool(server, setInputSchema[ogPRModifyInput](ogTool(
		"pr_modify", "Modify pull request",
		"Modify title and/or body for one pull request by explicit positive ID.", false, true, true,
	), false), prModifyHandler(projects, caller))

	mcp.AddTool(server, setInputSchema[ogPRCommentInput](ogTool(
		"pr_comment", "Comment on pull request",
		"Create a comment on one pull request by explicit positive ID.", false, false, false,
	), false), prCommentHandler(projects, caller))

	addPRLinesTool(server, projects, caller, "pr_checks", "Inspect pull request checks", "/pr/checks", false)
	addPRLinesTool(server, projects, caller, "pr_log", "Inspect pull request CI log", "/pr/log", true)
	addPRLinesTool(server, projects, caller, "pr_failures", "Inspect pull request failures", "/pr/failures", true)

	return server
}

func authStatusHandler(
	projects *project.Catalog,
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
		if resp.Auth == nil {
			return nil, ogAuthOutput{}, fmt.Errorf("og daemon returned no authentication status")
		}
		if resp.Auth.Project != input.Project {
			return nil, ogAuthOutput{}, fmt.Errorf(
				"og daemon returned authentication status for project %q, want %q",
				resp.Auth.Project,
				input.Project,
			)
		}
		return nil, ogAuthOutput{Project: input.Project, Auth: *resp.Auth}, nil
	}
}

func prModifyHandler(
	projects *project.Catalog,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPRModifyInput, ogPROutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRModifyInput,
	) (*mcp.CallToolResult, ogPROutput, error) {
		if err := validatePositivePRID(input.PRID); err != nil {
			return nil, ogPROutput{}, err
		}
		if input.Title == nil && input.Body == nil {
			return nil, ogPROutput{}, fmt.Errorf("nothing to update: provide title and/or body")
		}
		if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
			return nil, ogPROutput{}, fmt.Errorf("PR title must not be blank")
		}
		resp, err := callOGDaemon(ctx, projects, caller, input.Project, "/pr/modify", og.Request{
			Index: input.PRID, Title: input.Title, Body: input.Body,
		})
		if err != nil {
			return nil, ogPROutput{}, err
		}
		if err := validateDaemonPR(resp.PR, input.PRID); err != nil {
			return nil, ogPROutput{}, err
		}
		if input.Title != nil && resp.PR.Title != *input.Title {
			return nil, ogPROutput{}, fmt.Errorf("og daemon returned pull request with unexpected title")
		}
		if input.Body != nil && resp.PR.Body != *input.Body {
			return nil, ogPROutput{}, fmt.Errorf("og daemon returned pull request with unexpected body")
		}
		return nil, ogPROutput{Project: input.Project, PR: *resp.PR}, nil
	}
}

func prCommentHandler(
	projects *project.Catalog,
	caller ogDaemonCaller,
) mcp.ToolHandlerFor[ogPRCommentInput, ogCommentOutput] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input ogPRCommentInput,
	) (*mcp.CallToolResult, ogCommentOutput, error) {
		if err := validatePositivePRID(input.PRID); err != nil {
			return nil, ogCommentOutput{}, err
		}
		if strings.TrimSpace(input.Body) == "" {
			return nil, ogCommentOutput{}, fmt.Errorf("comment body must not be blank")
		}
		resp, err := callOGDaemon(ctx, projects, caller, input.Project, "/pr/comment", og.Request{
			Index: input.PRID, Body: &input.Body,
		})
		if err != nil {
			return nil, ogCommentOutput{}, err
		}
		if err := validateDaemonComment(resp.Comment, input.PRID, input.Body); err != nil {
			return nil, ogCommentOutput{}, err
		}
		return nil, ogCommentOutput{Project: input.Project, Comment: *resp.Comment}, nil
	}
}

func callPRTool(
	ctx context.Context,
	projects *project.Catalog,
	caller ogDaemonCaller,
	alias, path string,
	id int64,
) (*mcp.CallToolResult, ogPROutput, error) {
	if err := validatePositivePRID(id); err != nil {
		return nil, ogPROutput{}, err
	}
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, og.Request{Index: id})
	if err != nil {
		return nil, ogPROutput{}, err
	}
	if err := validateDaemonPR(resp.PR, id); err != nil {
		return nil, ogPROutput{}, err
	}
	return nil, ogPROutput{Project: alias, PR: *resp.PR}, nil
}

func addPRLinesTool(
	server *mcp.Server,
	projects *project.Catalog,
	caller ogDaemonCaller,
	name, title, path string,
	withTail bool,
) {
	if !withTail {
		mcp.AddTool(server, setInputSchema[ogPRInput](ogTool(
			name, title, "Inspect remote pull request state by explicit positive ID.", true, false, true,
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
		name, title, "Inspect remote pull request CI state and log lines by explicit positive ID.", true, false, true,
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
	projects *project.Catalog,
	caller ogDaemonCaller,
	alias, path string,
	id int64,
	tail int,
) (*mcp.CallToolResult, ogPRLinesOutput, error) {
	if err := validatePositivePRID(id); err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	if tail < 0 || tail > maximumLogTail {
		return nil, ogPRLinesOutput{}, fmt.Errorf("tail must be between 0 and %d", maximumLogTail)
	}
	resp, err := callOGDaemon(ctx, projects, caller, alias, path, og.Request{Index: id, Tail: tail})
	if err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	if err := validateDaemonPR(resp.PR, id); err != nil {
		return nil, ogPRLinesOutput{}, err
	}
	return nil, ogPRLinesOutput{Project: alias, PR: *resp.PR, Lines: resp.Lines}, nil
}

func validateDaemonPR(pr *og.PullRequest, expectedID int64) error {
	if pr == nil {
		return fmt.Errorf("og daemon returned no pull request")
	}
	if pr.Index != expectedID {
		return fmt.Errorf("og daemon returned PR ID %d, want %d", pr.Index, expectedID)
	}
	return nil
}

func validateDaemonComment(comment *og.Comment, expectedPRID int64, expectedBody string) error {
	if comment == nil {
		return fmt.Errorf("og daemon returned no comment")
	}
	identityMismatch := comment.ID <= 0 || comment.PRID != expectedPRID
	contentMismatch := comment.Body != expectedBody || strings.TrimSpace(comment.URL) == ""
	if identityMismatch || contentMismatch {
		return fmt.Errorf("og daemon returned an invalid comment result")
	}
	return nil
}

func newOGMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve alias-only forge tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projects, err := project.OpenCatalog(config.ProjectsPath())
			if err != nil {
				return err
			}
			client := og.NewClientFromEnv()
			return newOGMCPServer(projects, client).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
