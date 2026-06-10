package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/format"
	"github.com/tta-lab/organon/internal/org"
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
	cmd.AddCommand(newOrgCmd())

	return cmd
}

// --- list ---

func newListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list [org]",
		Short: "List all projects",
		Long:  helpList,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var orgFilter string
			if len(args) == 1 {
				orgFilter = args[0]
			}
			entries, err := project.ListFiltered(config.ProjectsPath(), orgFilter)
			if err != nil {
				return err
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Println("No projects found.")
				return nil
			}

			dimColor, headerStyle, cellStyle, _ := format.TableStyles()

			rows := make([][]string, len(entries))
			for i, e := range entries {
				rows[i] = []string{e.Alias, project.DeriveOrg(e.Path), e.Name}
			}

			t := table.New().
				Border(lipgloss.RoundedBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(dimColor)).
				StyleFunc(func(row, col int) lipgloss.Style {
					if row == table.HeaderRow {
						return headerStyle
					}
					return cellStyle
				}).
				Headers("ALIAS", "ORG", "NAME").
				Rows(rows...)

			fmt.Println(t)
			fmt.Printf("\n%d projects — use project get <alias> for the path\n", len(entries))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
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
			e, err := project.Resolve(config.ProjectsPath(), alias)
			if err != nil {
				return err
			}
			if e != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(e)
				}
				fmt.Printf("%s\n", e.Path)
				return nil
			}

			// Fall back to reference repos
			repoPath, repoErr := reporef.Resolve(alias, config.DefaultReferencesPath())
			if repoErr != nil {
				return repoErr
			}
			if jsonOut {
				type item struct {
					Alias string `json:"alias"`
					Name  string `json:"name"`
					Path  string `json:"path"`
					Org   string `json:"org"`
				}
				return json.NewEncoder(os.Stdout).Encode(item{Alias: alias, Path: repoPath, Org: reporef.DeriveOrg(repoPath)})
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
		Short: "Resolve a project alias or path to alias, path, org, and GitHub token env",
		Long:  helpResolve,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

			// If it looks like a path (contains /), resolve by path first.
			if strings.Contains(target, "/") {
				e, err := project.GetByPath(config.ProjectsPath(), target)
				if err != nil {
					return err
				}
				if e != nil {
					return json.NewEncoder(os.Stdout).Encode(e)
				}
			}

			// Resolve by alias.
			e, err := project.Resolve(config.ProjectsPath(), target)
			if err != nil {
				return err
			}

			if e != nil {
				return json.NewEncoder(os.Stdout).Encode(e)
			}

			// Fall back to reference repos
			repoPath, repoErr := reporef.Resolve(target, config.DefaultReferencesPath())
			if repoErr != nil {
				return repoErr
			}

			type refResolved struct {
				Alias string `json:"alias"`
				Path  string `json:"path"`
				Org   string `json:"org"`
			}
			return json.NewEncoder(os.Stdout).Encode(refResolved{
				Alias: target,
				Path:  repoPath,
				Org:   reporef.DeriveOrg(repoPath),
			})
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

			// 1. Try project alias
			e, err := project.Resolve(config.ProjectsPath(), target)
			if err != nil {
				return err
			}
			if e != nil {
				fmt.Println(e.Path)
				return nil
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

// --- org ---

func newOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "List and get orgs from orgs.toml",
		Long:  helpOrg,
	}
	cmd.AddCommand(newOrgListCmd())
	cmd.AddCommand(newOrgGetCmd())
	return cmd
}

func newOrgListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all orgs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := org.Load(config.OrgsPath())
			if err != nil {
				return err
			}

			if jsonOut {
				type item struct {
					Org            string `json:"org"`
					GitHubTokenEnv string `json:"github_token_env"`
				}
				out := make([]item, len(entries))
				for i, e := range entries {
					out[i] = item{Org: e.Name, GitHubTokenEnv: e.GitHubTokenEnv}
				}
				return json.NewEncoder(os.Stdout).Encode(out)
			}

			if len(entries) == 0 {
				fmt.Println("No orgs found.")
				return nil
			}

			dimColor, headerStyle, cellStyle, dimStyle := format.TableStyles()

			rows := make([][]string, len(entries))
			for i, e := range entries {
				rows[i] = []string{e.Name, e.GitHubTokenEnv}
			}

			t := table.New().
				Border(lipgloss.RoundedBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(dimColor)).
				StyleFunc(func(row, col int) lipgloss.Style {
					if row == table.HeaderRow {
						return headerStyle
					}
					if col == 1 {
						return dimStyle
					}
					return cellStyle
				}).
				Headers("ORG", "GITHUB_TOKEN_ENV").
				Rows(rows...)

			fmt.Println(t)
			fmt.Printf("\n%d orgs\n", len(entries))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func newOrgGetCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get a single org",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := org.Get(config.OrgsPath(), args[0])
			if err != nil {
				return err
			}
			if e == nil {
				return fmt.Errorf("org %q not found", args[0])
			}

			if jsonOut {
				type item struct {
					Org            string `json:"org"`
					GitHubTokenEnv string `json:"github_token_env"`
				}
				return json.NewEncoder(os.Stdout).Encode(item{Org: e.Name, GitHubTokenEnv: e.GitHubTokenEnv})
			}

			fmt.Printf("org: %s\n", e.Name)
			fmt.Printf("github_token_env: %s\n", e.GitHubTokenEnv)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}
