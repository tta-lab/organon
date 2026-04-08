package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/markdown"
	"github.com/tta-lab/organon/internal/treesitter"
)

// resolveID finds the section ID for a heading by text.
// It parses [XX] bracket patterns from HeadingTree output and matches them via ReadSection.
// This workaround is necessary because main_test.go is in package main and cannot access
// the unexported parseHeadings/assignIDs internals used by edit_test.go's sectionIDFor.
func resolveID(t *testing.T, source []byte, headingText string) string {
	t.Helper()
	tree, err := markdown.HeadingTree(source)
	require.NoError(t, err)

	// Extract IDs from [XX] bracket patterns in the tree output
	var candidates []string
	for i := 0; i < len(tree)-3; i++ {
		if tree[i] == '[' {
			end := i + 1
			for end < len(tree) && tree[end] != ']' {
				end++
			}
			if end < len(tree) {
				candidates = append(candidates, tree[i+1:end])
			}
		}
	}

	for _, id := range candidates {
		content, err := markdown.ReadSection(source, id)
		if err != nil {
			continue
		}
		if strings.Contains(content, headingText) {
			return id
		}
	}
	t.Fatalf("could not resolve ID for heading %q", headingText)
	return ""
}

func TestMarkdownDispatch_Tree(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.md")
	require.NoError(t, err)
	content := []byte("# Doc\n\n## Section One\n\nContent.\n\n## Section Two\n\nMore content.\n")
	require.NoError(t, f.Close())
	require.NoError(t, os.WriteFile(f.Name(), content, 0o644))

	cmd := &cobra.Command{}
	cmd.Flags().Bool("tree", false, "")
	cmd.Flags().StringP("symbol", "s", "", "")
	cmd.PersistentFlags().Int("depth", 2, "")

	err = runTreeOrRead(cmd, []string{f.Name()})
	assert.NoError(t, err)
}

func TestMarkdownDispatch_ReadSection(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	content := []byte("# Doc\n\n## Section One\n\nContent here.\n\n## Section Two\n\nOther.\n")
	require.NoError(t, os.WriteFile(f, content, 0o644))

	id := resolveID(t, content, "Section One")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("tree", false, "")
	cmd.Flags().StringP("symbol", "s", "", "")
	cmd.PersistentFlags().Int("depth", 2, "")
	require.NoError(t, cmd.Flags().Set("symbol", id))

	err := runTreeOrRead(cmd, []string{f})
	assert.NoError(t, err)
}

func TestMarkdownDispatch_Replace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	content := []byte("# Doc\n\n## Section One\n\nOld content.\n\n## Section Two\n\nOther.\n")
	require.NoError(t, os.WriteFile(f, content, 0o644))

	id := resolveID(t, content, "Section One")

	// Pipe new content via stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.Write([]byte("## Section One\n\nNew content.\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cmd := &cobra.Command{}
	cmd.Flags().StringP("symbol", "s", "", "")
	cmd.PersistentFlags().Int("depth", 2, "")
	require.NoError(t, cmd.Flags().Set("symbol", id))

	err = runReplace(cmd, []string{f})
	require.NoError(t, r.Close())
	assert.NoError(t, err)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "New content.")
	assert.NotContains(t, string(result), "Old content.")
	assert.Contains(t, string(result), "## Section Two")
}

func TestMarkdownDispatch_CommentUnsupported(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(f, []byte("# Doc\n\n## Section\n\nContent.\n"), 0o644))

	cmd := &cobra.Command{}
	cmd.Flags().StringP("symbol", "s", "", "")
	cmd.Flags().Bool("read", false, "")
	cmd.PersistentFlags().Int("depth", 2, "")
	require.NoError(t, cmd.Flags().Set("symbol", "xx"))

	err := runComment(cmd, []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported for markdown files")
	assert.Contains(t, err.Error(), "replace -s <id>")
}

