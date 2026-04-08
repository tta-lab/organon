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
