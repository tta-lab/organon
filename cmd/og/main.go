package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd(os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "og: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "og",
		Short: "Run typed repository and forge operations",
		Long:  helpRoot,
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.AddCommand(newPRCmd())
	cmd.AddCommand(newGitCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newPolicyCmd())
	cmd.AddCommand(newDaemonCmd())

	return cmd
}

func newPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Work with pull requests",
		Long:  helpPR,
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("create", "Create a pull request"))
	cmd.AddCommand(newStubCmd("view", "View or find a pull request"))
	cmd.AddCommand(newStubCmd("modify", "Modify a pull request"))
	cmd.AddCommand(newStubCmd("comment", "Comment on a pull request"))
	cmd.AddCommand(newStubCmd("checks", "Show pull request checks"))
	return cmd
}

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Run guarded git operations",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("push", "Push the current branch"))
	cmd.AddCommand(newStubCmd("pull", "Pull from the tracked branch"))
	cmd.AddCommand(newStubCmd("tag", "Create or push tags"))
	return cmd
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect forge authentication",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("status", "Show authentication status"))
	return cmd
}

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Explain repository policy decisions",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("explain", "Explain policy for the current repository"))
	return cmd
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Inspect or run the og daemon",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("status", "Show daemon health"))
	return cmd
}

func newStubCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s is not implemented yet", cmd.CommandPath())
		},
	}
}

func showHelp(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
