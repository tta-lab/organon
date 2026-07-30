package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigReadsSearchProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.toml")
	require.NoError(t, os.WriteFile(path, []byte("[search]\nprovider = \"brave\"\n"), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "brave", cfg.Search.Provider)
}

func TestLoadConfigMissingFileUsesAutomaticSelection(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Search.Provider)
}

func TestLoadConfigRejectsInvalidSearchProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.toml")
	require.NoError(t, os.WriteFile(path, []byte("[search]\nprovider = \"google\"\n"), 0o600))

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.ErrorContains(t, err, "search.provider")
}
