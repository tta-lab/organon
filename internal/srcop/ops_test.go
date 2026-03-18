package srcop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/treesitter"
)

const testFixture = "../../testdata/src/example.go"

func loadFixture(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile(testFixture)
	require.NoError(t, err)
	return src
}

// symbolIDFor finds the tree node ID for a symbol with the given name.
func symbolIDFor(t *testing.T, source []byte, name string) string {
	t.Helper()
	symbols, err := treesitter.ExtractSymbols("example.go", source, 2)
	require.NoError(t, err)
	nodes := treesitter.SymbolTree(symbols)
	for i, s := range symbols {
		if s.Name == name {
			return nodes[i].ID
		}
	}
	t.Fatalf("symbol %q not found", name)
	return ""
}

func TestReplace_UpdatesSymbol(t *testing.T) {
	source := loadFixture(t)
	id := symbolIDFor(t, source, "main")

	newContent := []byte("func main() {\n\t// replaced\n}")
	result, err := Replace("example.go", source, id, newContent, 2)
	require.NoError(t, err)

	// Verify old body is gone and new content is present
	assert.Contains(t, string(result), "// replaced")
	assert.NotContains(t, string(result), "Config{Host:")

	// Verify surrounding code is intact
	assert.Contains(t, string(result), "type Config struct")
}

func TestReplace_SymbolNotFound(t *testing.T) {
	source := loadFixture(t)
	_, err := Replace("example.go", source, "zz", []byte("replacement"), 2)
	assert.ErrorContains(t, err, "not found")
	assert.ErrorContains(t, err, "--tree")
}

func TestInsertAfter_AddsContent(t *testing.T) {
	source := loadFixture(t)
	id := symbolIDFor(t, source, "Config")

	newFunc := []byte("func newHelper() {}")
	result, err := InsertAfter("example.go", source, id, newFunc, 2)
	require.NoError(t, err)

	assert.Contains(t, string(result), "newHelper")
	// Config should come before newHelper
	configIdx := indexOf(result, []byte("type Config struct"))
	helperIdx := indexOf(result, []byte("func newHelper"))
	assert.True(t, configIdx < helperIdx, "Config should be before newHelper")
}

func TestInsertBefore_AddsContent(t *testing.T) {
	source := loadFixture(t)
	id := symbolIDFor(t, source, "Validate")

	comment := []byte("// BeforeValidate is a placeholder.")
	result, err := InsertBefore("example.go", source, id, comment, 2)
	require.NoError(t, err)

	assert.Contains(t, string(result), "BeforeValidate")
	// placeholder should come before Validate
	placeholderIdx := indexOf(result, []byte("BeforeValidate"))
	validateIdx := indexOf(result, []byte("func (c *Config) Validate"))
	assert.True(t, placeholderIdx < validateIdx, "placeholder should be before Validate")
}

func TestDelete_RemovesSymbol(t *testing.T) {
	source := loadFixture(t)
	id := symbolIDFor(t, source, "main")

	result, err := Delete("example.go", source, id, 2)
	require.NoError(t, err)

	assert.NotContains(t, string(result), "func main()")
	assert.Contains(t, string(result), "type Config struct")
}

func TestDelete_SymbolNotFound(t *testing.T) {
	source := loadFixture(t)
	_, err := Delete("example.go", source, "zz", 2)
	assert.ErrorContains(t, err, "not found")
}

func TestDelete_ResultParses(t *testing.T) {
	source := loadFixture(t)
	id := symbolIDFor(t, source, "main")

	result, err := Delete("example.go", source, id, 2)
	require.NoError(t, err)

	// Write to temp file and re-parse
	tmp := filepath.Join(t.TempDir(), "result.go")
	require.NoError(t, os.WriteFile(tmp, result, 0o644))

	symbols, err := treesitter.ExtractSymbols(tmp, result, 2)
	require.NoError(t, err)
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	assert.Contains(t, names, "Config")
	assert.NotContains(t, names, "main")
}

func indexOf(haystack, needle []byte) int {
	for i := range haystack {
		if i+len(needle) <= len(haystack) {
			match := true
			for j, b := range needle {
				if haystack[i+j] != b {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
	}
	return -1
}
