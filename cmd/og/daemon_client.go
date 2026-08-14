package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runAuthStatus(cmd *cobra.Command, args []string) error {
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	resp, err := daemonCall("/auth/status", og.Request{WorkDir: workDir})
	if err != nil {
		return err
	}
	if resp.Auth == nil {
		return fmt.Errorf("og daemon returned no authentication status")
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogAuthJSON{Project: alias, Auth: *resp.Auth})
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRDaemonWithOutput(cmd *cobra.Command, path string, req og.Request) error {
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if resp.PR != nil {
		if jsonFlag(cmd) {
			return printJSON(cmd, ogPRJSON{Project: alias, PR: *resp.PR})
		}
		return printPR(cmd, resp.PR)
	}
	if jsonFlag(cmd) {
		return fmt.Errorf("og daemon returned no pull request")
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runLinesDaemon(cmd *cobra.Command, path string, req og.Request) error {
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if jsonFlag(cmd) {
		if resp.PR == nil {
			return fmt.Errorf("og daemon returned no pull request")
		}
		return printJSON(cmd, ogPRLinesJSON{Project: alias, PR: *resp.PR, Lines: resp.Lines})
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
	if jsonFlag(cmd) {
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
