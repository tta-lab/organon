package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShow_IdenticalContent_NoOutput(t *testing.T) {
	content := []byte("func main() {}\n")
	var buf bytes.Buffer
	err := Show(&buf, content, content, "test.go")
	require.NoError(t, err)
	assert.Empty(t, buf.String(), "identical content should produce no output")
}

func TestShow_DifferentContent_ProducesUnifiedDiff(t *testing.T) {
	old := []byte("func main() {\n\t// old\n}\n")
	new := []byte("func main() {\n\t// new\n}\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "test.go")
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "--- a/test.go", "unified diff header should reference original file")
	assert.Contains(t, output, "+++ b/test.go", "unified diff header should reference new file")
	assert.Contains(t, output, "@@", "unified diff should contain hunk markers")
	lines := strings.Split(output, "\n")
	hasMinus := false
	hasPlus := false
	for _, line := range lines {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			hasMinus = true
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			hasPlus = true
		}
	}
	assert.True(t, hasMinus, "diff should contain at least one - line")
	assert.True(t, hasPlus, "diff should contain at least one + line")
}

func TestShow_NoANSIEscapes(t *testing.T) {
	old := []byte("line one\nline two\n")
	new := []byte("line one\nline TWO\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "test.go")
	require.NoError(t, err)
	output := buf.String()
	assert.NotContains(t, output, "\x1b[", "output should contain no ANSI escape sequences")
	assert.NotContains(t, output, "\033[", "output should contain no ANSI escape sequences")
}

func TestShow_NoTempfilePath(t *testing.T) {
	old := []byte("line one\nline two\n")
	new := []byte("line ONE\nline two\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "test.go")
	require.NoError(t, err)
	output := buf.String()
	assert.NotContains(t, output, "/tmp/", "diff header should not contain tempfile paths")
	assert.NotContains(t, output, "src-diff", "diff header should not contain internal tempfile names")
}

func TestShow_MarkdownProducesDiff(t *testing.T) {
	old := []byte("# Overview\n\nSome text.\n")
	new := []byte("# Overview\n\nDifferent text.\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "notes.md")
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "--- a/notes.md", "markdown should produce unified diff header")
	assert.Contains(t, output, "+++ b/notes.md", "markdown should produce unified diff header")
	assert.NotEmpty(t, output, "markdown should produce diff output, not empty string")
}