// pipeStdin replaces os.Stdin with a pipe that contains content, runs fn, then restores.
func pipeStdin(t *testing.T, content []byte, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	old := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = old
		r.Close()
	}()
	fn()
}

// newEditCmd builds a cobra root command with the edit subcommand registered,
// following the pattern required for getDepth (PersistentFlags on root).
func newEditCmd() *cobra.Command {
	root := &cobra.Command{Use: "src"}
	root.PersistentFlags().Int("depth", 2, "")
	edit := &cobra.Command{
		Use:  "edit <file>",
		Args: cobra.ExactArgs(1),
		RunE: runEdit,
	}
	edit.Flags().StringP("section", "s", "", "")
	root.AddCommand(edit)
	return root
}

func TestEdit_ErrorDoesNotPrintUsage(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	orig := []byte("package example\n\nfunc Foo() {}\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	// Deliberately mismatched indentation — will fail to match.
	stdin := []byte("===BEFORE===\n    Host string\n===AFTER===\n    Host string // comment\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.NotContains(t, runErr.Error(), "Usage:", "runtime errors should not print usage block")
	assert.NotContains(t, runErr.Error(), "Flags:", "runtime errors should not print flag help")
}

func TestEdit_AppliesReplacement(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "example.go")
	orig := []byte("package example\n\ntype Config struct {\n\tHost string\n\tPort int\n}\n")
	require.NoError(t, os.WriteFile(src, orig, 0o644))

	stdin := []byte("===BEFORE===\nHost string\n===AFTER===\nHost string // server hostname\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", src})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Contains(t, string(result), "Host string // server hostname")
	assert.Contains(t, string(result), "Port int") // surrounding code intact
}

func TestEdit_InvalidDelimiters(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	orig := []byte("package example\n\nfunc Foo() {}\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	stdin := []byte("no delimiters here\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "missing ===BEFORE===")
}

func TestEdit_NoMatch(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	orig := []byte("package example\n\nfunc Foo() {}\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	stdin := []byte("===BEFORE===\nthis text does not exist in the file at all\n===AFTER===\nreplacement\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "not found")
}

func TestEdit_WorksOnMarkdown(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notes.md")
	orig := []byte("# Notes\n\n## Overview\n\nThis is an example document.\n\n## Details\n\nSome details here.\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	stdin := []byte("===BEFORE===\nThis is an example document.\n===AFTER===\nThis is a sample document.\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "This is a sample document.")
	assert.NotContains(t, string(result), "This is an example document.")
}

func TestEdit_WorksOnPython(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "example.py")
	orig := []byte("class Config:\n    def __init__(self, host):\n        self.host = host\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	stdin := []byte("===BEFORE===\n        self.host = host\n===AFTER===\n        self.host = host.strip()\n")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, stdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "self.host = host.strip()")
}

// ---------- unsupported file types — exit 0 after successful edit ----------

func testUnsupportedFileExitZero(t *testing.T, filename, orig, stdin, want string) {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(f, []byte(orig), 0o644))

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte(stdin), func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr, "edit on %s should exit 0", filename)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), want)
}

func TestEdit_UnsupportedFile_Env(t *testing.T) {
	testUnsupportedFileExitZero(t, ".env",
		"FOO=bar\nBAZ=qux\n",
		"===BEFORE===\nFOO=bar\n===AFTER===\nFOO=baz\n",
		"FOO=baz\nBAZ=qux\n")
}

func TestEdit_UnsupportedFile_Jsonc(t *testing.T) {
	testUnsupportedFileExitZero(t, "x.jsonc",
		`{ "v": "1" }`+"\n",
		"===BEFORE===\n{ \"v\": \"1\" }\n===AFTER===\n{ \"v\": \"2\" }\n",
		`{ "v": "2" }`)
}

func TestEdit_UnsupportedFile_Extensionless(t *testing.T) {
	testUnsupportedFileExitZero(t, "noext",
		"line\n",
		"===BEFORE===\nline\n===AFTER===\nLINE\n",
		"LINE\n")
}

