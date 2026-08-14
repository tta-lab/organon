package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newReplaceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "replace", Args: cobra.ExactArgs(1), RunE: runReplace}
	cmd.Flags().String("symbol-id", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.PersistentFlags().Int("depth", 2, "")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "delete", Args: cobra.ExactArgs(1), RunE: runDelete}
	cmd.Flags().String("symbol-id", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.PersistentFlags().Int("depth", 2, "")
	return cmd
}

func newCommentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "comment", Args: cobra.ExactArgs(1), RunE: runComment}
	cmd.Flags().String("symbol-id", "", "")
	cmd.Flags().Bool("read", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.PersistentFlags().Int("depth", 2, "")
	return cmd
}

func newEditBatchCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "edit", Args: cobra.ExactArgs(1), RunE: runEdit}
	cmd.Flags().String("symbol-id", "", "")
	cmd.Flags().String("before-file", "", "")
	cmd.Flags().String("after-file", "", "")
	cmd.Flags().Bool("edits-json", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.PersistentFlags().Int("depth", 2, "")
	return cmd
}

func captureJSON(t *testing.T, fn func()) string {
	return captureStdout(t, fn)
}

func TestReplaceJSONReturnsDiffAndWrites(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {\n\treturn 1\n}\n")
	outline := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	var fooID string
	for _, s := range outline.Symbols {
		if s.Name == "Foo" {
			fooID = s.ID
		}
	}
	require.NotEmpty(t, fooID)

	cmd := newReplaceCmd()
	require.NoError(t, cmd.Flags().Set("symbol-id", fooID))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	pipeStdin(t, []byte("func Foo() {\n\treturn 2\n}\n"), func() {
		var out mutationJSON
		require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
			require.NoError(t, runReplace(cmd, []string{f}))
		})), &out))
		assert.Equal(t, f, out.Path)
		assert.Equal(t, "replace", out.Action)
		assert.Equal(t, fooID, out.SymbolID)
		assert.Contains(t, out.Diff, "-\treturn 1")
		assert.Contains(t, out.Diff, "+\treturn 2")
		assert.Equal(t, 4, out.FirstChangedLine)
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "return 2")
}

func TestDeleteJSONReturnsDiff(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {}\n\nfunc Bar() {}\n")
	outline := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	var fooID string
	for _, s := range outline.Symbols {
		if s.Name == "Foo" {
			fooID = s.ID
		}
	}
	cmd := newDeleteCmd()
	require.NoError(t, cmd.Flags().Set("symbol-id", fooID))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var out mutationJSON
	require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
		require.NoError(t, runDelete(cmd, []string{f}))
	})), &out))
	assert.Equal(t, "delete", out.Action)
	assert.Contains(t, out.Diff, "-func Foo()")
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "func Foo()")
}

func TestCommentJSONReadAndWrite(t *testing.T) {
	f := writeGoFile(t, "package sample\n\n// Old docs.\nfunc Foo() {}\n")
	outline := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	var fooID string
	for _, s := range outline.Symbols {
		if s.Name == "Foo" {
			fooID = s.ID
		}
	}

	readCmd := newCommentCmd()
	require.NoError(t, readCmd.Flags().Set("symbol-id", fooID))
	require.NoError(t, readCmd.Flags().Set("read", "true"))
	require.NoError(t, readCmd.Flags().Set("json", "true"))
	var readOut commentJSON
	require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
		require.NoError(t, runComment(readCmd, []string{f}))
	})), &readOut))
	assert.Contains(t, readOut.Comment, "Old docs.")

	writeCmd := newCommentCmd()
	require.NoError(t, writeCmd.Flags().Set("symbol-id", fooID))
	require.NoError(t, writeCmd.Flags().Set("json", "true"))
	pipeStdin(t, []byte("// New docs.\n"), func() {
		var out mutationJSON
		require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
			require.NoError(t, runComment(writeCmd, []string{f}))
		})), &out))
		assert.Equal(t, "comment", out.Action)
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Contains(t, string(result), "New docs.")
	assert.NotContains(t, string(result), "Old docs.")
}

func TestEditBatchJSONAppliesAtomically(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.txt")
	require.NoError(t, os.WriteFile(f, []byte("alpha\nbeta\ngamma\n"), 0o644))
	envelope := `{"edits":[{"oldText":"alpha","newText":"ALPHA"},{"oldText":"gamma","newText":"GAMMA"}]}`
	cmd := newEditBatchCmd()
	require.NoError(t, cmd.Flags().Set("edits-json", "true"))
	pipeStdin(t, []byte(envelope), func() {
		var out editBatchJSON
		require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
			require.NoError(t, runEdit(cmd, []string{f}))
		})), &out))
		assert.Equal(t, 2, out.EditsApplied)
		assert.Equal(t, 1, out.FirstChangedLine)
		assert.NotEmpty(t, out.Diff)
		assert.NotEmpty(t, out.Patch)
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Equal(t, "ALPHA\nbeta\nGAMMA\n", string(result))
}

func TestEditBatchJSONFailureLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.txt")
	orig := []byte("alpha\nbeta\n")
	require.NoError(t, os.WriteFile(f, orig, 0o644))
	cmd := newEditBatchCmd()
	require.NoError(t, cmd.Flags().Set("edits-json", "true"))
	pipeStdin(t, []byte(`{"edits":[{"oldText":"alpha","newText":"A"},{"oldText":"nope","newText":"B"}]}`), func() {
		err := runEdit(cmd, []string{f})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "text not found")
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Equal(t, orig, result)
}

func TestEditBatchJSONRejectsInvalidEnvelope(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.txt")
	require.NoError(t, os.WriteFile(f, []byte("alpha\n"), 0o644))
	cmd := newEditBatchCmd()
	require.NoError(t, cmd.Flags().Set("edits-json", "true"))
	pipeStdin(t, []byte("not json"), func() {
		err := runEdit(cmd, []string{f})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --edits-json envelope")
	})
	pipeStdin(t, []byte(`{"edits":[]}`), func() {
		err := runEdit(cmd, []string{f})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})
}

func TestEditBatchJSONPreservesBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "win.txt")
	require.NoError(t, os.WriteFile(f, append([]byte("\xef\xbb\xbf"), []byte("a\r\nb\r\n")...), 0o644))
	cmd := newEditBatchCmd()
	require.NoError(t, cmd.Flags().Set("edits-json", "true"))
	pipeStdin(t, []byte(`{"edits":[{"oldText":"b","newText":"B"}]}`), func() {
		var out editBatchJSON
		require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
			require.NoError(t, runEdit(cmd, []string{f}))
		})), &out))
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(result), "\xef\xbb\xbfa\r\nB\r\n"))
}

func TestEditBatchJSONLargeFileNoCap(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.txt")
	big := strings.Repeat("line\n", 30*1024) // 150 KiB, still over the single-edit 100-KB cap
	require.NoError(t, os.WriteFile(f, []byte("head\n"+big+"tail\n"), 0o644))
	cmd := newEditBatchCmd()
	require.NoError(t, cmd.Flags().Set("edits-json", "true"))
	pipeStdin(t, []byte(`{"edits":[{"oldText":"head","newText":"HEAD"}]}`), func() {
		var out editBatchJSON
		require.NoError(t, json.Unmarshal([]byte(captureJSON(t, func() {
			require.NoError(t, runEdit(cmd, []string{f}))
		})), &out))
		assert.Equal(t, 1, out.EditsApplied)
	})
	result, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(result), "HEAD\n"))
}
