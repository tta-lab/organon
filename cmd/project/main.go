package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/reporef"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage registered projects — list, get, resolve, and navigate",
		Long:  helpRoot,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newResolveCmd())
	cmd.AddCommand(newJumpCmd())
	cmd.AddCommand(newMCPCmd())

	return cmd
}

// --- list ---

func newListCmd() *cobra.Command {
	var jsonOut, includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  helpList,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := project.NewStore(config.ProjectsPath()).List(includeArchived)
			if err != nil {
				return err
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(projectListOutput{Projects: entries})
			}

			if len(entries) == 0 {
				fmt.Println("No projects found.")
				return nil
			}

			printProjectBullets(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived projects")
	return cmd
}

func printProjectBullets(entries []project.Entry) {
	fmt.Println("Available projects:")
	for _, e := range entries {
		if e.Name != "" && e.Path != "" {
			fmt.Printf("- %s: %s (path: %s)\n", e.Alias, e.Name, e.Path)
			continue
		}
		if e.Name != "" {
			fmt.Printf("- %s: %s\n", e.Alias, e.Name)
			continue
		}
		fmt.Printf("- %s: %s\n", e.Alias, e.Path)
	}
}

// --- get ---

func newGetCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get <alias>",
		Short: "Get a project by alias (includes references)",
		Long:  helpGet,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
			store := project.NewStore(config.ProjectsPath())
			if !strings.Contains(alias, "/") {
				e, err := store.Get(alias)
				if err == nil {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(projectGetOutput{Project: e})
					}
					fmt.Printf("%s\n", e.Path)
					return nil
				}
				if !errors.Is(err, project.ErrNotFound) {
					return err
				}
			}

			// Fall back to reference repos
			repoPath, repoErr := reporef.Resolve(alias, config.DefaultReferencesPath())
			if repoErr != nil {
				return repoErr
			}
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(projectGetOutput{Project: project.Entry{Alias: alias, Path: repoPath}})
			}
			fmt.Println(repoPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

// --- resolve ---

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <alias-or-path>",
		Short: "Resolve a project alias or path to project identity and path",
		Long:  helpResolve,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			store := project.NewStore(config.ProjectsPath())

			// Absolute paths are catalog lookups; org/repo targets go directly
			// to reference resolution instead of alias validation.
			if filepath.IsAbs(target) {
				e, err := store.GetByPath(target)
				if err == nil {
					return json.NewEncoder(os.Stdout).Encode(e)
				}
				return err
			}

			if !strings.Contains(target, "/") {
				e, err := store.Get(target)
				if err == nil {
					return json.NewEncoder(os.Stdout).Encode(e)
				}
				if !errors.Is(err, project.ErrNotFound) {
					return err
				}
			}

			// Fall back to reference repos
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr != nil {
				return repoErr
			}

			return json.NewEncoder(os.Stdout).Encode(project.Entry{Alias: target, Path: repoPath})
		},
	}
	return cmd
}

// --- jump ---

func newJumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jump <alias|org/repo>",
		Short: "Print the filesystem path for a project or reference repo",
		Long:  helpJump,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			store := project.NewStore(config.ProjectsPath())

			// 1. Try a bare project alias. org/repo targets go directly to
			// reference resolution so strict alias validation cannot reject them.
			if !strings.Contains(target, "/") {
				e, err := store.Get(target)
				if err == nil {
					fmt.Println(e.Path)
					return nil
				}
				if !errors.Is(err, project.ErrNotFound) {
					return err
				}
			}

			// 2. Try reference repo
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr == nil {
				fmt.Println(repoPath)
				return nil
			}

			// Surface repo lookup failure
			if strings.Contains(target, "/") {
				return repoErr
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: repo lookup also failed: %v\n", repoErr)
			return fmt.Errorf("project %q not found", target)
		},
	}
	return cmd
}
