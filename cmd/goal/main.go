package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/goal"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "goal: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal [command]",
		Short: "Read and mutate the current Lenos session goal file",
		Long:  helpRoot,
	}

	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newAppendCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newStatusCmd())

	return cmd
}

// resolvePath returns the goal path from LENOS_GOAL, or an error.
func resolvePath() (string, error) {
	return goal.GoalPath()
}

// --- add ---

func newAddCmd() *cobra.Command {
	var status string
	var force bool
	cmd := &cobra.Command{
		Use:   "add [--status <status>]",
		Short: "Create a new goal file",
		Long: `Create a new goal file with the body read from stdin.
Defaults to status "draft". Use --status to specify a different initial status.
Fails if the file already exists unless --force is passed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath()
			if err != nil {
				return err
			}

			body, err := readBodyStdin(cmd)
			if err != nil {
				return err
			}
			return goal.Add(path, body, status, force)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Initial status (draft, active, blocked, complete)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing goal file")
	return cmd
}

// --- update ---

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Replace the goal body, preserving status",
		Long:  "Replace the entire body of the goal file with stdin, keeping the current status.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath()
			if err != nil {
				return err
			}

			body, err := readBodyStdin(cmd)
			if err != nil {
				return err
			}
			return goal.Update(path, body)
		},
	}
	return cmd
}

// --- append ---

func newAppendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "append",
		Short: "Append text to the goal body",
		Long:  "Append stdin to the goal body, separated by a blank line. Preserves the current status.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath()
			if err != nil {
				return err
			}

			text, err := readBodyStdin(cmd)
			if err != nil {
				return err
			}
			return goal.Append(path, text)
		},
	}
	return cmd
}

// --- get ---

func newGetCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Print the goal file",
		Long:  "Print the goal file body, or structured JSON with --json.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath()
			if err != nil {
				return err
			}

			f, err := goal.Get(path)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{
					"path":       f.Path,
					"status":     f.Frontmatter.Status,
					"created_at": f.Frontmatter.CreatedAt,
					"updated_at": f.Frontmatter.UpdatedAt,
					"body":       f.Body,
				})
			}

			fmt.Print(f.Body)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

// --- status ---

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <draft|active|blocked|complete>",
		Short: "Update the goal status",
		Long:  "Update only the status frontmatter field. Does not touch the body.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath()
			if err != nil {
				return err
			}

			status := args[0]
			if err := goal.SetStatus(path, status); err != nil {
				return err
			}

			f, err := goal.Get(path)
			if err != nil {
				return err
			}
			fmt.Printf("status: %s\n", f.Frontmatter.Status)
			return nil
		},
	}
	return cmd
}

// readBodyStdin returns body text read from stdin.
func readBodyStdin(cmd *cobra.Command) (string, error) {
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
