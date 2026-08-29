package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/project"
)

type projectListInput struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"include archived projects; defaults to false"`
}

type projectGetInput struct {
	Project string `json:"project" jsonschema:"project reference: alias, checkout basename, or remote basename"`
}

type projectFindInput struct {
	Query string `json:"query" jsonschema:"non-blank natural-language query for active projects and references"`
	Limit *int   `json:"limit,omitempty" jsonschema:"maximum results; defaults to 8 and is capped at 32"`
}

type projectListOutput struct {
	Projects []project.Entry `json:"projects"`
}

type projectGetOutput struct {
	Project project.Entry `json:"project"`
}

func boolPointer(value bool) *bool { return &value }

func discoveryTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:          title,
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  boolPointer(false),
		},
	}
}

func newProjectMCPServer(projects *project.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-project", Version: "1.0.0"}, nil)

	projectListHandler := func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		input projectListInput,
	) (*mcp.CallToolResult, projectListOutput, error) {
		entries, err := projects.List(input.IncludeArchived)
		if err != nil {
			return nil, projectListOutput{}, fmt.Errorf("list projects: %w", err)
		}
		return nil, projectListOutput{Projects: entries}, nil
	}
	mcp.AddTool(server, discoveryTool(
		"project_list",
		"List registered projects",
		"List registered projects, optionally including archived entries.",
	), projectListHandler)

	projectGetHandler := func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		input projectGetInput,
	) (*mcp.CallToolResult, projectGetOutput, error) {
		entry, err := projects.Resolve(input.Project)
		if err != nil {
			return nil, projectGetOutput{}, fmt.Errorf("get project: %w", err)
		}
		return nil, projectGetOutput{Project: entry}, nil
	}
	mcp.AddTool(server, discoveryTool(
		"project_find",
		"Find projects and references",
		"Find active projects and locally cloned references by alias, display name, checkout name, or repository name. "+
			"A registered project takes precedence over a same-named reference.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input projectFindInput,
	) (*mcp.CallToolResult, projectListOutput, error) {
		limit := project.DefaultFindLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		entries, err := projects.Find(input.Query, limit)
		if err != nil {
			return nil, projectListOutput{}, fmt.Errorf("find projects: %w", err)
		}
		return nil, projectListOutput{Projects: entries}, nil
	})

	mcp.AddTool(server, discoveryTool(
		"project_get",
		"Get registered project",
		"Get one registered project by an exact case-insensitive project reference and return its canonical alias.",
	), projectGetHandler)

	return server
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve typed project discovery tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return newProjectMCPServer(discoveryStore()).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
