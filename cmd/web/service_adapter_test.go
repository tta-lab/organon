package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/search"
	webcore "github.com/tta-lab/organon/internal/web"
)

type stubWebService struct {
	searchResult webcore.SearchResult
	searchInput  string
	fetchResult  webcore.FetchResult
	resolve      webcore.DocsResolveResult
	docs         webcore.DocsFetchResult
	sgraph       webcore.SGraphResult
	fetchInput   webcore.FetchInput
	docsInput    webcore.DocsFetchInput
	sgraphInput  webcore.SGraphInput
}

func (s *stubWebService) Search(_ context.Context, query string) (webcore.SearchResult, error) {
	s.searchInput = query
	return s.searchResult, nil
}

func (s *stubWebService) Fetch(_ context.Context, input webcore.FetchInput) (webcore.FetchResult, error) {
	s.fetchInput = input
	return s.fetchResult, nil
}

func (s *stubWebService) DocsResolve(context.Context, string) (webcore.DocsResolveResult, error) {
	return s.resolve, nil
}

func (s *stubWebService) DocsFetch(_ context.Context, input webcore.DocsFetchInput) (webcore.DocsFetchResult, error) {
	s.docsInput = input
	return s.docs, nil
}

func (s *stubWebService) SGraphSearch(_ context.Context, input webcore.SGraphInput) (webcore.SGraphResult, error) {
	s.sgraphInput = input
	return s.sgraph, nil
}

func fixedServiceFactory(service webService) serviceFactory {
	return func(string) (webService, error) { return service, nil }
}

func capturingServiceFactory(service webService, captured *string) serviceFactory {
	return func(provider string) (webService, error) {
		*captured = provider
		return service, nil
	}
}

func TestSearchCLIFormatsTypedServiceResult(t *testing.T) {
	service := &stubWebService{searchResult: webcore.SearchResult{
		Provider: "Brave",
		Results:  []search.SearchResult{{Title: "One", Link: "https://example.com", Snippet: "Summary", Position: 1}},
	}}
	var provider string
	cmd := newSearchCmdWithFactory(capturingServiceFactory(service, &provider))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"query", "--provider", "brave"})

	require.NoError(t, cmd.Execute())
	want := "Found 1 search results:\n\n" +
		"1. One\n   URL: https://example.com\n   Summary: Summary\n\n"
	assert.Equal(t, want, output.String())
	assert.Equal(t, "brave", provider)
}

func TestFetchCLIMapsFlagsAndPrintsContent(t *testing.T) {
	service := &stubWebService{fetchResult: webcore.FetchResult{Content: "rendered"}}
	cmd := newFetchCmdWithFactory(fixedServiceFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"https://example.com", "--tree", "--section-id", "ab", "--tree-threshold", "42"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "rendered", output.String())
	assert.Equal(t, webcore.FetchInput{
		URL: "https://example.com", ShowTree: true, SectionID: "ab", TreeThreshold: 42,
	}, service.fetchInput)
}

func TestDocsCLIUsesTypedServiceResults(t *testing.T) {
	service := &stubWebService{
		resolve: webcore.DocsResolveResult{Libraries: []docs.Library{{ID: "/org/lib", Title: "Library"}}},
		docs:    webcore.DocsFetchResult{Content: "documentation"},
	}
	resolveCmd := newDocsResolveCmdWithFactory(fixedServiceFactory(service))
	var resolveOutput bytes.Buffer
	resolveCmd.SetOut(&resolveOutput)
	resolveCmd.SetArgs([]string{"library"})
	require.NoError(t, resolveCmd.Execute())
	assert.Contains(t, resolveOutput.String(), "Library")

	fetchCmd := newDocsFetchCmdWithFactory(fixedServiceFactory(service))
	var fetchOutput bytes.Buffer
	fetchCmd.SetOut(&fetchOutput)
	fetchCmd.SetArgs([]string{"org/lib", "topic", "--tokens", "500"})
	require.NoError(t, fetchCmd.Execute())
	assert.Equal(t, "documentation", fetchOutput.String())
	assert.Equal(t, webcore.DocsFetchInput{LibraryID: "org/lib", Topic: "topic", Tokens: 500}, service.docsInput)
}

