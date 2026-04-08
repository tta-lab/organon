package indent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReindent_Sp4ToTab(t *testing.T) {
	text := []byte("    func foo() {\n        return 42\n    }\n")
	target := Style{Tab, 0, "layer1:go"}

	result, from, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	assert.Equal(t, Space, from.Kind)
	assert.Equal(t, 4, from.Width)
	assert.Nil(t, warnings)
	assert.Contains(t, string(result), "\tfunc foo()")
	assert.Contains(t, string(result), "\t\treturn 42")
	assert.NotContains(t, string(result), "    func")
}

func TestReindent_TabToSp2(t *testing.T) {
	text := []byte("\tfunc foo() {\n\t\treturn 42\n\t}\n")
	target := Style{Space, 2, "layer1:ruby"}

	result, from, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	assert.Equal(t, Tab, from.Kind)
	assert.Nil(t, warnings)
	assert.Contains(t, string(result), "  func foo()")
	assert.Contains(t, string(result), "    return 42")
	assert.NotContains(t, string(result), "\tfunc")
}

func TestReindent_Sp2ToSp4(t *testing.T) {
	text := []byte("  func foo() {\n    return 42\n  }\n")
	target := Style{Space, 4, "layer1:rust"}

	result, from, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	assert.Equal(t, Space, from.Kind)
	assert.Equal(t, 2, from.Width)
	assert.Nil(t, warnings)
	assert.True(t, strings.HasPrefix(string(result), "    func foo()"),
		"first line should start with 4-space indent")
	assert.Contains(t, string(result), "        return 42")
	assert.Contains(t, string(result), "    }\n")
}

func TestReindent_SameStyle(t *testing.T) {
	text := []byte("\tfunc foo() {\n\t\treturn 42\n\t}\n")
	target := Style{Tab, 0, "layer1:go"}

	result, from, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	assert.Equal(t, Tab, from.Kind)
	assert.Nil(t, warnings)
	assert.Equal(t, text, result)
}

func TestReindent_UnknownSource(t *testing.T) {
	// Content with no detectable indent (no leading whitespace): treated as success (nothing to do).
	text := []byte("no indentation here\n")
	target := Style{Tab, 0, "layer1:go"}

	result, from, ok, warnings := Reindent(text, target)

	assert.True(t, ok) // no indent to transform = success
	assert.Equal(t, Unknown, from.Kind)
	assert.Nil(t, warnings)
	assert.Equal(t, text, result)
}

func TestReindent_MixedPrefixLine(t *testing.T) {
	// First line has mixed tab+space prefix: should be preserved + warning.
	// Second and third lines are clean tabs: should transform normally.
	text := []byte("\t    func foo() {\n\t\treturn 42\n\t}\n")
	target := Style{Space, 2, "layer1:ruby"} // from!=target to trigger reindent

	result, _, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	require.NotNil(t, warnings)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "mixed tab+space prefix")
	// First line with mixed prefix preserved.
	assert.True(t, strings.HasPrefix(string(result), "\t    func foo()"))
	// Other lines (clean tab) converted to spaces.
	assert.Contains(t, string(result), "  return 42")
	assert.Contains(t, string(result), "  }\n")
}

func TestReindent_NonDivisibleSpacePrefix(t *testing.T) {
	// 3 spaces when from=sp2: not cleanly divisible, should warn.
	text := []byte("   func foo() {\n  return 42\n}\n")
	target := Style{Tab, 0, "layer1:go"}

	result, _, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	require.NotNil(t, warnings)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "not divisible")
	// First line preserved (3 spaces not divisible by 2).
	assert.Contains(t, string(result), "   func")
}

func TestReindent_NoLeadingWhitespace(t *testing.T) {
	// Line with no leading whitespace should be untouched.
	text := []byte("func foo()\n")
	target := Style{Tab, 0, "layer1:go"}

	result, _, ok, warnings := Reindent(text, target)

	require.True(t, ok)
	assert.Nil(t, warnings)
	assert.Equal(t, text, result)
}
