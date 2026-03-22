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
