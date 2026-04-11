package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tta-lab/organon/internal/skill"
)

func main() {
	cmd := newRootCmd(os.Stdout, os.Stderr)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(out, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Skill discovery CLI — list, get, and find skills",
		Long: `Discover and read skills from agent skill directories.

Skills are directories containing a SKILL.md file with YAML frontmatter.
Discovery walks multiple paths in priority order (project-local first, then global).`,
	}

	cmd.AddCommand(newListCmd(out, errOut))
	cmd.AddCommand(newGetCmd(out))
	cmd.AddCommand(newFindCmd(out, errOut))

	return cmd
}

func resolvePaths() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	home := os.Getenv("HOME")
	if home == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}
	return skill.DiscoveryPaths(cwd, home), nil
}

func newListCmd(out, errOut io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all discovered skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths()
			if err != nil {
				return err
			}
			skills, err := skill.ListSkills(paths)
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				_, _ = fmt.Fprintln(errOut, "No skills found.")
				return nil
			}
			printSkillTable(out, skills)
			return nil
		},
	}
}

func newGetCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Print skill content to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths()
			if err != nil {
				return err
			}
			s, err := skill.GetSkill(paths, args[0])
			if err != nil {
				return fmt.Errorf("skill %q not found", args[0])
			}
			_, _ = fmt.Fprintln(out, s.Body)
			return nil
		},
	}
}

func newFindCmd(out, errOut io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "find <keyword>...",
		Short: "Find skills by keyword (OR match)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths()
			if err != nil {
				return err
			}
			skills, err := skill.FindSkills(paths, args)
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				_, _ = fmt.Fprintln(errOut, "No skills found.")
				return nil
			}
			printSkillTable(out, skills)
			return nil
		},
	}
}

func printSkillTable(out io.Writer, skills []skill.Skill) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCATEGORY\tSOURCE\tDESCRIPTION")
	for _, s := range skills {
		category := s.Category
		if category == "" {
			category = "-"
		}
		source := s.Source
		home := os.Getenv("HOME")
		if home != "" && strings.HasPrefix(source, home) {
			source = "~" + strings.TrimPrefix(source, home)
		}
		desc := s.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, category, source, desc)
	}
	_ = tw.Flush()
}
