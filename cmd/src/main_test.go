package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/markdown"
)

func resolveID(t *testing.T, source []byte, headingText string) string {
	t.Helper()
	// Try each known ID by reading section and checking if it contains the heading text
	// Generate candidate IDs by trying the tree output
	tree, err := markdown.HeadingTree(source)
	require.NoError(t, err)

	// Extract IDs from tree output by looking for [XX] patterns
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
		if contains(content, headingText) {
			return id
		}
	}
	t.Fatalf("could not resolve ID for heading %q", headingText)
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
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

	// Get the section ID dynamically
	id := resolveID(t, content, "Section One")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("tree", false, "")
	cmd.Flags().StringP("symbol", "s", "", "")
	cmd.PersistentFlags().Int("depth", 2, "")
	require.NoError(t, cmd.Flags().Set("symbol", id))

	err := runTreeOrRead(cmd, []string{f})
	assert.NoError(t, err)
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
