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
	cmd.AddCommand(newRunnableCmd("comment", "Comment on a pull request", runPRComment))
	cmd.AddCommand(newRunnableCmd("checks", "Show pull request checks", runPRChecks))
	cmd.AddCommand(newRunnableCmd(cmdStatus, "Show pull request status", runPRChecks))
	cmd.AddCommand(newPRFailuresCmd("failures"))
	cmd.AddCommand(newPRLogCmd())
	return cmd
}

func newPRCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <title>",
		Short: "Create a pull request",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runPRCreate,
	}
}

func newPRViewCmd(use string) *cobra.Command {
	cmd := newRunnableCmd(use, "View or find a pull request", runPRView)
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newPRFindCmd() *cobra.Command {
	cmd := newRunnableCmd("find", "Find a pull request for the current branch", runPRFind)
	cmd.Flags().String("state", "open", "PR state to search: open, closed, or all")
	return cmd
}

func newPRGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <index>",
		Short: "Get a pull request by index",
		Args:  cobra.ExactArgs(1),
		RunE:  runPRGet,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newPRModifyCmd() *cobra.Command {
	cmd := newRunnableCmd("modify", "Modify a pull request", runPRModify)
	cmd.Flags().String("title", "", "New PR title")
	cmd.Flags().String("pr-id", "", "PR number override")
	return cmd
}

func newPRLogCmd() *cobra.Command {
	return newPRFailuresCmd("log")
}

func newPRFailuresCmd(use string) *cobra.Command {
	cmd := newRunnableCmd(use, "Show CI failure logs for the current PR", runPRFailures)
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
	cmd.AddCommand(newRunnableCmd("pull", "Pull from the tracked branch", runGitPull))
	cmd.AddCommand(newGitTagCmd())
	return cmd
}

func newGitPushCmd() *cobra.Command {
	cmd := newRunnableCmd("push", "Push the current branch", runGitPush)
	cmd.Flags().Bool("force", false, "Force push with --force-with-lease")
	return cmd
}

func newGitTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag [<version> | --bump <major|minor|patch>]",
		Short: "Create and push a tag",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runGitTag,
	}
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
	cmd.AddCommand(newRunnableCmd(cmdStatus, "Show authentication status", runAuthStatus))
	return cmd
}

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Explain repository policy decisions",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}
	cmd.AddCommand(newRunnableCmd("explain", "Explain policy for the current repository", runPolicyExplain))
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
	cmd.AddCommand(newRunnableCmd("run", "Run the daemon in the foreground", runDaemonRun))
	cmd.AddCommand(newRunnableCmd("install", "Install the daemon user service", runDaemonInstall))
	cmd.AddCommand(newRunnableCmd("uninstall", "Remove the daemon user service", runDaemonUninstall))
	cmd.AddCommand(newRunnableCmd("start", "Start the daemon user service", runDaemonStart))
	cmd.AddCommand(newRunnableCmd("stop", "Stop the daemon user service", runDaemonStop))
	cmd.AddCommand(newRunnableCmd("restart", "Restart the daemon user service", runDaemonRestart))
	cmd.AddCommand(newRunnableCmd(cmdStatus, "Show daemon status", runDaemonStatus))
	cmd.AddCommand(newRunnableCmd("health", "Show daemon health", runDaemonHealth))
	return cmd
}

func newRunnableCmd(use, short string, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  run,
	}
}

func showHelp(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
