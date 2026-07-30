package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchCommandReadsWebConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EXA_API_KEY", "")
	configDir := filepath.Join(home, ".config", "ttal")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "web.toml"),
		[]byte("[search\nprovider = \"brave\"\n"),
		0o600,
	))

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"test query"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "read web config")
}
