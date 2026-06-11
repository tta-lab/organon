package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/tta-lab/organon/internal/skill"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	paths, err := resolvePaths(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	cmd := newRootCmd(os.Stdout, os.Stderr, paths)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(out, errOut io.Writer, paths []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill [command]",
		Short: "Discover and read agent skills from the filesystem",
		Long:  helpRoot,
	}

	cmd.AddCommand(newListCmd(out, errOut, paths))
	cmd.AddCommand(newGetCmd(out, paths))
	cmd.AddCommand(newFindCmd(out, errOut, paths))

	return cmd
}

func resolvePaths(home string) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	return skill.DiscoveryPaths(cwd, home), nil
}

func newListCmd(out, errOut io.Writer, paths []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all discovered skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := skill.ListSkills(paths)
			if err != nil {
				return err
			}
			return emitSkills(out, errOut, skills, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON array to stdout")
	return cmd
}

func newGetCmd(out io.Writer, paths []string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Print skill content to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := skill.GetSkill(paths, args[0])
			if err != nil {
				return fmt.Errorf("skill %q not found", args[0])
			}
			_, _ = fmt.Fprintln(out, s.Body)
			return nil
		},
	}
}

func newFindCmd(out, errOut io.Writer, paths []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "find <keyword>...",
		Short: "Find skills by keyword (OR match)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := skill.FindSkills(paths, args)
			if err != nil {
				return err
			}
			return emitSkills(out, errOut, skills, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit JSON array to stdout")
	return cmd
}

// skillJSON is the JSON-serializable shape emitted by --json output.
// Path and Body are intentionally excluded; only user-facing metadata is surfaced.
type skillJSON struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// skillJSONFromSkill converts a skill.Skill to skillJSON, excluding internal fields.
func skillJSONFromSkill(s skill.Skill) skillJSON {
	return skillJSON{
		Name:        s.Name,
		Category:    s.Category,
		Source:      s.Source,
		Description: s.Description,
	}
}

// emitSkills writes skills to out in JSON or bullet format.
// For JSON output, encoding errors are returned as errors so the caller
// (cobra) can exit non-zero. For bullet output, nothing is returned.
func emitSkills(out, errOut io.Writer, skills []skill.Skill, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(out)
		outSkills := make([]skillJSON, len(skills))
		for i, s := range skills {
			outSkills[i] = skillJSONFromSkill(s)
		}
		return enc.Encode(outSkills)
	}
	if len(skills) == 0 {
		_, _ = fmt.Fprintln(errOut, "No skills found.")
		return nil
	}
	printSkillBullets(out, skills)
	return nil
}

func printSkillBullets(out io.Writer, skills []skill.Skill) {
	_, _ = fmt.Fprintln(out, "Available skills:")
	for _, s := range skills {
		fmt.Fprintf(out, "- %s: %s (file: %s)\n", s.Name, s.Description, s.Path)
	}
}
