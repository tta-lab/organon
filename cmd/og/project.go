package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/reporef"
)

// newProjectCmd owns local registered-project discovery and navigation.
func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage registered projects — list, find, get, resolve, and navigate",
		Long:  helpProject,
	}
	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectFindCmd())
	cmd.AddCommand(newProjectGetCmd())
	cmd.AddCommand(newProjectResolveCmd())
	cmd.AddCommand(newProjectJumpCmd())
	return cmd
}

func discoveryStore() *project.Store {
	return project.NewDiscoveryStore(config.ProjectsPath(), config.DefaultReferencesPath())
}

func newProjectListCmd() *cobra.Command {
	var jsonOut, includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects",
		Long:  helpProjectList,
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
				return json.NewEncoder(cmd.OutOrStdout()).Encode(projectListOutput{Projects: entries})
			}
			if len(entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No projects found.")
				return nil
			}
			printProjectBullets(cmd, entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived projects")
	return cmd
}

func printProjectBullets(cmd *cobra.Command, entries []project.Entry) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Available projects:")
	printProjectEntries(cmd, entries)
}

func printProjectEntries(cmd *cobra.Command, entries []project.Entry) {
	for _, entry := range entries {
		referenceSuffix := ""
		if entry.Reference {
			referenceSuffix = " [reference]"
		}
		if entry.Name != "" && entry.Path != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s%s: %s (path: %s)\n", entry.Alias, referenceSuffix, entry.Name, entry.Path)
			continue
		}
		if entry.Name != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s%s: %s\n", entry.Alias, referenceSuffix, entry.Name)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "- %s%s: %s\n", entry.Alias, referenceSuffix, entry.Path)
	}
}

func newProjectFindCmd() *cobra.Command {
	var jsonOut bool
	var limit int
	cmd := &cobra.Command{
		Use:   "find <query>...",
		Short: "Find projects and references by relevance",
		Long:  helpProjectFind,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := discoveryStore().Find(strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if jsonOut {
				if entries == nil {
					entries = []project.Entry{}
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(projectListOutput{Projects: entries})
			}
			if len(entries) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No active projects or references found.")
				return nil
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Matching projects and references:")
			printProjectEntries(cmd, entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().IntVar(&limit, "limit", project.DefaultFindLimit,
		"Maximum number of projects and references to return (maximum 32)")
	return cmd
}

func canUseProjectReferenceFallback(target string) bool { return strings.TrimSpace(target) != "" }

func newProjectGetCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get <project-reference>",
		Short: "Get a project by reference (includes references)",
		Long:  helpProjectGet,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reference := args[0]
			entry, resolveErr := project.NewStore(config.ProjectsPath()).Resolve(reference)
			if resolveErr == nil {
				if jsonOut {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(projectGetOutput{Project: entry})
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), entry.Path)
				return nil
			}
			if jsonOut || !errors.Is(resolveErr, project.ErrNotFound) {
				return resolveErr
			}
			if !canUseProjectReferenceFallback(reference) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(reference, config.DefaultReferencesPath())
			if repoErr != nil {
				return resolveErr
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), repoPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newProjectResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <project-reference-or-path>",
		Short: "Resolve a project reference or path to project identity and path",
		Long:  helpProjectResolve,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			store := project.NewStore(config.ProjectsPath())
			if filepath.IsAbs(target) {
				entry, err := store.GetByPath(target)
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(entry)
			}
			entry, resolveErr := store.Resolve(target)
			if resolveErr == nil {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(entry)
			}
			if !errors.Is(resolveErr, project.ErrNotFound) || !canUseProjectReferenceFallback(target) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr != nil {
				return resolveErr
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(project.Entry{Alias: target, Path: repoPath})
		},
	}
}

func newProjectJumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "jump <project-reference|org/repo>",
		Short: "Print the filesystem path for a project or reference repo",
		Long:  helpProjectJump,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			entry, resolveErr := project.NewStore(config.ProjectsPath()).Resolve(target)
			if resolveErr == nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), entry.Path)
				return nil
			}
			if !errors.Is(resolveErr, project.ErrNotFound) || !canUseProjectReferenceFallback(target) {
				return resolveErr
			}
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr == nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), repoPath)
				return nil
			}
			if strings.Contains(target, "/") {
				return repoErr
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "note: repo lookup also failed: %v\n", repoErr)
			return resolveErr
		},
	}
}
