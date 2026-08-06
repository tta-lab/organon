package web

import (
	"github.com/tta-lab/organon/internal/docs"
	"github.com/tta-lab/organon/internal/search"
)

// SearchResult identifies the selected provider and ranked search results.
type SearchResult = search.Response

// FetchInput controls page retrieval and markdown rendering.
type FetchInput struct {
	URL           string `json:"url"`
	ShowTree      bool   `json:"tree,omitempty"`
	SectionID     string `json:"section_id,omitempty"`
	Full          bool   `json:"full,omitempty"`
	TreeThreshold int    `json:"tree_threshold,omitempty"`
}

// FetchResult is rendered content from one URL.
type FetchResult struct {
	URL     string `json:"url"`
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

// DocsResolveResult contains Context7 libraries matching a query.
type DocsResolveResult struct {
	Query     string         `json:"query"`
	Libraries []docs.Library `json:"libraries"`
}

// DocsFetchInput selects Context7 documentation.
type DocsFetchInput struct {
	LibraryID string `json:"library_id"`
	Topic     string `json:"topic,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
}

// DocsFetchResult contains fetched Context7 documentation.
type DocsFetchResult struct {
	LibraryID string `json:"library_id"`
	Topic     string `json:"topic,omitempty"`
	Content   string `json:"content"`
}

// SGraphInput controls a Sourcegraph public code search.
type SGraphInput struct {
	Query         string `json:"query"`
	Count         int    `json:"count,omitempty"`
	ContextWindow int    `json:"context,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
}

// SGraphResult contains Sourcegraph's stable Markdown rendering.
type SGraphResult struct {
	Content string `json:"content"`
}
