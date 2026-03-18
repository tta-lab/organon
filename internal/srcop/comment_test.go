package srcop

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/treesitter"
)

func TestReadComment_ExistingComment(t *testing.T) {
	source, err := os.ReadFile(testFixture)
	require.NoError(t, err)

	id := symbolIDFor(t, source, "Config")
	comment, err := ReadComment("example.go", source, id, 2)
	require.NoError(t, err)
	assert.Contains(t, comment, "Config holds server configuration")
}

func TestReadComment_NoComment(t *testing.T) {
	source, err := os.ReadFile(testFixture)
	require.NoError(t, err)

	id := symbolIDFor(t, source, "main")
	comment, err := ReadComment("example.go", source, id, 2)
	require.NoError(t, err)
	assert.Empty(t, comment, "main has no doc comment")
}

func TestReadComment_NotFound(t *testing.T) {
	source, err := os.ReadFile(testFixture)
	require.NoError(t, err)

	_, err = ReadComment("example.go", source, "zz", 2)
	assert.Error(t, err)
}

func TestWriteComment_NewComment(t *testing.T) {
	source, err := os.ReadFile(testFixture)
	require.NoError(t, err)

	id := symbolIDFor(t, source, "main")
	result, err := WriteComment("example.go", source, id, []byte("main is the entry point."), 2)
	require.NoError(t, err)

	assert.Contains(t, string(result), "// main is the entry point.")
	assert.Contains(t, string(result), "func main()")
}

func TestWriteComment_ReplaceExisting(t *testing.T) {
	source, err := os.ReadFile(testFixture)
	require.NoError(t, err)

	id := symbolIDFor(t, source, "Config")
	result, err := WriteComment("example.go", source, id, []byte("Config is a replaced comment."), 2)
	require.NoError(t, err)

	assert.Contains(t, string(result), "// Config is a replaced comment.")
	assert.NotContains(t, string(result), "Config holds server configuration")
	assert.Contains(t, string(result), "type Config struct")
}

func TestWriteComment_PythonHashPrefix(t *testing.T) {
	source, err := os.ReadFile("../../testdata/src/example.py")
	require.NoError(t, err)

	symbols, err := treesitter.ExtractSymbols("example.py", source, 2)
	if err != nil {
		t.Skipf("Python grammar not available: %v", err)
	}
	nodes := treesitter.SymbolTree(symbols)

	// Find main function
	id := ""
	for i, s := range symbols {
		if s.Name == "main" {
			id = nodes[i].ID
		}
	}
	if id == "" {
		t.Skip("main function not found (Python grammar may not extract it)")
	}

	result, err := WriteComment("example.py", source, id, []byte("Entry point."), 2)
	require.NoError(t, err)

	assert.Contains(t, string(result), "# Entry point.")
}

func TestFormatComment_GoStyle(t *testing.T) {
	out := formatComment("Does something useful.", "go")
	assert.Equal(t, "// Does something useful.\n", out)
}

func TestFormatComment_PythonStyle(t *testing.T) {
	out := formatComment("Does something useful.", "python")
	assert.Equal(t, "# Does something useful.\n", out)
}

func TestFormatComment_MultiLine(t *testing.T) {
	out := formatComment("Line one.\nLine two.", "go")
	assert.Equal(t, "// Line one.\n// Line two.\n", out)
}
