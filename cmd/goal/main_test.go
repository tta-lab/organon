package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tta-lab/organon/internal/goal"
)

func executeGoalCmd(t *testing.T, stdin string, args ...string) error {
	t.Helper()

	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	return cmd.Execute()
}

func TestAddReadsBodyFromStdinWhenNoTextArgument(t *testing.T) {
	path := t.TempDir() + "/goal.md"
	t.Setenv("LENOS_GOAL", path)

	err := executeGoalCmd(t, "# Goal\n\nFrom stdin\n", "add")
	require.NoError(t, err)

	f, err := goal.Get(path)
	require.NoError(t, err)
	assert.Equal(t, goal.StatusDraft, f.Frontmatter.Status)
	assert.Equal(t, "# Goal\n\nFrom stdin\n", f.Body)
}

func TestUpdateReadsBodyFromStdinWhenNoTextArgument(t *testing.T) {
	path := t.TempDir() + "/goal.md"
	t.Setenv("LENOS_GOAL", path)
	require.NoError(t, goal.Add(path, "old", goal.StatusActive, false))

	err := executeGoalCmd(t, "new\nbody\n", "update")
	require.NoError(t, err)

	f, err := goal.Get(path)
	require.NoError(t, err)
	assert.Equal(t, goal.StatusActive, f.Frontmatter.Status)
	assert.Equal(t, "new\nbody\n", f.Body)
}

func TestAppendReadsBodyFromStdin(t *testing.T) {
	path := t.TempDir() + "/goal.md"
	t.Setenv("LENOS_GOAL", path)
	require.NoError(t, goal.Add(path, "old", goal.StatusActive, false))

	err := executeGoalCmd(t, "added\nbody\n", "append")
	require.NoError(t, err)

	f, err := goal.Get(path)
	require.NoError(t, err)
	assert.Equal(t, "old\n\nadded\nbody\n", f.Body)
}

func TestBodyCommandsRejectPositionalText(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "add", args: []string{"add", "inline"}},
		{name: "update", args: []string{"update", "inline"}},
		{name: "append", args: []string{"append", "inline"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/goal.md"
			t.Setenv("LENOS_GOAL", path)
			if tc.name != "add" {
				require.NoError(t, goal.Add(path, "old", goal.StatusActive, false))
			}

			err := executeGoalCmd(t, "", tc.args...)
			require.Error(t, err)
		})
	}
}
