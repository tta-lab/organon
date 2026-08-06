package web

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/fetch"
	"github.com/tta-lab/organon/internal/search"
)

type stubFetchBackend struct {
	content string
	err     error
}

func (s stubFetchBackend) Fetch(_ context.Context, _ string) (string, error) {
	return s.content, s.err
}

type stubDocsClient struct {
	libraries []docs.Library
	content   string
	resolveQ  string
	docsID    string
	topic     string
	tokens    int
}

func (s *stubDocsClient) Resolve(_ context.Context, query string) ([]docs.Library, error) {
	s.resolveQ = query
	return s.libraries, nil
}

func (s *stubDocsClient) Docs(_ context.Context, id, topic string, tokens int) (string, error) {
	s.docsID, s.topic, s.tokens = id, topic, tokens
	return s.content, nil
}

func TestServiceSearchReturnsProviderAndTypedResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.toml")
	require.NoError(t, os.WriteFile(path, []byte("[search]\nprovider = \"brave\"\n"), 0o600))

	var gotProvider string
	svc, err := NewService(Options{
		SearchConfigPath: path,
		Dependencies: Dependencies{
			Search: func(_ context.Context, query string, cfg search.Config) (search.Response, error) {
				assert.Equal(t, "typed mcp", query)
				gotProvider = cfg.Search.Provider
				return search.Response{
					Provider: "Brave",
					Results:  []search.SearchResult{{Title: "Result", Link: "https://example.com", Position: 1}},
				}, nil
			},
		},
	})
	require.NoError(t, err)

	got, err := svc.Search(context.Background(), "typed mcp")
	require.NoError(t, err)
	assert.Equal(t, "brave", gotProvider)
	assert.Equal(t, "Brave", got.Provider)
	require.Len(t, got.Results, 1)
	assert.Equal(t, "Result", got.Results[0].Title)
}

func TestNewServiceRejectsInvalidSearchConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.toml")
	require.NoError(t, os.WriteFile(path, []byte("[search\nprovider = \"brave\"\n"), 0o600))

	_, err := NewService(Options{SearchConfigPath: path})
	require.Error(t, err)
	assert.ErrorContains(t, err, "read web config")
}

func TestServiceFetchReturnsRenderedMode(t *testing.T) {
	svc, err := NewService(Options{Dependencies: Dependencies{
		ResolveFetch: func() fetch.Backend {
			return stubFetchBackend{content: "# Page\n\n## Section\n\nBody\n"}
		},
	}})
	require.NoError(t, err)

	got, err := svc.Fetch(context.Background(), FetchInput{
		URL:           "https://example.com/page",
		ShowTree:      true,
		TreeThreshold: 5000,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/page", got.URL)
	assert.Equal(t, "tree", got.Mode)
	assert.Contains(t, got.Content, "Section")
}

func TestServiceFetchRejectsBinaryFromAnyBackend(t *testing.T) {
	svc, err := NewService(Options{Dependencies: Dependencies{
		ResolveFetch: func() fetch.Backend {
			return stubFetchBackend{content: "%PDF-1.7\x00"}
		},
	}})
	require.NoError(t, err)

	_, err = svc.Fetch(context.Background(), FetchInput{URL: "https://example.com/file.pdf"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "binary")
}

func TestServiceDocsResolveAndFetch(t *testing.T) {
	client := &stubDocsClient{
		libraries: []docs.Library{{ID: "/effect-ts/effect", Title: "Effect"}},
		content:   "Effect docs",
	}
	svc, err := NewService(Options{Dependencies: Dependencies{
		NewDocsClient: func() (DocsClient, error) { return client, nil },
	}})
	require.NoError(t, err)

	resolved, err := svc.DocsResolve(context.Background(), "effect")
	require.NoError(t, err)
	assert.Equal(t, "effect", client.resolveQ)
	assert.Equal(t, client.libraries, resolved.Libraries)

	fetched, err := svc.DocsFetch(context.Background(), DocsFetchInput{
		LibraryID: "effect-ts/effect",
		Topic:     "schema",
		Tokens:    1200,
	})
	require.NoError(t, err)
	assert.Equal(t, "/effect-ts/effect", client.docsID)
	assert.Equal(t, "/effect-ts/effect", fetched.LibraryID)
	assert.Equal(t, "schema", fetched.Topic)
	assert.Equal(t, "Effect docs", fetched.Content)
}

func TestServiceDocsResolveRejectsNoMatches(t *testing.T) {
	svc, err := NewService(Options{Dependencies: Dependencies{
		NewDocsClient: func() (DocsClient, error) { return &stubDocsClient{}, nil },
	}})
	require.NoError(t, err)

	_, err = svc.DocsResolve(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorContains(t, err, `no libraries found for "missing"`)
}

func TestServiceSGraphAppliesExistingDefaults(t *testing.T) {
	var query string
	var count, contextWindow, timeout int
	svc, err := NewService(Options{Dependencies: Dependencies{
		SGraphSearch: func(_ context.Context, q string, c, w, to int) (string, error) {
			query, count, contextWindow, timeout = q, c, w, to
			return "# results", nil
		},
	}})
	require.NoError(t, err)

	got, err := svc.SGraphSearch(context.Background(), SGraphInput{Query: "repo:tta-lab", Count: -1})
	require.NoError(t, err)
	assert.Equal(t, "repo:tta-lab", query)
	assert.Equal(t, 10, count)
	assert.Equal(t, 10, contextWindow)
	assert.Zero(t, timeout)
	assert.Equal(t, "# results", got.Content)
}

func TestServicePropagatesDependencyError(t *testing.T) {
	want := errors.New("offline")
	svc, err := NewService(Options{Dependencies: Dependencies{
		Search: func(context.Context, string, search.Config) (search.Response, error) {
			return search.Response{}, want
		},
	}})
	require.NoError(t, err)

	_, err = svc.Search(context.Background(), "query")
	assert.ErrorIs(t, err, want)
}
