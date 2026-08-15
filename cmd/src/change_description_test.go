package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/srcop"
)

func TestSymbolMutationJSONUsesBatchChangeContract(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "sample.go")
	old := []byte("package sample\n\nfunc Foo() {\n\treturn 1\n}\n")
	newContent := []byte("package sample\n\nfunc Foo() {\n\treturn 2\n}\n")
	require.NoError(t, os.WriteFile(filename, old, 0o644))

	batch, err := srcop.ApplyBatch(filename, old, []srcop.BatchEdit{{
		OldText: string(old), NewText: string(newContent),
	}})
	require.NoError(t, err)

	var mutation mutationJSON
	require.NoError(t, json.Unmarshal([]byte(captureStdout(t, func() {
		require.NoError(t, writeMutationJSON(filename, "replace", "bK", old, newContent))
	})), &mutation))

	require.Equal(t, batch.Diff, mutation.Diff)
	require.Equal(t, batch.Patch, mutation.Patch)
	require.Equal(t, batch.FirstChangedLine, mutation.FirstChangedLine)
	require.Contains(t, mutation.Diff, "-4 ")
	require.NotContains(t, mutation.Diff, "--- ")
	require.Contains(t, mutation.Patch, "--- a/")
	require.Contains(t, mutation.Patch, "@@")
}
