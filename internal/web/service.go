package web

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/fetch"
	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/search"
	"github.com/tta-lab/organon/internal/sgraph"
)

// DocsClient is the Context7 behavior used by Service.
type DocsClient interface {
	Resolve(context.Context, string) ([]docs.Library, error)
	Docs(context.Context, string, string, int) (string, error)
}

// Dependencies are injectable external operations used by Service.
type Dependencies struct {
	Search        func(context.Context, string, search.Config) (search.Response, error)
	ResolveFetch  func() fetch.Backend
	NewDocsClient func() (DocsClient, error)
	SGraphSearch  func(context.Context, string, int, int, int) (string, error)
}

// Options configure a Service. SearchProvider applies to every search in the service.
type Options struct {
	SearchProvider string
	Dependencies   Dependencies
}

// Service coordinates web backends for CLI and MCP adapters.
type Service struct {
	searchProvider string
	deps           Dependencies
}

// NewService constructs a shared web service.
func NewService(options Options) (*Service, error) {
	if err := search.ValidateProvider(options.SearchProvider); err != nil {
		return nil, err
	}

	deps := options.Dependencies
	if deps.Search == nil {
		deps.Search = search.SearchResultsWithConfig
	}
	if deps.ResolveFetch == nil {
		deps.ResolveFetch = fetch.Resolve
	}
	if deps.NewDocsClient == nil {
		deps.NewDocsClient = NewDocsClientFromEnvironment
	}
	if deps.SGraphSearch == nil {
		deps.SGraphSearch = sgraph.Search
	}

	return &Service{searchProvider: options.SearchProvider, deps: deps}, nil
}

// Search performs a configured web search.
func (s *Service) Search(ctx context.Context, query string) (SearchResult, error) {
	return s.deps.Search(ctx, query, search.Config{Provider: s.searchProvider})
}

// Fetch retrieves a page, rejects binary bodies, and renders Markdown.
func (s *Service) Fetch(ctx context.Context, input FetchInput) (FetchResult, error) {
	content, err := s.deps.ResolveFetch().Fetch(ctx, input.URL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: %w", input.URL, err)
	}
	if fetch.IsBinaryBody([]byte(content)) {
		return FetchResult{}, fetch.BinaryFetchError(input.URL, "")
	}

	rendered, err := markdown.RenderContent(
		[]byte(content),
		input.ShowTree,
		input.SectionID,
		input.Full,
		input.TreeThreshold,
	)
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{URL: input.URL, Mode: rendered.Mode, Content: rendered.Content}, nil
}

// DocsResolve resolves a Context7 library name.
func (s *Service) DocsResolve(ctx context.Context, query string) (DocsResolveResult, error) {
	client, err := s.deps.NewDocsClient()
	if err != nil {
		return DocsResolveResult{}, err
	}
	libraries, err := client.Resolve(ctx, query)
	if err != nil {
		return DocsResolveResult{}, err
	}
	if len(libraries) == 0 {
		return DocsResolveResult{}, fmt.Errorf("no libraries found for %q", query)
	}
	return DocsResolveResult{Query: query, Libraries: libraries}, nil
}

// DocsFetch fetches Context7 documentation for a normalized library ID.
func (s *Service) DocsFetch(ctx context.Context, input DocsFetchInput) (DocsFetchResult, error) {
	client, err := s.deps.NewDocsClient()
	if err != nil {
		return DocsFetchResult{}, err
	}
	id := NormalizeLibraryID(input.LibraryID)
	content, err := client.Docs(ctx, id, input.Topic, input.Tokens)
	if err != nil {
		return DocsFetchResult{}, err
	}
	return DocsFetchResult{LibraryID: id, Topic: input.Topic, Content: content}, nil
}

// SGraphSearch searches Sourcegraph with the CLI's established defaults.
func (s *Service) SGraphSearch(ctx context.Context, input SGraphInput) (SGraphResult, error) {
	count := input.Count
	if count <= 0 {
		count = 10
	} else if count > 20 {
		count = 20
	}
	contextWindow := input.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 10
	}
	timeout := input.Timeout
	if timeout > 120 {
		timeout = 120
	}
	content, err := s.deps.SGraphSearch(ctx, input.Query, count, contextWindow, timeout)
	if err != nil {
		return SGraphResult{}, err
	}
	return SGraphResult{Content: content}, nil
}

// NormalizeLibraryID adds the leading slash expected by Context7.
func NormalizeLibraryID(id string) string {
	if id == "" || strings.HasPrefix(id, "/") {
		return id
	}
	return "/" + id
}

// NewDocsClientFromEnvironment validates CONTEXT7_API_KEY and constructs a client.
func NewDocsClientFromEnvironment() (DocsClient, error) {
	key, set := os.LookupEnv("CONTEXT7_API_KEY")
	if set && strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("CONTEXT7_API_KEY is set but empty; provide a key or unset it")
	}
	return docs.NewClient(key), nil
}
