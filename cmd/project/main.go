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
		Short: "Manage registered projects — list, find, get, resolve, and navigate",
		Long:  helpRoot,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newFindCmd())
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
				if entries == nil {
					entries = []project.Entry{}
				}
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
	printProjectEntries(entries)
}

func printProjectEntries(entries []project.Entry) {
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

// --- find ---

func newFindCmd() *cobra.Command {
	var jsonOut bool
	var limit int
	cmd := &cobra.Command{
		Use:   "find <query>...",
		Short: "Find active projects by relevance",
		Long:  helpFind,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := project.NewStore(config.ProjectsPath()).Find(strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if jsonOut {
				if entries == nil {
					entries = []project.Entry{}
				}
				return json.NewEncoder(os.Stdout).Encode(projectListOutput{Projects: entries})
			}
			if len(entries) == 0 {
				fmt.Println("No active projects found.")
				return nil
			}
			fmt.Println("Matching active projects:")
			printProjectEntries(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().IntVar(
		&limit, "limit", project.DefaultFindLimit,
		"Maximum number of active projects to return (maximum 32)",
	)
	return cmd
}

// --- get ---

func canUseProjectReferenceFallback(target string) bool {
	return strings.TrimSpace(target) != ""
}

func newGetCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get <project-reference>",
		Short: "Get a project by reference (includes references)",
		Long:  helpGet,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			store := project.NewStore(config.ProjectsPath())
			entry, resolveErr := store.Resolve(reference)
			if resolveErr == nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(projectGetOutput{Project: entry})
				}
				fmt.Println(entry.Path)
				return nil
			}
			if jsonOut || !errors.Is(resolveErr, project.ErrNotFound) {
				return resolveErr
			}

			// Human mode keeps the existing local reference-repository fallback.
			if !canUseProjectReferenceFallback(reference) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(reference, config.DefaultReferencesPath())
			if repoErr != nil {
				return resolveErr
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
		Use:   "resolve <project-reference-or-path>",
		Short: "Resolve a project reference or path to project identity and path",
		Long:  helpResolve,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			store := project.NewStore(config.ProjectsPath())

			// Absolute paths are an explicit catalog lookup mode.
			if filepath.IsAbs(target) {
				entry, err := store.GetByPath(target)
				if err != nil {
					return err
				}
				return json.NewEncoder(os.Stdout).Encode(entry)
			}

			entry, resolveErr := store.Resolve(target)
			if resolveErr == nil {
				return json.NewEncoder(os.Stdout).Encode(entry)
			}
			if !errors.Is(resolveErr, project.ErrNotFound) {
				return resolveErr
			}

			// org/repo and valid bare names retain their explicit reference-repo mode.
			if !canUseProjectReferenceFallback(target) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr != nil {
				return resolveErr
			}
			return json.NewEncoder(os.Stdout).Encode(project.Entry{Alias: target, Path: repoPath})
		},
	}
	return cmd
}

// --- jump ---

func newJumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jump <project-reference|org/repo>",
		Short: "Print the filesystem path for a project or reference repo",
		Long:  helpJump,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			store := project.NewStore(config.ProjectsPath())

			entry, resolveErr := store.Resolve(target)
			if resolveErr == nil {
				fmt.Println(entry.Path)
				return nil
			}
			if !errors.Is(resolveErr, project.ErrNotFound) {
				return resolveErr
			}

			// Reference-repository lookup remains a separate explicit mode.
			if !canUseProjectReferenceFallback(target) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr == nil {
				fmt.Println(repoPath)
				return nil
			}
			if strings.Contains(target, "/") {
				return repoErr
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: repo lookup also failed: %v\n", repoErr)
			return resolveErr
		},
	}
	return cmd
}
