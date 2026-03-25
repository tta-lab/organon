package diff

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownTools lists tools the detection loop recognises, in priority order.
var knownTools = []string{"delta", "diff-so-fancy", "colordiff", "diff"}

func TestShow_IdenticalContent_NoOutput(t *testing.T) {
	content := []byte("func main() {}\n")
	var buf bytes.Buffer
	err := Show(&buf, content, content, "test.go")
	require.NoError(t, err)
	assert.Empty(t, buf.String(), "identical content should produce no output")
}

func TestShow_DifferentContent_ProducesOutput(t *testing.T) {
	old := []byte("func main() {\n\t// old\n}\n")
	new := []byte("func main() {\n\t// new\n}\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "test.go")
	require.NoError(t, err)
	output := buf.String()
	assert.NotEmpty(t, output, "diff should produce output for different content")
}

func TestShow_DifferentExtension_ProducesOutput(t *testing.T) {
	old := []byte("def main():\n    pass\n")
	new := []byte("def main():\n    print('hello')\n")
	var buf bytes.Buffer
	err := Show(&buf, old, new, "example.py")
	require.NoError(t, err)
	assert.NotEmpty(t, buf.String(), "diff should work for .py files too")
}

func TestDetectTool_FindsSomething(t *testing.T) {
	detectTool()
	assert.NotEmpty(t, toolName, "should detect at least plain diff")
	assert.NotEmpty(t, toolPath, "should have a path for the detected tool")

	// toolName must be one of the recognised tools.
	assert.Contains(t, knownTools, toolName, "detected tool should be in the known list")

	// toolPath must point to an executable file.
	info, err := exec.LookPath(toolName)
	require.NoError(t, err)
	assert.Equal(t, toolPath, info, "toolPath should match LookPath result for toolName")
}
