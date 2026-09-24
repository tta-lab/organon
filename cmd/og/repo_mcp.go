package main

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

type repoDiffInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Path    string `json:"path,omitempty" jsonschema:"optional repository-relative file or directory path"`
}

type repoDiffOutput struct {
	Project string `json:"project"`
	og.WorkingDiffResult
}

func addRepoTools(server *mcp.Server, projects *project.Store) {
	schema := inputSchemaFor[repoDiffInput](false)
	if field := schema.Properties["project"]; field != nil {
		field.MinLength = jsonschema.Ptr(1)
	}
	mcp.AddTool(server, sourceTool(
		"repo_diff", "Inspect repository working diff",
		"Return the tracked unified diff from the default-branch merge base through the working tree, "+
			"plus untracked paths. Uses local origin state without fetching.",
		schema,
	), func(ctx context.Context, _ *mcp.CallToolRequest, in repoDiffInput) (
		*mcp.CallToolResult, repoDiffOutput, error,
	) {
		entry, err := resolveOGProject(projects, in.Project)
		if err != nil {
			return nil, repoDiffOutput{}, err
		}
		result, err := og.WorkingDiff(ctx, entry.Path, in.Path)
		if err != nil {
			return nil, repoDiffOutput{}, err
		}
		return nil, repoDiffOutput{Project: entry.Alias, WorkingDiffResult: result}, nil
	})
}
