package treesitter

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symbolNames returns a slice of symbol names for easy assertion.
func symbolNames(symbols []Symbol) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	return names
}

// findSymbol returns the first symbol with the given name, or nil.
func findSymbol(symbols []Symbol, name string) *Symbol {
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}
	return nil
}

func TestExtractSymbols_Go(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.go")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)

	names := symbolNames(symbols)
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

func TestExtractSymbols_MarkdownDFAParsing(t *testing.T) {
	// Markdown previously panicked due to a missing external scanner.
	// With the DFA fallback in ParseFile, it now parses partially without error.
	// cmd/src bypasses ExtractSymbols for .md files entirely (uses markdown package).
	// Symbol extraction on a DFA-parsed markdown tree may return empty results.
	source := []byte("# Hello\n\n## World\n\nText.\n")
	_, err := ExtractSymbols("test.md", source, 2)
	assert.NoError(t, err, "markdown should parse via DFA fallback without error")
}

func TestExtractSymbols_Cpp(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.cpp")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.cpp", source, 2)
	require.NoError(t, err)

	names := symbolNames(symbols)
	assert.Contains(t, names, "Shape")
	assert.Contains(t, names, "Circle")
	assert.Contains(t, names, "computeArea")
	assert.Contains(t, names, "Point")
	assert.Contains(t, names, "Color")
	assert.Contains(t, names, "uint")

	// Kind assertions — proves query-based extraction, not heuristic
	shape := findSymbol(symbols, "Shape")
	require.NotNil(t, shape)
	assert.Equal(t, "class", shape.Kind)

	computeArea := findSymbol(symbols, "computeArea")
	require.NotNil(t, computeArea)
	assert.Equal(t, "function", computeArea.Kind)
}

func TestExtractSymbols_Tsx(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.tsx")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("example.tsx", source, 2)
	require.NoError(t, err)

	names := symbolNames(symbols)
	assert.Contains(t, names, "ButtonProps")
	assert.Contains(t, names, "Button")
	assert.Contains(t, names, "Counter")
	assert.Contains(t, names, "Theme")

	// Kind assertions — proves typescript.scm query fired (not heuristic)
	button := findSymbol(symbols, "Button")
	require.NotNil(t, button)
	assert.Equal(t, "function", button.Kind, "TSX should use typescript.scm query, not heuristic")

	buttonProps := findSymbol(symbols, "ButtonProps")
	require.NotNil(t, buttonProps)
	assert.Equal(t, "interface", buttonProps.Kind)

	counter := findSymbol(symbols, "Counter")
	require.NotNil(t, counter)
	assert.Equal(t, "class", counter.Kind)
}

func TestExtractSymbols_Java(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/Example.java")
	require.NoError(t, err)

	symbols, err := ExtractSymbols("Example.java", source, 2)
	require.NoError(t, err)

	names := symbolNames(symbols)
	assert.Contains(t, names, "Animal")
	assert.Contains(t, names, "Dog")
	assert.Contains(t, names, "Direction")

	// Kind assertions
	animal := findSymbol(symbols, "Animal")
	require.NotNil(t, animal)
	assert.Equal(t, "interface", animal.Kind)

	dog := findSymbol(symbols, "Dog")
	require.NotNil(t, dog)
	assert.Equal(t, "class", dog.Kind)
}

func TestExtractSymbols_FallbackToResolveTagsQuery(t *testing.T) {
	// Scala has a gotreesitter grammar but no vendored .scm file.
	// If Scala isn't available, this test documents the fallback path.
	// The heuristic walker should still return symbols for any parseable file.
	source := []byte(`object Main {
  def greet(name: String): Unit = {
    println(s"Hello, $name")
  }
}`)
	symbols, err := ExtractSymbols("Main.scala", source, 1)
	if err != nil {
		t.Skipf("Scala grammar not available: %v", err)
	}
	// If we get here, the fallback chain worked (tier 2 or tier 3)
	assert.NotEmpty(t, symbols, "fallback should extract at least some symbols")
}
