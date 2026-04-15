package srcop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/indent"
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
	assert.Contains(t, err.Error(), "no ===BEFORE=== found")
}

func TestParseEditInput_MissingAfter(t *testing.T) {
	input := "===BEFORE===\nold text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ===AFTER=== found")
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
	assert.Contains(t, string(result.Content), "func Foo() { return 42 }")
	assert.Contains(t, string(result.Content), "func Bar() {}")
}

func TestEdit_PlainText(t *testing.T) {
	source := []byte("Hello World\nThis is a test.\n")
	input := "===BEFORE===\nHello World\n===AFTER===\nHello Go\n"
	result, err := Edit("notes.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "Hello Go")
	assert.NotContains(t, string(result.Content), "Hello World")
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
	assert.True(t, strings.Contains(string(result.Content), "\r\n"), "result should preserve CRLF")
	assert.Contains(t, string(result.Content), "line TWO")
	assert.Contains(t, string(result.Content), "line one\r\n")
	assert.Contains(t, string(result.Content), "line three\r\n")
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
	assert.Contains(t, string(result.Content), "line TWO")
	// Non-edited lines must retain CRLF endings.
	assert.Contains(t, string(result.Content), "line one  \r\n")
	assert.Contains(t, string(result.Content), "line three\r\n")
	// The replacement line must also get CRLF.
	assert.Contains(t, string(result.Content), "line TWO\r\n")
}

// ---------- Empty AFTER (deletion) ----------

func TestEdit_EmptyAfter_DeletesText(t *testing.T) {
	source := []byte("line one\nline two\nline three\n")
	// Empty AFTER section — should delete the matched text.
	input := "===BEFORE===\nline two\n===AFTER===\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	assert.NotContains(t, string(result.Content), "line two")
	assert.Contains(t, string(result.Content), "line one")
	assert.Contains(t, string(result.Content), "line three")
}

// ---------- EditResult — pass disclosure ----------

func TestEdit_ReturnsPass_Exact(t *testing.T) {
	source := []byte("package example\n\nfunc Foo() {}\n")
	input := "===BEFORE===\nfunc Foo() {}\n===AFTER===\nfunc Foo() { return 1 }\n"
	result, err := Edit("example.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "exact", result.Pass)
}

func TestEdit_ReturnsPass_TrimBoth(t *testing.T) {
	// Source has 4-space indent; BEFORE uses tab indent — needs trim-both.
	source := []byte("    func foo() {\n        return 42\n    }\n")
	input := "===BEFORE===\n\tfunc foo() {\n\t\treturn 42\n\t}\n===AFTER===\n\tfunc foo() {\n\t\treturn 99\n\t}\n"
	result, err := Edit("example.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "trim-both", result.Pass)
}

func TestEdit_ReturnsPass_TrimTrailing(t *testing.T) {
	// Source matches BEFORE exactly except BEFORE has extra trailing spaces on a line.
	// The trim-trailing pass strips trailing whitespace and matches.
	source := []byte("func foo() {\n\treturn 1\n}\n")
	// Note trailing spaces after "return 1  "
	input := "===BEFORE===\nfunc foo() {\n\treturn 1  \n}\n===AFTER===\nfunc foo() {\n\treturn 99\n}\n"
	result, err := Edit("example.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "trim-trailing", result.Pass)
	assert.Contains(t, string(result.Content), "return 99")
}

// ---------- EditResult — reindent wiring ----------

func TestEdit_TrimBothPass_ReindentsAfterToTabs(t *testing.T) {
	// Go source (tab-indented), BEFORE uses 4-space indent → trim-both match.
	// AFTER uses 4-space indent → should be reindented to tabs.
	source := []byte("package main\n\nfunc foo() {\n\treturn 1\n}\n")
	// BEFORE has 4-space indent, needs trim-both to match source.
	input := "===BEFORE===\n    func foo() {\n        return 1\n    }\n" +
		"===AFTER===\n    func foo() {\n        return 99\n    }\n"
	result, err := Edit("main.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "trim-both", result.Pass)
	assert.True(t, result.Reindented)
	assert.Equal(t, indent.Tab, result.IndentTo.Kind)
	// The replacement should contain tabs, not 4 spaces.
	assert.Contains(t, string(result.Content), "\treturn 99")
	assert.NotContains(t, string(result.Content), "    return 99")
}

func TestEdit_ExactPass_DoesNotReindent(t *testing.T) {
	// Exact match — no reindent should happen.
	source := []byte("package main\n\nfunc foo() {\n\treturn 1\n}\n")
	input := "===BEFORE===\nfunc foo() {\n\treturn 1\n}\n===AFTER===\nfunc foo() {\n\treturn 99\n}\n"
	result, err := Edit("main.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "exact", result.Pass)
	assert.False(t, result.Reindented)
}

func TestEdit_UnknownTarget_EmitsWarning(t *testing.T) {
	// File with unknown style (txt) → reindent skipped, warning emitted.
	source := []byte("some text\nmore text\n")
	input := "===BEFORE===\nsome text\n===AFTER===\nnew text\n"
	result, err := Edit("notes.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "exact", result.Pass)
	assert.False(t, result.Reindented)
	// Unknown target means no reindent was possible, but no error either.
	// The "could not detect AFTER indent style" warning only fires when
	// pass is trim-both AND target is unknown — here pass is exact, so no warning.
	assert.Empty(t, result.Warnings)
}

func TestEdit_TrimBoth_UnindentedAfter_GetsIndentFromMatchedBefore(t *testing.T) {
	// Bug fix: un-indented BEFORE on tab-indented Go file. AFTER (also un-indented)
	// must be written WITH the matched BEFORE's indent depth, not as-is.
	// Source: tab-indented Go file. BEFORE has no indent (needs trim-both to match).
	source := []byte("package main\n\nfunc A() {\n\tx := 1\n\ty := 2\n\treturn\n}\n")
	// BEFORE lines have no leading whitespace → need trim-both to match.
	input := "===BEFORE===\nx := 1\ny := 2\n===AFTER===\nx := 7\ny := 7\n"
	result, err := Edit("main.go", source, []byte(input))
	require.NoError(t, err)
	assert.Equal(t, "trim-both", result.Pass)
	assert.True(t, result.Reindented)
	assert.Equal(t, indent.Tab, result.IndentTo.Kind)
	// AFTER lines must have leading tabs in the result, matching the matched BEFORE depth.
	assert.Contains(t, string(result.Content), "\tx := 7")
	assert.Contains(t, string(result.Content), "\ty := 7")
	// The bare un-indented lines must NOT appear (no match for "newline + no tab").
	assert.NotContains(t, string(result.Content), "\nx := 7")
	assert.NotContains(t, string(result.Content), "\ny := 7")
}

// TestEdit_UnicodeFold exercises the unicode-fold pass as an end-to-end Edit().
// Source uses curly quotes; BEFORE uses straight quotes.
// The unicode-fold pass matches them, and AFTER is inserted verbatim (straight quotes).
func TestEdit_UnicodeFold(t *testing.T) {
	source := []byte("He said \u201chello\u201d to her.\n")
	input := "===BEFORE===\nHe said \"hello\" to her.\n===AFTER===\nHe said \"hi\" to her.\n"
	result, err := Edit("file.txt", source, []byte(input))
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "He said \"hi\" to her.",
		"AFTER text must be applied via unicode-fold match")
	assert.NotContains(t, string(result.Content), "\u201c", "matched source curly quotes replaced by AFTER")
}

// ---------- strict marker counting ----------

func TestParseEditInput_ZeroMarkers(t *testing.T) {
	input := "some content\nwith no markers\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ===BEFORE=== found")
}

func TestParseEditInput_OnlyBeforeNoAfter(t *testing.T) {
	input := "===BEFORE===\nold text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ===AFTER=== found")
}

func TestParseEditInput_NormalSingleMarker(t *testing.T) {
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n"
	old, new, err := parseEditInput([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "old text\n", string(old))
	assert.Equal(t, "new text\n", string(new))
}

func TestParseEditInput_TrailingAfterMarker(t *testing.T) {
	// Model habit: trailing ===AFTER=== after the actual new text.
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n===AFTER===\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "This looks like a trailing")
}

func TestParseEditInput_BeforeContentContainsAfterMarker(t *testing.T) {
	// Literal ===AFTER=== line in BEFORE content — must error, not silently corrupt.
	input := "===BEFORE===\nold text\n===AFTER===\nmore old\n===AFTER===\nnew text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
}

func TestParseEditInput_DuplicateBeforeMarker(t *testing.T) {
	input := "some content\n===BEFORE===\n===BEFORE===\nold text\n===AFTER===\nnew text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===BEFORE===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
}

func TestParseEditInput_DuplicateAfterMarker(t *testing.T) {
	// Genuine duplicate: two ===AFTER=== markers with content after the second.
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n===AFTER===\nmore content\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
}

func TestParseEditInput_TrailingAfterMarker_NoTrailingNewline(t *testing.T) {
	// Trailing case with no newline at EOF — still detected as trailing.
	input := "===BEFORE===\nold\n===AFTER===\nnew\n===AFTER==="
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This looks like a trailing")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
}

func TestParseEditInput_DuplicateBeforeMarker_ShowsFraming(t *testing.T) {
	input := "some content\n===BEFORE===\n===BEFORE===\nold text\n===AFTER===\nnew text\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===BEFORE===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
	assert.Contains(t, err.Error(), "src edit")
}

func TestParseEditInput_TwoCompleteEdits_SuggestsMultipleCalls(t *testing.T) {
	input := "===BEFORE===\nold1\n===AFTER===\nnew1\n===BEFORE===\nold2\n===AFTER===\nnew2\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===BEFORE===")
	assert.Contains(t, err.Error(), "src edit")
}

func TestParseEditInput_ThreeBeforeMarkers(t *testing.T) {
	input := "===BEFORE===\nline1\n===BEFORE===\nline2\n===BEFORE===\nline3\n===AFTER===\nnew\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 3 lines matching ===BEFORE===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs")
	assert.Contains(t, err.Error(), "src edit")
}

// ---------- EditDirect ----------

func TestEditDirect_ValidReplacement(t *testing.T) {
	source := []byte("package example\n\nfunc Foo() {}\nfunc Bar() {}\n")
	oldText := []byte("func Foo() {}\n")
	newText := []byte("func Foo() { return 42 }\n")
	result, err := EditDirect("example.go", source, oldText, newText)
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "func Foo() { return 42 }")
	assert.Contains(t, string(result.Content), "func Bar() {}")
}

func TestEditDirect_EmptyOldText(t *testing.T) {
	source := []byte("hello world\n")
	_, err := EditDirect("file.txt", source, []byte{}, []byte("new\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oldText is empty")
}

func TestEditDirect_IdenticalOldNew(t *testing.T) {
	source := []byte("hello world\n")
	_, err := EditDirect("file.txt", source, []byte("hello\n"), []byte("hello\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-op")
}

func TestEditDirect_BinaryFile(t *testing.T) {
	source := []byte("normal\x00null\n")
	_, err := EditDirect("binary.bin", source, []byte("null\n"), []byte("data\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "binary file")
}

func TestEditDirect_FileTooLarge(t *testing.T) {
	source := make([]byte, maxFileSize+1)
	for i := range source {
		source[i] = 'a'
	}
	_, err := EditDirect("big.txt", source, []byte("aaa\n"), []byte("bbb\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestEditDirect_NoMatch(t *testing.T) {
	source := []byte("hello world\n")
	_, err := EditDirect("file.txt", source, []byte("not present\n"), []byte("new\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ---------- isTrailingAfterDelimiter — edge cases ----------

func TestParseEditInput_TrailingAfterMarker_HasFraming(t *testing.T) {
	// Trailing case: N=2 markers, all lines after second are blank.
	// Error must include the "section headers, not tag pairs" framing.
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n===AFTER===\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 2 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "section headers, not tag pairs",
		"trailing branch must include orientation framing")
}

func TestParseEditInput_TrailingAfterMarker_BlankThenContent_FallsThroughToGeneric(t *testing.T) {
	// Blank line after second ===AFTER===, then content — this is a genuine
	// literal-marker case, NOT trailing. Must NOT emit the trailing error.
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n===AFTER===\n\nsome content here\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	// Must be the generic duplicate message, not the trailing one.
	assert.Contains(t, err.Error(), "found 2 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "src edit")
	assert.NotContains(t, err.Error(), "This looks like a trailing",
		"blank-then-content pattern must not trigger trailing framing")
}

func TestParseEditInput_NThreeAfterMarkers_FallsThroughToGeneric(t *testing.T) {
	// N=3 markers — ambiguous, fall through to generic (not trailing).
	input := "===BEFORE===\nold text\n===AFTER===\nnew text\n===AFTER===\n===AFTER===\n"
	_, _, err := parseEditInput([]byte(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "found 3 lines matching ===AFTER===")
	assert.Contains(t, err.Error(), "src edit")
	assert.NotContains(t, err.Error(), "This looks like a trailing",
		"N=3 must fall through to generic, not trailing framing")
}
