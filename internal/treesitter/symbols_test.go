package treesitter

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSymbols_Go(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.go")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)

	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	assert.Contains(t, names, "Config")
	assert.Contains(t, names, "Host")
	assert.Contains(t, names, "Port")
	assert.Contains(t, names, "Validate")
	assert.Contains(t, names, "main")
}

func TestExtractSymbols_GoDocComments(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.go")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)

	for _, s := range symbols {
		if s.Name == "Config" {
			assert.True(t, s.DocStart >= 0, "Config should have doc comment")
			return
		}
	}
	t.Fatal("Config symbol not found")
}

func TestExtractSymbols_DepthFiltering(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.go")
	require.NoError(t, err)

	// Depth 1: should NOT include struct fields
	symbols1, err := ExtractSymbols("example.go", source, 1)
	require.NoError(t, err)
	for _, s := range symbols1 {
		assert.Equal(t, 1, s.Level, "depth=1 should only return level-1 symbols")
	}

	// Depth 2: should include struct fields
	symbols2, err := ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)
	hasLevel2 := false
	for _, s := range symbols2 {
		if s.Level == 2 {
			hasLevel2 = true
			break
		}
	}
	assert.True(t, hasLevel2, "depth=2 should include level-2 symbols")
}

func TestExtractSymbols_Heuristic(t *testing.T) {
	// The heuristic walker is exercised for any language without a custom .scm file.
	// We test with an unsupported extension that gracefully returns an error.
	_, err := ExtractSymbols("example.unknown_lang_xyz", []byte("hello world"), 2)
	assert.Error(t, err, "unsupported file type should return error")
}

func TestSymbolTree_IDsAssigned(t *testing.T) {
	symbols := []Symbol{
		{Name: "main", Kind: "function", Level: 1},
		{Name: "Config", Kind: "type", Level: 1},
	}
	nodes := SymbolTree(symbols)
	assert.Len(t, nodes, 2)
	assert.NotEmpty(t, nodes[0].ID)
	assert.NotEmpty(t, nodes[1].ID)
	assert.NotEqual(t, nodes[0].ID, nodes[1].ID)
}

func TestExtractSymbols_GoMethodReceiver(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.go")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)

	for _, s := range symbols {
		if s.Name == "Validate" {
			assert.Equal(t, "method", s.Kind)
			assert.Equal(t, "Config", s.Parent)
			return
		}
	}
	t.Fatal("Validate method not found")
}

func TestExtractSymbols_MarkdownGrammarError(t *testing.T) {
	// Documents that gotreesitter (pure Go, no cgo) cannot handle the markdown
	// grammar's external scanner. ExtractSymbols must return an error, not panic.
	// If this test fails (no error returned), it signals that gotreesitter has gained
	// markdown support and the bypass in cmd/src may be revisitable.
	source := []byte("# Hello\n\n## World\n\nText.\n")
	_, err := ExtractSymbols("test.md", source, 2)
	assert.Error(t, err, "expected error for markdown grammar; gotreesitter cannot handle its external scanner")
}
