package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runGitClone(cmd *cobra.Command, args []string) error {
	alias, _ := cmd.Flags().GetString("alias")
	reference, _ := cmd.Flags().GetBool("reference")
	if reference && alias != "" {
		return fmt.Errorf("--alias cannot be used with --reference")
	}
	selector := args[0]
	req := og.Request{Alias: alias, Reference: reference}
	isURL := strings.HasPrefix(selector, "http://") || strings.HasPrefix(selector, "https://")
	if isURL {
		req.URL = selector
	} else {
		if alias != "" || reference {
			return fmt.Errorf("project clone does not accept --alias or --reference")
		}
		req.Project = selector
	}
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	resp, err := runtime.executor.GitClone(requestFor(cmd, req))
	if err != nil {
		return err
	}
	if err := og.ValidateCloneResponse(resp); err != nil {
		return err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(ogCloneJSON{Clone: *resp.Clone})
	}
	if resp.Clone.Registered {
		cmd.Printf("Cloned %s to %s\n", resp.Clone.Alias, resp.Clone.Path)
	} else {
		cmd.Printf("Cloned reference to %s\n", resp.Clone.Path)
	}
	return nil
}

func runGitPush(cmd *cobra.Command, args []string) error {
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	workDir, alias, err := resolveWorkDir(cmd, runtime)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	resp, err := runtime.executor.GitPush(requestFor(cmd, og.Request{WorkDir: workDir, Force: force}))
	if err != nil {
		return err
	}
	if err := og.ValidateMessageResponse(resp); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogMessageJSON{Project: alias, Message: resp.Message})
	}
	printProjectResponse(cmd, alias, resp)
	return nil
}

func runGitPull(cmd *cobra.Command, args []string) error {
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	workDir, alias, err := resolveWorkDir(cmd, runtime)
	if err != nil {
		return err
	}
	resp, err := runtime.executor.GitPull(requestFor(cmd, og.Request{WorkDir: workDir}))
	if err != nil {
		return err
	}
	if err := og.ValidateMessageResponse(resp); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogMessageJSON{Project: alias, Message: resp.Message})
	}
	printProjectResponse(cmd, alias, resp)
	return nil
}

func runGitTag(cmd *cobra.Command, args []string) error {
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	workDir, alias, err := resolveWorkDir(cmd, runtime)
	if err != nil {
		return err
	}
	bump, _ := cmd.Flags().GetString("bump")
	if bump != "" && len(args) > 0 {
		return fmt.Errorf("--bump and a positional version are mutually exclusive")
	}
	tag := ""
	if len(args) > 0 {
		tag = args[0]
	}
	resp, err := runtime.executor.GitTag(requestFor(cmd, og.Request{WorkDir: workDir, Tag: tag, Bump: bump}))
	if err != nil {
		return err
	}
	if err := og.ValidateMessageResponse(resp); err != nil {
		return err
	}
	printProjectResponse(cmd, alias, resp)
	return nil
}
