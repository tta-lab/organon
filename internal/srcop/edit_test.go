package srcop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- parseEditInput ----------

func TestParseEditInput_Valid(t *testing.T) {
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n"
	old, new, err := parseEditInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "old text\n", string(old))
	assert.Equal(t, "new text\n", string(new))
}

func TestParseEditInput_MissingBefore(t *testing.T) {
	input := "===AFTER===\nnew text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ===BEFORE===")
}

func TestParseEditInput_MissingAfter(t *testing.T) {
	input := "===BEFORE===\nold text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ===AFTER===")
}

func TestParseEditInput_IdenticalOldNew(t *testing.T) {
	input := "===BEFORE===\nsome text\n===AFTER===\nsome text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-op")
}

func TestParseEditInput_ExtraWhitespaceAroundDelimiters(t *testing.T) {
	input := "  ===BEFORE===  \nold text\n  ===AFTER===  \nnew text\n"
	old, new, err := parseEditInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "old text\n", string(old))
	assert.Equal(t, "new text\n", string(new))
}

func TestParseEditInput_EmptyBeforeBlock(t *testing.T) {
	input := "===BEFORE===\n===AFTER===\nnew text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BEFORE section is empty")
}

// ---------- findMatch — exact pass ----------

func TestFindMatch_ExactSingle(t *testing.T) {
	source := []byte("hello world\nfoo bar\nbaz\n")
	old := []byte("foo bar\n")
	start, end, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "exact", pass)
	assert.Equal(t, string(old), string(source[start:end]))
}

func TestFindMatch_ExactMultiple(t *testing.T) {
	source := []byte("foo bar\nfoo bar\n")
	old := []byte("foo bar\n")
	_, _, _, err := findMatch(source, old, "test.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 matches")
	assert.Contains(t, err.Error(), "line 1:")
	assert.Contains(t, err.Error(), "line 2:")
	assert.Contains(t, err.Error(), "foo bar")
}

func TestFindMatch_ExactMultipleNoFallThrough(t *testing.T) {
	// Pass 1 finds 2 exact matches. Pass 2 with trim-trailing would also find 2.
	// Test that error is returned immediately (no fall-through to pass 2).
	source := []byte("  foo  \n  foo  \n")
	old := []byte("  foo  \n")
	_, _, _, err := findMatch(source, old, "test.txt")
	require.Error(t, err)
	// Must error (not succeed via a later pass).
	assert.Contains(t, err.Error(), "found")
}

// ---------- findMatch — trim-trailing pass ----------

func TestFindMatch_TrimTrailingSpaces(t *testing.T) {
	source := []byte("hello   \nfoo bar   \nbaz\n")
	old := []byte("foo bar\n")
	start, end, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "trim-trailing", pass)
	// Byte range must point into original source (with trailing spaces), not normalized.
	assert.Equal(t, "foo bar   \n", string(source[start:end]))
}

func TestFindMatch_TrimTrailingTabs(t *testing.T) {
	source := []byte("hello\t\nfoo bar\t\nbaz\n")
	old := []byte("foo bar\n")
	start, end, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "trim-trailing", pass)
	// Byte range must point into original source (with trailing tab).
	assert.Equal(t, "foo bar\t\n", string(source[start:end]))
}

// ---------- findMatch — trim-both pass ----------

func TestFindMatch_TrimBothIndentation(t *testing.T) {
	source := []byte("func foo() {\n\t\treturn 42\n}\n")
	old := []byte("func foo() {\n        return 42\n}\n") // spaces instead of tabs
	_, _, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "trim-both", pass)
}

func TestFindMatch_TrimBothLeadingWhitespace(t *testing.T) {
	// A whole-line case where indentation differs — source indents with 4 spaces,
	// old block uses no indentation.
	source := []byte("before\n    indented line\nafter\n")
	old := []byte("before\nindented line\nafter\n")
	_, _, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "trim-both", pass)
}

// ---------- findMatch — unicode-fold pass ----------

func TestFindMatch_CurlyQuotes(t *testing.T) {
	source := []byte("He said \u201chello\u201d to her.\n")
	old := []byte("He said \"hello\" to her.\n")
	_, _, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "unicode-fold", pass)
}

func TestFindMatch_EmDash(t *testing.T) {
	source := []byte("foo \u2014 bar\n")
	old := []byte("foo - bar\n")
	_, _, pass, err := findMatch(source, old, "test.txt")
	require.NoError(t, err)
	assert.Equal(t, "unicode-fold", pass)
}

// ---------- findMatch — no match ----------