func runWebJSON(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return output.String()
}

func TestSearchCLIJSONOutputMatchesStructuredResult(t *testing.T) {
	service := &stubWebService{searchResult: webcore.SearchResult{
		Provider: "Brave",
		Results:  []search.SearchResult{{Title: "One", Link: "https://example.com", Snippet: "Summary", Position: 1}},
	}}
	out := runWebJSON(t, newSearchCmdWithFactory(fixedServiceFactory(service)), "query", "--json")
	var got webcore.SearchResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "Brave", got.Provider)
	assert.Equal(t, "One", got.Results[0].Title)
	assert.Equal(t, "https://example.com", got.Results[0].Link)
}

func TestFetchCLIJSONOutputMatchesStructuredResult(t *testing.T) {
	service := &stubWebService{
		fetchResult: webcore.FetchResult{URL: "https://example.com", Mode: "full", Content: "rendered"},
	}
	out := runWebJSON(t, newFetchCmdWithFactory(fixedServiceFactory(service)), "https://example.com", "--json")
	var got webcore.FetchResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, webcore.FetchResult{URL: "https://example.com", Mode: "full", Content: "rendered"}, got)
}

func TestDocsAndSGraphCLISeparateHyphenArguments(t *testing.T) {
	service := &stubWebService{}

	fetchCmd := newDocsFetchCmdWithFactory(fixedServiceFactory(service))
	fetchCmd.SetArgs([]string{"--json", "--tokens", "500", "--", "-org/lib", "-topic"})
	require.NoError(t, fetchCmd.Execute())
	assert.Equal(t, webcore.DocsFetchInput{LibraryID: "-org/lib", Topic: "-topic", Tokens: 500}, service.docsInput)

	sgraphCmd := newSgraphCmdWithFactory(fixedServiceFactory(service))
	sgraphCmd.SetArgs([]string{"--json", "--count", "14", "--", "-repo:test"})
	require.NoError(t, sgraphCmd.Execute())
	assert.Equal(t, webcore.SGraphInput{Query: "-repo:test", Count: 14, ContextWindow: 10}, service.sgraphInput)
}

func TestDocsResolveCLIJSONOutputMatchesStructuredResult(t *testing.T) {
	service := &stubWebService{resolve: webcore.DocsResolveResult{
		Query:     "library",
		Libraries: []docs.Library{{ID: "/org/lib", Title: "Library", TrustScore: 1.0}},
	}}
	out := runWebJSON(t, newDocsResolveCmdWithFactory(fixedServiceFactory(service)), "library", "--json")
	var got webcore.DocsResolveResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, service.resolve, got)
}

func TestDocsFetchCLIJSONOutputMatchesStructuredResult(t *testing.T) {
	service := &stubWebService{
		docs: webcore.DocsFetchResult{LibraryID: "/org/lib", Topic: "topic", Content: "documentation"},
	}
	out := runWebJSON(
		t,
		newDocsFetchCmdWithFactory(fixedServiceFactory(service)),
		"org/lib", "topic", "--tokens", "500", "--json",
	)
	var got webcore.DocsFetchResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "/org/lib", got.LibraryID)
	assert.Equal(t, "topic", got.Topic)
	assert.Equal(t, "documentation", got.Content)
}

func TestSGraphCLIJSONOutputMatchesStructuredResult(t *testing.T) {
	service := &stubWebService{sgraph: webcore.SGraphResult{Content: "matches"}}
	out := runWebJSON(t, newSgraphCmdWithFactory(fixedServiceFactory(service)), "repo:test", "--json")
	var got webcore.SGraphResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "matches", got.Content)
}

func TestSGraphCLIMapsFlagsAndPrintsContent(t *testing.T) {
	service := &stubWebService{sgraph: webcore.SGraphResult{Content: "matches"}}
	cmd := newSgraphCmdWithFactory(fixedServiceFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"repo:test", "--count", "14", "--context", "3", "--timeout", "9"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "matches", output.String())
	assert.Equal(t, webcore.SGraphInput{Query: "repo:test", Count: 14, ContextWindow: 3, Timeout: 9}, service.sgraphInput)
}
