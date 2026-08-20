package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchCommandPassesExplicitProviderAndIgnoresLegacyWebConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "ttal")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "web.toml"),
		[]byte("[search\nprovider = \"brave\"\n"),
		0o600,
	))

	var gotProvider string
	cmd := newSearchCmdWithFactory(func(provider string) (webService, error) {
		gotProvider = provider
		return &stubWebService{}, nil
	})
	cmd.SetArgs([]string{"test query", "--provider", "exa"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "exa", gotProvider)
}

func TestSearchCommandLeavesProviderEmptyForFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotProvider string
	cmd := newSearchCmdWithFactory(func(provider string) (webService, error) {
		gotProvider = provider
		return &stubWebService{}, nil
	})
	cmd.SetArgs([]string{"test query"})

	require.NoError(t, cmd.Execute())
	assert.Empty(t, gotProvider)
}
