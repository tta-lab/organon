package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	webcore "github.com/tta-lab/organon/internal/web"
)

const defaultFetchTreeThreshold = 5000

type webSearchInput struct {
	Query string `json:"query" jsonschema:"web search query"`
}

type webFetchInput struct {
	URL           string `json:"url" jsonschema:"HTTP or HTTPS URL to fetch"`
	ShowTree      bool   `json:"tree,omitempty" jsonschema:"show the page heading tree"`
	SectionID     string `json:"section_id,omitempty" jsonschema:"optional heading section ID to return"`
	Full          bool   `json:"full,omitempty" jsonschema:"return full content without automatic tree mode"`
	TreeThreshold *int   `json:"tree_threshold,omitempty" jsonschema:"automatic tree threshold; defaults to 5000"`
}

type webDocsResolveInput struct {
	Query string `json:"query" jsonschema:"library name or package query"`
}

type webDocsFetchInput struct {
	LibraryID string `json:"library_id" jsonschema:"Context7 library ID returned by docs_resolve"`
	Topic     string `json:"topic,omitempty" jsonschema:"optional documentation topic"`
	Tokens    int    `json:"tokens,omitempty" jsonschema:"optional token budget; zero uses the backend default"`
}

type webSGraphInput struct {
	Query         string `json:"query" jsonschema:"Sourcegraph search query"`
	Count         int    `json:"count,omitempty" jsonschema:"optional result count; defaults to 10"`
	ContextWindow int    `json:"context,omitempty" jsonschema:"optional context lines; defaults to 10"`
	Timeout       int    `json:"timeout,omitempty" jsonschema:"optional timeout in seconds; zero disables the timeout"`
}

func webBoolPointer(value bool) *bool { return &value }

func webTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    true,
			DestructiveHint: webBoolPointer(false),
			IdempotentHint:  true,
			OpenWorldHint:   webBoolPointer(true),
		},
	}
}

func webInputSchema[T any](defaults map[string]int) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("infer MCP input schema for %s: %v", reflect.TypeFor[T](), err))
	}
	for field, value := range defaults {
		property := schema.Properties[field]
		if property == nil {
			panic(fmt.Sprintf("MCP input schema for %s has no %q field", reflect.TypeFor[T](), field))
		}
		property.Default = json.RawMessage(fmt.Sprint(value))
	}
	return schema
}

func webToolWithSchema[T any](tool *mcp.Tool, defaults map[string]int) *mcp.Tool {
	tool.InputSchema = webInputSchema[T](defaults)
	return tool
}

func newWebMCPServer(service webService) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-web", Version: "1.0.0"}, nil)

	mcp.AddTool(server, webToolWithSchema[webSearchInput](webTool(
		"search", "Search the web", "Search the web and return the selected provider with ranked results.",
	), nil), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input webSearchInput,
	) (*mcp.CallToolResult, webcore.SearchResult, error) {
		result, err := service.Search(ctx, input.Query)
		return nil, result, err
	})

	mcp.AddTool(server, webToolWithSchema[webFetchInput](webTool(
		"fetch", "Fetch a web page", "Fetch a web page and return rendered Markdown content.",
	), map[string]int{"tree_threshold": defaultFetchTreeThreshold}), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input webFetchInput,
	) (*mcp.CallToolResult, webcore.FetchResult, error) {
		threshold := defaultFetchTreeThreshold
		if input.TreeThreshold != nil {
			threshold = *input.TreeThreshold
		}
		result, err := service.Fetch(ctx, webcore.FetchInput{
			URL:           input.URL,
			ShowTree:      input.ShowTree,
			SectionID:     input.SectionID,
			Full:          input.Full,
			TreeThreshold: threshold,
		})
		return nil, result, err
	})

	mcp.AddTool(server, webToolWithSchema[webDocsResolveInput](webTool(
		"docs_resolve", "Resolve a documentation library",
		"Resolve a library or package query to typed Context7 library IDs.",
	), nil), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input webDocsResolveInput,
	) (*mcp.CallToolResult, webcore.DocsResolveResult, error) {
		result, err := service.DocsResolve(ctx, input.Query)
		return nil, result, err
	})

	mcp.AddTool(server, webToolWithSchema[webDocsFetchInput](webTool(
		"docs_fetch", "Fetch library documentation",
		"Fetch documentation for a Context7 library ID and optional topic.",
	), map[string]int{"tokens": 0}), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input webDocsFetchInput,
	) (*mcp.CallToolResult, webcore.DocsFetchResult, error) {
		result, err := service.DocsFetch(ctx, webcore.DocsFetchInput{
			LibraryID: input.LibraryID,
			Topic:     input.Topic,
			Tokens:    input.Tokens,
		})
		return nil, result, err
	})

	mcp.AddTool(server, webToolWithSchema[webSGraphInput](webTool(
		"sgraph_search", "Search public source code",
		"Search public source code through Sourcegraph and return Markdown results.",
	), map[string]int{"count": 10, "context": 10, "timeout": 0}), func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		input webSGraphInput,
	) (*mcp.CallToolResult, webcore.SGraphResult, error) {
		result, err := service.SGraphSearch(ctx, webcore.SGraphInput{
			Query:         input.Query,
			Count:         input.Count,
			ContextWindow: input.ContextWindow,
			Timeout:       input.Timeout,
		})
		return nil, result, err
	})

	return server
}

func newWebMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve typed web tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := defaultServiceFactory(config.WebConfigPath())
			if err != nil {
				return err
			}
			return newWebMCPServer(service).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
