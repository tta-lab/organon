package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/srcview"
)

type sourcePathInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Path    string `json:"path" jsonschema:"repository-relative file path"`
}
type sourceListInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Path    string `json:"path,omitempty" jsonschema:"optional repository-relative directory path; defaults to root"`
}
type sourceReadInput struct {
	Project  string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Path     string `json:"path" jsonschema:"repository-relative file path"`
	SymbolID string `json:"symbol_id,omitempty" jsonschema:"opaque symbol or section ID"`
	Offset   *int   `json:"offset,omitempty" jsonschema:"one-indexed line offset; defaults to 1"`
	Limit    *int   `json:"limit,omitempty" jsonschema:"maximum lines; omitted reads up to 2000 lines"`
}
type sourceSearchInput struct {
	Project string `json:"project" jsonschema:"project reference or exact registered checkout path"`
	Pattern string `json:"pattern" jsonschema:"ripgrep regular-expression pattern; non-blank, maximum 4096 bytes"`
	Limit   *int   `json:"limit,omitempty" jsonschema:"maximum matches; defaults to 50, maximum 200"`
}
type sourceSymbolsOutput struct {
	Project string `json:"project"`
	srcview.SymbolResult
}
type sourceReadOutput struct {
	Project string `json:"project"`
	srcview.ReadResult
}
type sourceSearchOutput struct {
	Project string `json:"project"`
	srcview.SearchResult
}
type sourceListOutput struct {
	srcview.ListResult
}

const sourceProjectField = "project"

func sourceSchema[T any]() *jsonschema.Schema {
	schema := inputSchemaFor[T](false)
	for _, name := range []string{sourceProjectField, "path"} {
		if field := schema.Properties[name]; field != nil {
			field.MinLength = jsonschema.Ptr(1)
		}
	}
	return schema
}

func sourceTool(name, title, description string, schema *jsonschema.Schema) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Title: title, Description: description, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title: title, ReadOnlyHint: true, DestructiveHint: ogBoolPointer(false),
			IdempotentHint: true, OpenWorldHint: ogBoolPointer(false),
		},
	}
}
func addSourceTools(server *mcp.Server, projects *project.Store) {
	files := srcview.NewProjectService(projects)
	listSchema := sourceSchema[sourceListInput]()
	mcp.AddTool(server, sourceTool("source_list", "List source directory",
		"List up to 200 direct children of one registered project directory.", listSchema),
		func(_ context.Context, _ *mcp.CallToolRequest, in sourceListInput) (
			*mcp.CallToolResult, sourceListOutput, error,
		) {
			result, err := files.ListDirectory(in.Project, in.Path)
			if err != nil {
				return nil, sourceListOutput{}, err
			}
			return nil, sourceListOutput{ListResult: result}, nil
		})
	symbolsSchema := sourceSchema[sourcePathInput]()
	readSchema := sourceSchema[sourceReadInput]()
	readSchema.Properties["offset"].Minimum = jsonschema.Ptr(1.0)
	readSchema.Properties["offset"].Default = json.RawMessage("1")
	readSchema.Properties["limit"].Minimum = jsonschema.Ptr(0.0)
	searchSchema := sourceSchema[sourceSearchInput]()
	searchSchema.Properties["pattern"].MinLength = jsonschema.Ptr(1)
	searchSchema.Properties["pattern"].MaxLength = jsonschema.Ptr(4096)
	searchSchema.Properties["limit"].Minimum = jsonschema.Ptr(1.0)
	searchSchema.Properties["limit"].Maximum = jsonschema.Ptr(float64(srcview.MaxSearchLimit))
	searchSchema.Properties["limit"].Default = json.RawMessage("50")
	mcp.AddTool(server, sourceTool("source_symbols", "Inspect source symbols",
		"Inspect code symbols or Markdown headings in one registered project file.", symbolsSchema),
		func(_ context.Context, _ *mcp.CallToolRequest, in sourcePathInput) (
			*mcp.CallToolResult, sourceSymbolsOutput, error,
		) {
			f, err := files.ReadFile(in.Project, in.Path)
			if err != nil {
				return nil, sourceSymbolsOutput{}, err
			}
			outline, err := srcview.BuildSymbolResult(f.Path, f.Source, false)
			if err != nil {
				return nil, sourceSymbolsOutput{}, err
			}
			return nil, sourceSymbolsOutput{Project: f.Project, SymbolResult: outline}, nil
		})
	mcp.AddTool(server, sourceTool("source_read", "Read source text",
		"Read a project file or opaque symbol/section ID with bounded line pagination.", readSchema),
		func(_ context.Context, _ *mcp.CallToolRequest, in sourceReadInput) (
			*mcp.CallToolResult, sourceReadOutput, error,
		) {
			offset, limit := 0, 0
			if in.Offset != nil {
				offset = *in.Offset
				if offset < 1 {
					return nil, sourceReadOutput{}, fmt.Errorf("offset must be 1 or greater")
				}
			}
			if in.Limit != nil {
				limit = *in.Limit
				if limit < 0 {
					return nil, sourceReadOutput{}, fmt.Errorf("limit must be zero or greater")
				}
			}
			f, err := files.ReadFile(in.Project, in.Path)
			if err != nil {
				return nil, sourceReadOutput{}, err
			}
			result, err := srcview.BuildReadResult(f.Path, f.Source, in.SymbolID,
				offset, limit, in.Limit != nil)
			if err != nil {
				return nil, sourceReadOutput{}, err
			}
			return nil, sourceReadOutput{Project: f.Project, ReadResult: result}, nil
		})
	mcp.AddTool(server, sourceTool("source_search", "Search source text",
		"Search a ripgrep regular-expression pattern under one registered checkout; "+
			"return bounded structured matches.", searchSchema),
		func(ctx context.Context, _ *mcp.CallToolRequest, in sourceSearchInput) (
			*mcp.CallToolResult, sourceSearchOutput, error,
		) {
			entry, err := files.Resolve(in.Project)
			if err != nil {
				return nil, sourceSearchOutput{}, err
			}
			limit := srcview.DefaultSearchLimit
			if in.Limit != nil {
				limit = *in.Limit
			}
			result, err := srcview.Search(ctx, entry.Path, in.Pattern, limit)
			if err != nil {
				return nil, sourceSearchOutput{}, err
			}
			return nil, sourceSearchOutput{Project: entry.Alias, SearchResult: result}, nil
		})
}
