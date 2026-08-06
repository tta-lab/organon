package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/org"
	"github.com/tta-lab/organon/internal/project"
)

type projectListInput struct {
	Org string `json:"org,omitempty" jsonschema:"optional exact org name used to filter projects"`
}

type projectGetInput struct {
	Alias string `json:"alias" jsonschema:"exact registered single-layer project alias"`
}

type orgGetInput struct {
	Name string `json:"name" jsonschema:"exact registered single-layer org name"`
}

type projectListOutput struct {
	Projects []project.Entry `json:"projects"`
}

type projectGetOutput struct {
	Project project.Entry `json:"project"`
}

type orgListOutput struct {
	Orgs []org.Entry `json:"orgs"`
}

type orgGetOutput struct {
	Org org.Entry `json:"org"`
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

func newProjectMCPServer(projects *project.Catalog, orgs *org.Catalog) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-project", Version: "1.0.0"}, nil)

	projectListHandler := func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		input projectListInput,
	) (*mcp.CallToolResult, projectListOutput, error) {
		return nil, projectListOutput{Projects: projects.List(input.Org)}, nil
	}
	mcp.AddTool(server, discoveryTool(
		"project_list",
		"List registered projects",
		"List registered projects, optionally filtered by exact org name.",
	), projectListHandler)

	projectGetHandler := func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		input projectGetInput,
	) (*mcp.CallToolResult, projectGetOutput, error) {
		entry, err := projects.GetExact(input.Alias)
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

	mcp.AddTool(server, discoveryTool(
		"org_list",
		"List registered orgs",
		"List registered organizations used by Organon project metadata.",
	), func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, orgListOutput, error) {
		return nil, orgListOutput{Orgs: orgs.List()}, nil
	})

	mcp.AddTool(server, discoveryTool(
		"org_get",
		"Get registered org",
		"Get one registered organization by exact single-layer name.",
	), func(_ context.Context, _ *mcp.CallToolRequest, input orgGetInput) (*mcp.CallToolResult, orgGetOutput, error) {
		entry, err := orgs.GetExact(input.Name)
		if err != nil {
			return nil, orgGetOutput{}, fmt.Errorf("get org: %w", err)
		}
		return nil, orgGetOutput{Org: entry}, nil
	})

	return server
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve typed project and org discovery tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projects, err := project.OpenCatalog(config.ProjectsPath())
			if err != nil {
				return err
			}
			orgs, err := org.OpenCatalog(config.OrgsPath())
			if err != nil {
				return err
			}
			return newProjectMCPServer(projects, orgs).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
