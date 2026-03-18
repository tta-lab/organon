package tree

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRender_Empty(t *testing.T) {
	assert.Equal(t, "(empty)\n", Render(nil))
}

func TestRender_FlatList(t *testing.T) {
	nodes := []Node{
		{ID: "aE", Label: "func main()", Level: 1, Meta: "[L1-L15]"},
		{ID: "bK", Label: "func handleRequest()", Level: 1, Meta: "[L17-L45]"},
	}
	out := Render(nodes)
	assert.Contains(t, out, "[aE] func main()")
	assert.Contains(t, out, "[bK] func handleRequest()")
	assert.Contains(t, out, "├──")
	assert.Contains(t, out, "└──")
}

func TestRender_Nested(t *testing.T) {
	nodes := []Node{
		{ID: "aE", Label: "type Config struct", Level: 1, Meta: "[L3-L8]"},
		{ID: "a1", Label: "Host string", Level: 2, Meta: "[L4]"},
		{ID: "a2", Label: "Port int", Level: 2, Meta: "[L5]"},
		{ID: "bK", Label: "func main()", Level: 1, Meta: "[L10-L12]"},
	}
	out := Render(nodes)
	assert.Contains(t, out, "│   ") // nested indent under ├── parent
}

func TestRender_IndentAfterLastSibling(t *testing.T) {
	// After └── parent, children should use "    " not "│   "
	nodes := []Node{
		{ID: "aE", Label: "type A", Level: 1},
		{ID: "bK", Label: "type B", Level: 1}, // last at level 1
		{ID: "b1", Label: "field X", Level: 2},
	}
	out := Render(nodes)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "field X") {
			assert.True(t, strings.HasPrefix(line, "    "),
				"children under └── should use space indent, got: %q", line)
			assert.False(t, strings.HasPrefix(line, "│"),
				"children under └── should NOT use │ indent, got: %q", line)
		}
	}
}
