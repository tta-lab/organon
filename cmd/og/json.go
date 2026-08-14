package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

// JSON result shapes mirror the og MCP server's structured outputs so the CLI
// and the Pi extension share one domain representation.

type ogAuthJSON struct {
	Project string        `json:"project,omitempty"`
	Auth    og.AuthStatus `json:"auth"`
}

type ogPRJSON struct {
	Project string          `json:"project,omitempty"`
	PR      og.PullRequest  `json:"pr"`
}

type ogPRLinesJSON struct {
	Project string         `json:"project,omitempty"`
	PR      og.PullRequest `json:"pr"`
	Lines   []string       `json:"lines"`
}

type ogCommentJSON struct {
	Project string     `json:"project,omitempty"`
	Comment og.Comment `json:"comment"`
}

type ogMessageJSON struct {
	Project string `json:"project,omitempty"`
	Message string `json:"message"`
}

type ogCloneJSON struct {
	Clone og.CloneResult `json:"clone"`
}

// printJSON writes one JSON document to the command's stdout.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// jsonFlag reports whether the --json flag is set.
func jsonFlag(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// resolveDaemonWorkDir returns the work directory for a guarded og call: the
// exact registered project path when --project is set, otherwise the current
// working directory. The alias must resolve exactly through the project store;
// callers can never select a worktree path, root, URI, or credential.
func resolveDaemonWorkDir(cmd *cobra.Command) (workDir, alias string, err error) {
	alias, _ = cmd.Flags().GetString("project")
	if alias == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("get working directory: %w", err)
		}
		return workDir, "", nil
	}
	entry, err := project.NewStore(config.ProjectsPath()).Get(alias)
	if err != nil {
		return "", "", fmt.Errorf("resolve project: %w", err)
	}
	return entry.Path, alias, nil
}
