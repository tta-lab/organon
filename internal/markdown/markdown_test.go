package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/fetch"
)

const testDoc = `# My Document

Introduction text.

## Installation

Install with go get.

### Requirements

Go 1.21 or higher.

## Usage

Run the binary.

## Configuration

Set env vars.
`

func TestParseHeadings_Levels(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assert.Len(t, headings, 5)
	assert.Equal(t, 1, headings[0].level)
	assert.Equal(t, "My Document", headings[0].text)
	assert.Equal(t, 2, headings[1].level)
	assert.Equal(t, "Installation", headings[1].text)
	assert.Equal(t, 3, headings[2].level)
	assert.Equal(t, "Requirements", headings[2].text)
}

func TestAssignIDs_H1HasNoID(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assignIDs(headings)
	assert.Empty(t, headings[0].id, "H1 should have no ID")
	assert.NotEmpty(t, headings[1].id, "H2 should have ID")
}

func TestAssignIDs_Unique(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assignIDs(headings)
	seen := map[string]bool{}
	for _, h := range headings {
		if h.id == "" {
			continue
		}
		assert.False(t, seen[h.id], "duplicate ID: %s", h.id)
		seen[h.id] = true
	}
}

func TestExtractSection_ValidID(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assignIDs(headings)

	// Find the Installation section ID
	var installID string
	for _, h := range headings {
		if h.text == "Installation" {
			installID = h.id
		}
	}
	require.NotEmpty(t, installID)

	section, err := extractSection([]byte(testDoc), headings, installID)
	require.NoError(t, err)
	assert.Contains(t, section, "## Installation")
	assert.Contains(t, section, "Install with go get")
	assert.Contains(t, section, "### Requirements")
	assert.NotContains(t, section, "## Usage") // should not include next same-level section
}

func TestExtractSection_InvalidID(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assignIDs(headings)
	_, err = extractSection([]byte(testDoc), headings, "zz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenderContent_Full(t *testing.T) {
	result, err := RenderContent([]byte(testDoc), false, "", true, 0)
	require.NoError(t, err)
	assert.Equal(t, "full", result.Mode)
	assert.Contains(t, result.Content, "My Document")
	assert.Contains(t, result.Content, "Installation")
}

func TestRenderContent_Tree(t *testing.T) {
	result, err := RenderContent([]byte(testDoc), true, "", false, 0)
	require.NoError(t, err)
	assert.Equal(t, "tree", result.Mode)
	assert.Contains(t, result.Content, "## Installation")
	assert.Contains(t, result.Content, "## Usage")
	// Should include hint
	assert.Contains(t, result.Content, "-s")
}

func TestRenderContent_AutoTree(t *testing.T) {
	// Build a doc larger than threshold
	bigDoc := testDoc + strings.Repeat("x", 6000)
	result, err := RenderContent([]byte(bigDoc), false, "", false, 5000)
	require.NoError(t, err)
	assert.Equal(t, "tree", result.Mode)
}

func TestRenderContent_Section(t *testing.T) {
	headings, err := parseHeadings([]byte(testDoc))
	require.NoError(t, err)
	assignIDs(headings)
	var usageID string
	for _, h := range headings {
		if h.text == "Usage" {
			usageID = h.id
		}
	}
	require.NotEmpty(t, usageID)

	result, err := RenderContent([]byte(testDoc), false, usageID, false, 0)
	require.NoError(t, err)
	assert.Equal(t, "section", result.Mode)
	assert.Contains(t, result.Content, "## Usage")
	assert.NotContains(t, result.Content, "## Configuration")
}

func TestFormatNum(t *testing.T) {
	assert.Equal(t, "42", formatNum(42))
	assert.Equal(t, "1,000", formatNum(1000))
	assert.Equal(t, "12,345", formatNum(12345))
	assert.Equal(t, "1,234,567", formatNum(1234567))
}

func TestTruncateContent_Short(t *testing.T) {
	s := "hello world"
	assert.Equal(t, s, fetch.TruncateContent(s))
}

func TestTruncateContent_Long(t *testing.T) {
	s := strings.Repeat("a", 31000)
	result := fetch.TruncateContent(s)
	assert.True(t, len(result) < len(s))
	assert.Contains(t, result, "truncated")
}
