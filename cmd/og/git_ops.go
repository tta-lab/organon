package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runGitPush(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	force, _ := cmd.Flags().GetBool("force")
	resp, err := daemonCall("/git/push", og.Request{WorkDir: workDir, Force: force})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runGitPull(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	resp, err := daemonCall("/git/pull", og.Request{WorkDir: workDir})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runGitTag(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	bump, _ := cmd.Flags().GetString("bump")
	if bump != "" && len(args) > 0 {
		return fmt.Errorf("--bump and a positional version are mutually exclusive")
	}
	tag := ""
	if len(args) > 0 {
		tag = args[0]
	}
	resp, err := daemonCall("/git/tag", og.Request{WorkDir: workDir, Tag: tag, Bump: bump})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}
