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
	cmd.AddCommand(newPRCreateCmd())
	cmd.AddCommand(newPRViewCmd("view"))
	cmd.AddCommand(newPRViewCmd("list"))
	cmd.AddCommand(newPRFindCmd())
	cmd.AddCommand(newPRGetCmd())
	cmd.AddCommand(newPRModifyCmd())
	cmd.AddCommand(newStubCmd("comment", "Comment on a pull request"))
	cmd.AddCommand(newStubCmd("checks", "Show pull request checks"))
	cmd.AddCommand(newStubCmd("status", "Show pull request status"))
	cmd.AddCommand(newPRFailuresCmd("failures"))
	cmd.AddCommand(newPRLogCmd())
	return cmd
}

func newPRCreateCmd() *cobra.Command {
	return newStubCmdWithArgs("create <title>", "Create a pull request", cobra.MinimumNArgs(1))
}

func newPRViewCmd(use string) *cobra.Command {
	cmd := newStubCmd(use, "View or find a pull request")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newPRFindCmd() *cobra.Command {
	cmd := newStubCmd("find", "Find a pull request for the current branch")
	cmd.Flags().String("state", "open", "PR state to search: open, closed, or all")
	return cmd
}

func newPRGetCmd() *cobra.Command {
	cmd := newStubCmdWithArgs("get <index>", "Get a pull request by index", cobra.ExactArgs(1))
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newPRModifyCmd() *cobra.Command {
	cmd := newStubCmd("modify", "Modify a pull request")
	cmd.Flags().String("title", "", "New PR title")
	cmd.Flags().String("pr-id", "", "PR number override")
	return cmd
}

func newPRLogCmd() *cobra.Command {
	return newPRFailuresCmd("log")
}

func newPRFailuresCmd(use string) *cobra.Command {
	cmd := newStubCmd(use, "Show CI failure logs for the current PR")
	cmd.Flags().Int("tail", 50, "Number of log tail lines to fetch")
	return cmd
}

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Run guarded git operations",
		Long:  helpGit,
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newGitPushCmd())
	cmd.AddCommand(newStubCmd("pull", "Pull from the tracked branch"))
	cmd.AddCommand(newGitTagCmd())
	return cmd
}

func newGitPushCmd() *cobra.Command {
	cmd := newStubCmd("push", "Push the current branch")
	cmd.Flags().Bool("force", false, "Force push with --force-with-lease")
	return cmd
}

func newGitTagCmd() *cobra.Command {
	cmd := newStubCmd("tag [<version> | --bump <major|minor|patch>]", "Create and push a tag")
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Flags().String("bump", "", "Bump version: major, minor, or patch")
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
		Long:  helpDaemon,
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newStubCmd("run", "Run the daemon in the foreground"))
	cmd.AddCommand(newStubCmd("install", "Install the daemon user service"))
	cmd.AddCommand(newStubCmd("uninstall", "Remove the daemon user service"))
	cmd.AddCommand(newStubCmd("start", "Start the daemon user service"))
	cmd.AddCommand(newStubCmd("stop", "Stop the daemon user service"))
	cmd.AddCommand(newStubCmd("restart", "Restart the daemon user service"))
	cmd.AddCommand(newStubCmd("status", "Show daemon status"))
	cmd.AddCommand(newStubCmd("health", "Show daemon health"))
	return cmd
}

func newStubCmd(use, short string) *cobra.Command {
	return newStubCmdWithArgs(use, short, cobra.NoArgs)
}

func newStubCmdWithArgs(use, short string, args cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s is not implemented yet", cmd.CommandPath())
		},
	}
}

func showHelp(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