func TestEdit_UnsupportedFile_Editorconfig(t *testing.T) {
	testUnsupportedFileExitZero(t, ".editorconfig",
		"[*]\nindent_style = space\n",
		"===BEFORE===\nindent_style = space\n===AFTER===\nindent_style = tab\n",
		"indent_style = tab\n")
}

func TestEdit_UnsupportedFile_Text(t *testing.T) {
	testUnsupportedFileExitZero(t, "notes.txt",
		"hello world\n",
		"===BEFORE===\nhello world\n===AFTER===\nhello world!\n",
		"hello world!\n")
}

func TestEdit_SectionOnUnsupportedFile_FailsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	orig := "FOO=bar\nBAZ=qux\n"
	require.NoError(t, os.WriteFile(f, []byte(orig), 0o644))

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte("===BEFORE===\nFOO=bar\n===AFTER===\nFOO=baz\n"), func() {
		root.SetArgs([]string{"edit", "-s", "anyid", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "unsupported file type")

	// File must be unchanged — no half-applied edits.
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Equal(t, orig, string(result))
}

// ---------- edit --section ----------

func extractSymbolID(t *testing.T, filename string, source []byte, depth int, labelText string) string {
	t.Helper()
	symbols, err := treesitter.ExtractSymbols(filename, source, depth)
	require.NoError(t, err)
	nodes := treesitter.SymbolTree(symbols)
	for _, n := range nodes {
		if strings.Contains(n.Label, labelText) {
			return n.ID
		}
	}
	t.Fatalf("symbol with label %q not found", labelText)
	return ""
}

func TestEditCmd_ScopedResolvesAmbiguity(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "ambiguous.go")
	orig := []byte("package main\n\nfunc foo() int {\n\treturn 1\n}\n\nfunc bar() int {\n\treturn 1\n}\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	firstID := extractSymbolID(t, f, orig, 2, "foo")

	// Unscoped edit: ambiguous match (same code in two functions) → should fail.
	unscopedStdin := []byte("===BEFORE===\n\treturn 1\n===AFTER===\n\treturn 42\n")
	root := newEditCmd()
	var runErr error
	pipeStdin(t, unscopedStdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "found 2 matches")

	// Scoped edit to first function only.
	scopedStdin := []byte("===BEFORE===\n\treturn 1\n===AFTER===\n\treturn 42\n")
	pipeStdin(t, scopedStdin, func() {
		root.SetArgs([]string{"edit", "-s", firstID, f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	// First function modified.
	assert.Contains(t, string(result), "return 42")
	// Second function untouched.
	assert.Contains(t, string(result), "return 1")
}

func TestEditCmd_ScopedMarkdown(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notes.md")
	orig := []byte("# Notes\n\n## Section A\n\nSame line.\n\n## Section B\n\nSame line.\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))

	idA := resolveID(t, orig, "Section A")

	// Unscoped edit: "Same line." appears in both sections → ambiguous.
	unscopedStdin := []byte("===BEFORE===\nSame line.\n===AFTER===\nDifferent line.\n")
	root := newEditCmd()
	var runErr error
	pipeStdin(t, unscopedStdin, func() {
		root.SetArgs([]string{"edit", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "found 2 matches")

	// Scoped edit to Section A only.
	scopedStdin := []byte("===BEFORE===\nSame line.\n===AFTER===\nDifferent line.\n")
	pipeStdin(t, scopedStdin, func() {
		root.SetArgs([]string{"edit", "-s", idA, f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "Different line.")
	assert.Contains(t, string(result), "Same line.") // Section B untouched
}

func TestEditCmd_ScopedSymbolNotFound(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	require.NoError(t, os.WriteFile(f, []byte("package main\n\nfunc main() {}\n"), 0o644))

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte("===BEFORE===\nfunc main()\n===AFTER===\nfunc main() {}\n"), func() {
		root.SetArgs([]string{"edit", "-s", "notfoundid", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "not found")
	assert.Contains(t, runErr.Error(), "--tree")
}

func TestEditCmd_ScopedNestedSymbol_LineBoundaryExtension(t *testing.T) {
	// Uses testdata/example.go: a struct field whose tree-sitter StartByte may land
	// mid-line (e.g., after a comment or inside a multi-line declaration).
	// Verifies that runEditScoped extends to line boundaries so the edit succeeds.
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "src", "example.go"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f, src, 0o644))

	hostID := extractSymbolID(t, f, src, 2, "Host")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte("===BEFORE===\n\tHost string\n===AFTER===\n\tHost string // server hostname\n"), func() {
		root.SetArgs([]string{"edit", "-s", hostID, f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr)

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "Host string // server hostname")
}

func TestEditCmd_ScopedEmptyTree(t *testing.T) {
	// .go file with only 'package main' (no symbols). resolveSectionBounds should
	// return a clear 'symbol not found' error suggesting --tree.
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.go")
	require.NoError(t, os.WriteFile(f, []byte("package main\n"), 0o644))

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte("===BEFORE===\npackage main\n===AFTER===\npackage main\n"), func() {
		root.SetArgs([]string{"edit", "-s", "anyid", f})
		runErr = root.Execute()
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "not found")
	assert.Contains(t, runErr.Error(), "--tree")
}

// ---------- line boundary helpers ----------

func TestLineStartAt(t *testing.T) {
	src := []byte("line1\nline2\nline3")

	tests := []struct {
		pos      int
		expected int
		desc     string
	}{
		{0, 0, "at start of first line"},
		{1, 0, "mid first line"},
		{5, 0, "at newline (end of line1)"},
		{6, 6, "start of second line"},
		{10, 6, "mid second line"},
		{11, 6, "at second newline"},
		{12, 12, "start of third line"},
		{17, 12, "end of third line (no trailing newline)"},
		{18, 12, "past end of source"},
		{100, 12, "far past end"},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := lineStartAt(src, tc.pos)
			assert.Equal(t, tc.expected, got, "pos=%d", tc.pos)
		})
	}
}

func TestLineEndAfter(t *testing.T) {
	// "line1\nline2\nline3": newlines at 5, 11; len=17
	src := []byte("line1\nline2\nline3")

	tests := []struct {
		pos      int
		expected int
		desc     string
	}{
		{0, 5, "at start of line1: scans to newline at 5, returns 5"},
		{1, 5, "mid line1: scans to newline at 5"},
		{5, 6, "at first newline: returns 6 (pos+1)"},
		{6, 11, "at start of line2: scans to newline at 11"},
		{10, 11, "mid line2: scans to newline at 11"},
		{11, 12, "at second newline: returns 12 (pos+1)"},
		{12, 17, "at start of line3 (no trailing newline): returns len"},
		{17, 17, "at end of source"},
		{18, 17, "past end of source"},
		{100, 17, "far past end"},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := lineEndAfter(src, tc.pos)
			assert.Equal(t, tc.expected, got, "pos=%d", tc.pos)
		})
	}
}

func TestLineBoundaryExtension_Integration(t *testing.T) {
	// testdata/example.go: "// Config holds server configuration.\ntype Config struct {\n\tHost string\n..."
	// Host symbol tree-sitter range is [61:70): 'H' through 'g' of "Host string".
	// Leading tab is at byte 60, newline after field at byte 71.
	dir := t.TempDir()
	f := filepath.Join(dir, "example.go")
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "src", "example.go"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f, src, 0o644))

	hostID := extractSymbolID(t, f, src, 2, "Host")

	root := newEditCmd()
	var runErr error
	pipeStdin(t, []byte("===BEFORE===\n\tHost string\n===AFTER===\n\tHost string // server hostname\n"), func() {
		root.SetArgs([]string{"edit", "-s", hostID, f})
		runErr = root.Execute()
	})
	require.NoError(t, runErr, "line boundary extension should succeed")

	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "Host string // server hostname")
}