func TestFindMatch_NoMatch(t *testing.T) {
	source := []byte("hello world\nfoo bar\nbaz\n")
	old := []byte("completely different text\n")
	_, _, _, err := findMatch(source, old, "test.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	// No similar region exists — BEFORE shares no lines with source.
	assert.Contains(t, err.Error(), "no similar region")
}

func TestClosestRegion_NoOverlap_ReturnsNoMatchMessage(t *testing.T) {
	source := []byte("foo bar baz qux\nanother line here\n")
	old := []byte("completely unrelated text\nthat does not appear in source\n")
	region := closestRegion(source, old)
	assert.Contains(t, region, "no similar region", "should report no overlap when BEFORE shares no lines with source")
}

// ---------- findMatch — cascade with duplicates boundary ----------

func TestFindMatch_ExactDuplicateNoFallThrough(t *testing.T) {
	// Exact pass finds 2 matches. Even though pass 2 might return 1 match,
	// we must error on pass 1 immediately.
	source := []byte("alpha\nbeta\nalpha\nbeta\n")
	old := []byte("alpha\nbeta\n")
	_, _, _, err := findMatch(source, old, "test.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 matches")
}

// ---------- Edit integration ----------

func TestEdit_ValidReplacement(t *testing.T) {
	source := []byte("package example\n\nfunc Foo() {}\nfunc Bar() {}\n")
	input := "===BEFORE===\nfunc Foo() {}\n===AFTER===\nfunc Foo() { return 42 }\n"
	result, err := Edit("example.go", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result), "func Foo() { return 42 }")
	assert.Contains(t, string(result), "func Bar() {}")
}

func TestEdit_PlainText(t *testing.T) {
	source := []byte("Hello World\nThis is a test.\n")
	input := "===BEFORE===\nHello World\n===AFTER===\nHello Go\n"
	result, err := Edit("notes.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result), "Hello Go")
	assert.NotContains(t, string(result), "Hello World")
}

func TestEdit_BinaryFile(t *testing.T) {
	source := []byte("normal text\x00with null byte\n")
	input := "===BEFORE===\nnormal text\n===AFTER===\nreplaced\n"
	_, err := Edit("binary.bin", source, []byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "binary file")
}

func TestEdit_FileTooLarge(t *testing.T) {
	source := make([]byte, maxFileSize+1)
	for i := range source {
		source[i] = 'a'
	}
	input := "===BEFORE===\naaa\n===AFTER===\nbbb\n"
	_, err := Edit("big.txt", source, []byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestEdit_CRLFPreserved(t *testing.T) {
	// Source with CRLF line endings.
	source := []byte("line one\r\nline two\r\nline three\r\n")
	input := "===BEFORE===\nline two\n===AFTER===\nline TWO\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	// Result must still have CRLF line endings.
	assert.True(t, strings.Contains(string(result), "\r\n"), "result should preserve CRLF")
	assert.Contains(t, string(result), "line TWO")
	assert.Contains(t, string(result), "line one\r\n")
	assert.Contains(t, string(result), "line three\r\n")
}

// ---------- isBinary ----------

func TestIsBinary_Text(t *testing.T) {
	assert.False(t, isBinary([]byte("hello world\nsome text here\n")))
}

func TestIsBinary_NullBytes(t *testing.T) {
	assert.True(t, isBinary([]byte("data\x00more")))
}

// ---------- closestRegion ----------

func TestClosestRegion_PartialOverlap(t *testing.T) {
	source := []byte("line one\nline two\nline three\nline four\n")
	old := []byte("line two\nline five\n") // partial overlap with source
	region := closestRegion(source, old)
	assert.Contains(t, region, "line two")
	// Should include line numbers.
	assert.Contains(t, region, ":")
}

func TestClosestRegion_EmptySource(t *testing.T) {
	region := closestRegion([]byte{}, []byte("some text\n"))
	assert.Contains(t, region, "empty")
}

func TestClosestRegion_SourceShorterThanOld(t *testing.T) {
	source := []byte("one line\n")
	old := []byte("line a\nline b\nline c\n")
	region := closestRegion(source, old)
	assert.Contains(t, region, "no region to show")
}

// ---------- CRLF + normalized pass ----------

func TestEdit_CRLF_TrimTrailingPass(t *testing.T) {
	// CRLF source where match requires trim-trailing (trailing spaces on lines).
	source := []byte("line one  \r\nline two  \r\nline three\r\n")
	// old uses no trailing spaces — needs trim-trailing pass to match
	input := "===BEFORE===\nline two\n===AFTER===\nline TWO\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result), "line TWO")
	// Non-edited lines must retain CRLF endings.
	assert.Contains(t, string(result), "line one  \r\n")
	assert.Contains(t, string(result), "line three\r\n")
	// The replacement line must also get CRLF.
	assert.Contains(t, string(result), "line TWO\r\n")
}

// ---------- Empty AFTER (deletion) ----------

func TestEdit_EmptyAfter_DeletesText(t *testing.T) {
	source := []byte("line one\nline two\nline three\n")
	// Empty AFTER section — should delete the matched text.
	input := "===BEFORE===\nline two\n===AFTER===\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	assert.NotContains(t, string(result), "line two")
	assert.Contains(t, string(result), "line one")
	assert.Contains(t, string(result), "line three")
}

// TestEdit_UnicodeFold exercises the unicode-fold pass as an end-to-end Edit().
// Source uses curly quotes; BEFORE uses straight quotes.
// The unicode-fold pass matches them, and AFTER is inserted verbatim (straight quotes).
func TestEdit_UnicodeFold(t *testing.T) {
	source := []byte("He said \u201chello\u201d to her.\n")
	input := "===BEFORE===\nHe said \"hello\" to her.\n===AFTER===\nHe said \"hi\" to her.\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result), "He said \"hi\" to her.", "AFTER text must be applied via unicode-fold match")
	assert.NotContains(t, string(result), "\u201c", "matched source curly quotes replaced by AFTER")
}
