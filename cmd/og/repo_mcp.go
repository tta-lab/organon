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
type repoStatusInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
}
type repoStatusOutput struct {
	Project string `json:"project"`
	og.RepositoryStatus
}
type repoCommitsInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Ref     string `json:"ref,omitempty" jsonschema:"head or default; defaults to head"`
	Limit   *int   `json:"limit,omitempty" jsonschema:"number of commits; defaults to 20, maximum 100"`
}
type repoCommitsOutput struct {
	Project string `json:"project"`
	og.CommitListResult
}
type repoCompareInput struct {
	Project    string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	BaseCommit string `json:"base_commit" jsonschema:"full base commit ID"`
	HeadCommit string `json:"head_commit" jsonschema:"full head commit ID"`
	Path       string `json:"path,omitempty" jsonschema:"optional repository-relative file or directory path"`
}
type repoCompareOutput struct {
	Project string `json:"project"`
	og.CommitCompareResult
}

func addRepoTools(server *mcp.Server, projects *project.Store) {
	statusSchema := inputSchemaFor[repoStatusInput](false)
	statusSchema.Properties[sourceProjectField].MinLength = jsonschema.Ptr(1)
	mcp.AddTool(server, sourceTool(
		"repo_status", "Inspect repository status",
		"Return local branch and HEAD plus staged, unstaged, untracked, and conflicted paths.",
		statusSchema,
	), func(ctx context.Context, _ *mcp.CallToolRequest, in repoStatusInput) (
		*mcp.CallToolResult, repoStatusOutput, error,
	) {
		entry, err := resolveOGProject(projects, in.Project)
		if err != nil {
			return nil, repoStatusOutput{}, err
		}
		result, err := og.RepoStatus(ctx, entry.Path)
		if err != nil {
			return nil, repoStatusOutput{}, err
		}
		return nil, repoStatusOutput{Project: entry.Alias, RepositoryStatus: result}, nil
	})
	commitsSchema := inputSchemaFor[repoCommitsInput](false)
	commitsSchema.Properties[sourceProjectField].MinLength = jsonschema.Ptr(1)
	commitsSchema.Properties["limit"].Minimum = jsonschema.Ptr(1.0)
	commitsSchema.Properties["limit"].Maximum = jsonschema.Ptr(float64(og.MaxCommitLimit))
	mcp.AddTool(server, sourceTool(
		"repo_commits", "List repository commits",
		"List recent commits from HEAD or the local origin default branch without fetching.",
		commitsSchema,
	), func(ctx context.Context, _ *mcp.CallToolRequest, in repoCommitsInput) (
		*mcp.CallToolResult, repoCommitsOutput, error,
	) {
		entry, err := resolveOGProject(projects, in.Project)
		if err != nil {
			return nil, repoCommitsOutput{}, err
		}
		limit := og.DefaultCommitLimit
		if in.Limit != nil {
			limit = *in.Limit
		}
		result, err := og.RepoCommits(ctx, entry.Path, in.Ref, limit)
		if err != nil {
			return nil, repoCommitsOutput{}, err
		}
		return nil, repoCommitsOutput{Project: entry.Alias, CommitListResult: result}, nil
	})
	compareSchema := inputSchemaFor[repoCompareInput](false)
	for _, field := range []string{sourceProjectField, "base_commit", "head_commit"} {
		compareSchema.Properties[field].MinLength = jsonschema.Ptr(1)
	}
	mcp.AddTool(server, sourceTool(
		"repo_compare", "Compare repository commits",
		"Return a bounded patch and file summaries between two exact commit IDs; excludes working-tree changes.",
		compareSchema,
	), func(ctx context.Context, _ *mcp.CallToolRequest, in repoCompareInput) (
		*mcp.CallToolResult, repoCompareOutput, error,
	) {
		entry, err := resolveOGProject(projects, in.Project)
		if err != nil {
			return nil, repoCompareOutput{}, err
		}
		result, err := og.RepoCompare(ctx, entry.Path, in.BaseCommit, in.HeadCommit, in.Path)
		if err != nil {
			return nil, repoCompareOutput{}, err
		}
		return nil, repoCompareOutput{Project: entry.Alias, CommitCompareResult: result}, nil
	})
	schema := inputSchemaFor[repoDiffInput](false)
	if field := schema.Properties[sourceProjectField]; field != nil {
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
