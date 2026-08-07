package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
)

type projectListInput struct {
	IncludeArchived bool `json:"include_archived,omitempty" jsonschema:"include archived projects; defaults to false"`
}

type projectGetInput struct {
	Alias string `json:"alias" jsonschema:"exact registered single-layer project alias"`
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
		entry, err := projects.Get(input.Alias)
		if err != nil {
			return nil, projectGetOutput{}, fmt.Errorf("get project: %w", err)
		}
		return nil, projectGetOutput{Project: entry}, nil
	}
	mcp.AddTool(server, discoveryTool(
		"project_get",
		"Get registered project",
		"Get one registered project by exact single-layer alias.",
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
			return newProjectMCPServer(project.NewStore(config.ProjectsPath())).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
