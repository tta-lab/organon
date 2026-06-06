package textdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodesJSONTopLevelObjectKeys(t *testing.T) {
	source := []byte("{\n  \"name\": \"organon\",\n  \"version\": 2\n}\n")

	nodes, err := Nodes("package.json", source)
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	assert.Equal(t, "key name", nodes[0].Label)
	assert.Equal(t, 1, nodes[0].Level)
	assert.Equal(t, "[L2]", nodes[0].Meta)
	assert.Equal(t, "key version", nodes[1].Label)

	start, end, err := Bounds("package.json", source, nodes[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "  \"name\": \"organon\",\n", string(source[start:end]))
}

func TestNodesCSVHeaderAndRows(t *testing.T) {
	source := []byte("name,score\nada,10\nlinus,9\n")

	nodes, err := Nodes("scores.csv", source)
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	assert.Equal(t, "header: name,score", nodes[0].Label)
	assert.Equal(t, "row 2: ada,10", nodes[1].Label)
	assert.Equal(t, "[L2]", nodes[1].Meta)

	start, end, err := Bounds("scores.csv", source, nodes[1].ID)
	require.NoError(t, err)
	assert.Equal(t, "ada,10\n", string(source[start:end]))
}

func TestNodesTeXSectionsEnvironmentsAndParagraphs(t *testing.T) {
	source := []byte("\\section{Intro}\nText here.\n\n\\begin{equation}\na=b\n\\end{equation}\n")

	nodes, err := Nodes("paper.tex", source)
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	assert.Equal(t, `\section{Intro}`, nodes[0].Label)
	assert.Equal(t, 1, nodes[0].Level)
	assert.Equal(t, `environment equation`, nodes[1].Label)
	assert.Equal(t, 2, nodes[1].Level)

	start, end, err := Bounds("paper.tex", source, nodes[1].ID)
	require.NoError(t, err)
	assert.Equal(t, "\\begin{equation}\na=b\n\\end{equation}\n", string(source[start:end]))
}

func TestNodesPlainTextParagraphs(t *testing.T) {
	source := []byte("first paragraph\ncontinues\n\nsecond paragraph\n")

	nodes, err := Nodes("notes.txt", source)
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	assert.Equal(t, "paragraph 1: first paragraph", nodes[0].Label)
	assert.Equal(t, "[L1-L2]", nodes[0].Meta)
	assert.Equal(t, "paragraph 2: second paragraph", nodes[1].Label)

	start, end, err := Bounds("notes.txt", source, nodes[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "first paragraph\ncontinues\n", string(source[start:end]))
}

func TestInsertAfterLastLineWithoutTrailingNewline(t *testing.T) {
	source := []byte("name,score\nada,10")
	nodes, err := Nodes("scores.csv", source)
	require.NoError(t, err)

	result, err := InsertAfter("scores.csv", source, nodes[1].ID, []byte("linus,9\n"))
	require.NoError(t, err)
	assert.Equal(t, "name,score\nada,10\nlinus,9\n", string(result))
}

func TestUnsupportedFile(t *testing.T) {
	_, err := Nodes("example.go", []byte("package main\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported text document type")
}
