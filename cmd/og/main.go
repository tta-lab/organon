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
	cmd.AddCommand(newGitPushCmd())
	cmd.AddCommand(newGitPullCmd())
	cmd.AddCommand(newGitTagCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newDaemonCmd())
	cmd.AddCommand(newOGMCPCmd())

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
	cmd.AddCommand(newPRCommentCmd())
	cmd.AddCommand(newPRChecksCmd("checks", "Show pull request checks"))
	cmd.AddCommand(newPRChecksCmd(cmdStatus, "Show pull request status"))
	cmd.AddCommand(newPRFailuresCmd("failures"))
	cmd.AddCommand(newPRLogCmd())
	return cmd
}

func newPRCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <title>",
		Short: "Create a pull request",
		Long:  "Create a pull request. The PR body is read from stdin.",
		Example: `cat <<'EOF' | og pr create "feat(scope): add feature"
## Summary

Describe the change here.
EOF`,
		Args: cobra.MinimumNArgs(1),
		RunE: runPRCreate,
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
	cmd.Long = "Modify a pull request title and/or body. The new body is read from stdin."
	cmd.Example = `cat <<'EOF' | og pr modify --pr-id 123 --title "fix(scope): clearer title"
## Summary

Replace the PR body with this text.
EOF`
	cmd.Flags().String("title", "", "New PR title")
	addOptionalPRIDFlag(cmd)
	return cmd
}

func newPRCommentCmd() *cobra.Command {
	cmd := newRunnableCmd("comment", "Comment on a pull request", runPRComment)
	cmd.Long = "Comment on an explicit pull request or the pull request for the current branch. " +
		"The comment is read from stdin."
	cmd.Example = `cat <<'EOF' | og pr comment --pr-id 123
Tests now pass. Please review again.
EOF`
	addOptionalPRIDFlag(cmd)
	return cmd
}

func newPRChecksCmd(use, short string) *cobra.Command {
	cmd := newRunnableCmd(use, short, runPRChecks)
	addOptionalPRIDFlag(cmd)
	return cmd
}

func newPRLogCmd() *cobra.Command {
	cmd := newRunnableCmd("log", "Show CI status and failure logs for the current PR", runPRLog)
	cmd.Flags().Int("tail", 50, "Number of log tail lines to fetch")
	addOptionalPRIDFlag(cmd)
	return cmd
}

func newPRFailuresCmd(use string) *cobra.Command {
	cmd := newRunnableCmd(use, "Show CI failure logs for the current PR", runPRFailures)
	cmd.Flags().Int("tail", 50, "Number of log tail lines to fetch")
	addOptionalPRIDFlag(cmd)
	return cmd
}

func addOptionalPRIDFlag(cmd *cobra.Command) {
	cmd.Flags().String("pr-id", "", "PR number override")
}

func newGitPushCmd() *cobra.Command {
	cmd := newRunnableCmd("push", "Push the current branch", runGitPush)
	cmd.Flags().Bool("force", false, "Force push with --force-with-lease")
	return cmd
}

func newGitPullCmd() *cobra.Command {
	cmd := newRunnableCmd("pull", "Pull the current branch or clean up a closed PR branch", runGitPull)
	cmd.Long = `Pull the current branch with fast-forward only.

When the current feature branch has a closed PR, og returns to the default
branch and deletes the feature branch locally and remotely. Cleanup refuses a
dirty worktree, unpushed local commits, or a closed-unmerged branch whose remote
ref is already missing.`
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
