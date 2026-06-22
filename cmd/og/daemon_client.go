package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runAuthStatus(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	resp, err := daemonCall("/auth/status", og.Request{WorkDir: workDir})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRDaemonWithOutput(cmd *cobra.Command, path string, req og.Request) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if resp.PR != nil {
		return printPR(cmd, resp.PR)
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runLinesDaemon(cmd *cobra.Command, path string, req og.Request) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if len(resp.Lines) == 0 {
		printDaemonResponse(cmd, resp)
		return nil
	}
	for _, line := range resp.Lines {
		cmd.Println(line)
	}
	return nil
}

func daemonCall(path string, req og.Request) (og.Response, error) {
	return og.NewClientFromEnv().Call(path, req)
}

func printDaemonResponse(cmd *cobra.Command, resp og.Response) {
	if resp.Message != "" {
		cmd.Println(resp.Message)
		return
	}
	if resp.PR != nil {
		_ = printPR(cmd, resp.PR)
		return
	}
	for _, line := range resp.Lines {
		cmd.Println(line)
	}
}

func printPR(cmd *cobra.Command, pr *og.PullRequest) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	}
	cmd.Printf("PR #%d  %s  [%s]\n", pr.Index, pr.Title, pr.State)
	if og.DisplayPRURL(pr) != "" {
		cmd.Printf("  %s\n", og.DisplayPRURL(pr))
	}
	if pr.Head != "" || pr.Base != "" {
		cmd.Printf("  %s -> %s\n", pr.Head, pr.Base)
	}
	return nil
}
