package main

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/srcview"
)

const (
	defaultReadLimit = 64 * 1024
	maximumReadLimit = 256 * 1024
	outlineDepth     = 32
)

type srcSymbolsInput struct {
	Project string `json:"project" jsonschema:"exact registered project alias"`
	Path    string `json:"path" jsonschema:"repository-relative file path"`
}

type srcReadInput struct {
	Project  string `json:"project" jsonschema:"exact registered project alias"`
	Path     string `json:"path" jsonschema:"repository-relative file path"`
	SymbolID string `json:"symbol_id,omitempty" jsonschema:"exact symbol or Markdown section ID; omit for a byte range"`
	Offset   int    `json:"offset,omitempty" jsonschema:"zero-based UTF-8 byte offset for range reads; defaults to 0"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max bytes; defaults to 65536 and cannot exceed 262144"`
}

type srcSymbolsOutput struct {
	Project    string           `json:"project"`
	Path       string           `json:"path"`
	Language   string           `json:"language"`
	TotalBytes int              `json:"total_bytes"`
	Title      string           `json:"title,omitempty"`
	Symbols    []srcview.Symbol `json:"symbols"`
}

type srcReadOutput struct {
	Project    string `json:"project"`
	Path       string `json:"path"`
	Content    string `json:"content"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	NextOffset *int   `json:"next_offset,omitempty"`
	Total      int    `json:"total"`
	Truncated  bool   `json:"truncated"`
}

type srcMCPService struct {
	files *srcview.ProjectService
}

func srcBoolPointer(value bool) *bool { return &value }

func srcTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Title: title, Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title: title, ReadOnlyHint: true, DestructiveHint: srcBoolPointer(false),
			IdempotentHint: true, OpenWorldHint: srcBoolPointer(false),
		},
	}
}

func normalizedReadLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultReadLimit, nil
	}
	if limit < 1 || limit > maximumReadLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d bytes", maximumReadLimit)
	}
	return limit, nil
}

func (s srcMCPService) symbols(
	_ context.Context, _ *mcp.CallToolRequest, input srcSymbolsInput,
) (*mcp.CallToolResult, srcSymbolsOutput, error) {
	file, err := s.files.ReadFile(input.Project, input.Path)
	if err != nil {
		return nil, srcSymbolsOutput{}, err
	}
	outline, err := srcview.NewInspector(file.Path, file.Source, outlineDepth).Outline()
	if err != nil {
		return nil, srcSymbolsOutput{}, fmt.Errorf("inspect %s:%s: %w", file.Project, file.Path, err)
	}
	return nil, srcSymbolsOutput{
		Project: file.Project, Path: file.Path, Language: outline.Language,
		TotalBytes: len(file.Source), Title: outline.Title, Symbols: outline.Symbols,
	}, nil
}

func (s srcMCPService) read(
	_ context.Context, _ *mcp.CallToolRequest, input srcReadInput,
) (*mcp.CallToolResult, srcReadOutput, error) {
	limit, err := normalizedReadLimit(input.Limit)
	if err != nil {
		return nil, srcReadOutput{}, err
	}
	if input.Offset < 0 {
		return nil, srcReadOutput{}, fmt.Errorf("offset must be zero or greater")
	}
	if input.SymbolID != "" && input.Offset != 0 {
		return nil, srcReadOutput{}, fmt.Errorf("symbol_id and offset cannot be used together")
	}
	file, err := s.files.ReadFile(input.Project, input.Path)
	if err != nil {
		return nil, srcReadOutput{}, err
	}
	result, err := readSource(file, input, limit)
	if err != nil {
		return nil, srcReadOutput{}, err
	}
	return nil, result, nil
}

func readSource(file srcview.File, input srcReadInput, limit int) (srcReadOutput, error) {
	var content string
	var start, end int
	truncated := false
	if input.SymbolID != "" {
		result, err := srcview.NewInspector(file.Path, file.Source, outlineDepth).Read(input.SymbolID, limit)
		if err != nil {
			return srcReadOutput{}, fmt.Errorf("read %s:%s: %w", file.Project, file.Path, err)
		}
		content, start, end, truncated = result.Content, result.Start, result.End, result.Truncated
	} else {
		var err error
		content, start, end, truncated, err = readRange(file.Source, input.Offset, limit)
		if err != nil {
			return srcReadOutput{}, err
		}
	}
	output := srcReadOutput{
		Project: file.Project, Path: file.Path, Content: content,
		Start: start, End: end, Total: len(file.Source), Truncated: truncated,
	}
	if truncated {
		output.NextOffset = &end
	}
	return output, nil
}

func readRange(source []byte, offset, limit int) (string, int, int, bool, error) {
	if offset > len(source) {
		return "", 0, 0, false, fmt.Errorf("offset %d exceeds file size %d", offset, len(source))
	}
	if !utf8Boundary(source, offset) {
		return "", 0, 0, false, fmt.Errorf("offset %d is not a UTF-8 boundary", offset)
	}
	end := offset + limit
	truncated := end < len(source)
	if !truncated {
		end = len(source)
	}
	for end > offset && !utf8Boundary(source, end) {
		end--
	}
	if truncated && end == offset {
		return "", 0, 0, false, fmt.Errorf("limit %d is too small for the next UTF-8 character", limit)
	}
	return string(source[offset:end]), offset, end, truncated, nil
}

func newSrcMCPServer(files *srcview.ProjectService) *mcp.Server {
	service := srcMCPService{files: files}
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-src", Version: "1.0.0"}, nil)
	mcp.AddTool(server, srcTool(
		"symbols", "Inspect source symbols",
		"Inspect one file in a registered project and return code symbols or Markdown headings with IDs.",
	), service.symbols)
	mcp.AddTool(server, srcTool(
		"read", "Read source text",
		"Read one symbol or Markdown section by ID, or read a bounded UTF-8 byte range from one project file.",
	), service.read)
	return server
}

func utf8Boundary(source []byte, offset int) bool {
	return offset == 0 || offset == len(source) || utf8.RuneStart(source[offset])
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use: "mcp", Short: "Serve typed read-only source tools over stdio MCP", Long: helpMCP,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			files := srcview.NewProjectService(project.NewStore(config.ProjectsPath()))
			return newSrcMCPServer(files).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
