package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const editDoc = `# My Document

Introduction text.

## Getting Started

Setup instructions.

### Prerequisites

- Go 1.21+
- Make

## API Reference

Details here.

## Contributing

PR guidelines.
`

// sectionIDFor resolves a section ID by heading text for use in tests.
func sectionIDFor(t *testing.T, source []byte, headingText string) string {
	t.Helper()
	headings, err := parseHeadings(source)
	require.NoError(t, err)
	assignIDs(headings)
	for _, h := range headings {
		if h.text == headingText {
			require.NotEmpty(t, h.id, "heading %q has no ID (H1 headings cannot be targeted)", headingText)
			return h.id
		}
	}
	t.Fatalf("heading %q not found in document", headingText)
	return ""
}

func TestHeadingTree(t *testing.T) {
	tree, err := HeadingTree([]byte(editDoc))
	require.NoError(t, err)
	assert.Contains(t, tree, "## Getting Started")
	assert.Contains(t, tree, "## API Reference")
	assert.Contains(t, tree, "## Contributing")
	assert.Contains(t, tree, "-s")
	// IDs should be present
	assert.Contains(t, tree, "[")
}

func TestReadSection(t *testing.T) {
	id := sectionIDFor(t, []byte(editDoc), "Getting Started")
	content, err := ReadSection([]byte(editDoc), id)
	require.NoError(t, err)
	assert.Contains(t, content, "## Getting Started")
	assert.Contains(t, content, "Setup instructions")
	assert.Contains(t, content, "### Prerequisites")
	assert.NotContains(t, content, "## API Reference")
}

func TestReplaceSection(t *testing.T) {
	id := sectionIDFor(t, []byte(editDoc), "Getting Started")
	newContent := []byte("## Getting Started\n\nNew setup instructions.\n")
	result, err := ReplaceSection([]byte(editDoc), id, newContent)
	require.NoError(t, err)
	assert.Contains(t, string(result), "New setup instructions")
	assert.NotContains(t, string(result), "Setup instructions.")
	assert.NotContains(t, string(result), "### Prerequisites")
	assert.Contains(t, string(result), "## API Reference")
}

func TestInsertBeforeSection(t *testing.T) {
	id := sectionIDFor(t, []byte(editDoc), "API Reference")
	newContent := []byte("## New Section\n\nNew section content.\n")
	result, err := InsertBeforeSection([]byte(editDoc), id, newContent)
	require.NoError(t, err)
	s := string(result)
	assert.Contains(t, s, "## New Section")
	// New section should appear before API Reference
	newIdx := indexOf(s, "## New Section")
	apiIdx := indexOf(s, "## API Reference")
	assert.Less(t, newIdx, apiIdx)
}

func TestInsertAfterSection(t *testing.T) {
	id := sectionIDFor(t, []byte(editDoc), "Contributing")
	newContent := []byte("## Appendix\n\nExtra info.\n")
	result, err := InsertAfterSection([]byte(editDoc), id, newContent)
	require.NoError(t, err)
	s := string(result)
	assert.Contains(t, s, "## Appendix")
	assert.Contains(t, s, "Extra info")
	contribIdx := indexOf(s, "## Contributing")
	appendixIdx := indexOf(s, "## Appendix")
	assert.Less(t, contribIdx, appendixIdx)
}

func TestDeleteSection(t *testing.T) {
	id := sectionIDFor(t, []byte(editDoc), "Contributing")
	result, err := DeleteSection([]byte(editDoc), id)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "## Contributing")
	assert.NotContains(t, string(result), "PR guidelines")
	assert.Contains(t, string(result), "## API Reference")
}

const emptyBodyDoc = `# Doc

## Section A

## Section B

Content here.
`

func TestReadSection_EmptyBody(t *testing.T) {
	id := sectionIDFor(t, []byte(emptyBodyDoc), "Section A")
	content, err := ReadSection([]byte(emptyBodyDoc), id)
	require.NoError(t, err)
	assert.Contains(t, content, "## Section A")
	assert.NotContains(t, content, "## Section B")
}

func TestReplaceSection_EmptyBody(t *testing.T) {
	id := sectionIDFor(t, []byte(emptyBodyDoc), "Section A")
	newContent := []byte("## Section A\n\nNow has content.\n")
	result, err := ReplaceSection([]byte(emptyBodyDoc), id, newContent)
	require.NoError(t, err)
	assert.Contains(t, string(result), "Now has content")
	assert.Contains(t, string(result), "## Section B")
}

func TestDeleteSection_EmptyBody(t *testing.T) {
	id := sectionIDFor(t, []byte(emptyBodyDoc), "Section A")
	result, err := DeleteSection([]byte(emptyBodyDoc), id)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "## Section A")
	assert.Contains(t, string(result), "## Section B")
}

// indexOf returns the byte index of substr in s, or -1.
func indexOf(s, substr string) int {
	for i := range s {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
