package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runPRView(cmd *cobra.Command, args []string) error {
	return runPRWithOutput(
		cmd, og.Request{State: og.PRStateAll},
		og.Executor.PRView,
	)
}

func runPRFind(cmd *cobra.Command, args []string) error {
	state, _ := cmd.Flags().GetString("state")
	state, err := og.NormalizePRState(state)
	if err != nil {
		return err
	}
	return runPRWithOutput(cmd, og.Request{State: state}, og.Executor.PRFind)
}

func runPRGet(cmd *cobra.Command, args []string) error {
	index, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PR index %q: %w", args[0], err)
	}
	if err := og.ValidatePositivePRID(index); err != nil {
		return err
	}
	return runPRWithOutput(cmd, og.Request{Index: index}, og.Executor.PRGet)
}

func runPRComment(cmd *cobra.Command, args []string) error {
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR comment: %w", err)
	}
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	body := string(bodyBytes)
	if err := og.ValidatePRCommentBody(&body); err != nil {
		return err
	}
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	workDir, alias, err := resolveWorkDir(cmd, runtime)
	if err != nil {
		return err
	}
	resp, err := runtime.executor.PRComment(requestFor(cmd, og.Request{WorkDir: workDir, Index: index, Body: &body}))
	if err != nil {
		return err
	}
	if err := og.ValidateCommentResponse(resp, index, body); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogCommentJSON{Project: alias, Comment: *resp.Comment})
	}
	printProjectResponse(cmd, alias, resp)
	return nil
}

func runPRChecks(cmd *cobra.Command, args []string) error {
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	return runLines(
		cmd, og.Request{Index: index, State: og.PRStateAll},
		og.Executor.PRChecks,
	)
}

func runPRLog(cmd *cobra.Command, args []string) error {
	tail, _ := cmd.Flags().GetInt("tail")
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	if err := og.ValidatePRLogTail(tail); err != nil {
		return err
	}
	return runLines(
		cmd, og.Request{Index: index, State: og.PRStateAll, Tail: tail},
		og.Executor.PRLog,
	)
}

func runPRFailures(cmd *cobra.Command, args []string) error {
	tail, _ := cmd.Flags().GetInt("tail")
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	if err := og.ValidatePRLogTail(tail); err != nil {
		return err
	}
	return runLines(
		cmd, og.Request{Index: index, State: og.PRStateAll, Tail: tail},
		og.Executor.PRFailures,
	)
}

func runPRMerge(cmd *cobra.Command, args []string) error {
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	actionID, _ := cmd.Flags().GetString("action-id")
	if actionID != "" && actionID != strings.TrimSpace(actionID) {
		return fmt.Errorf("action ID must not contain surrounding whitespace")
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	wait, _ := cmd.Flags().GetBool("wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	req := og.Request{Index: index, ActionID: actionID, DryRun: dryRun, Wait: wait, Timeout: timeout}
	if err := og.ValidatePRMergeRequest(req); err != nil {
		return err
	}
	runtime, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	workDir, alias, err := resolveWorkDir(cmd, runtime)
	if err != nil {
		return err
	}
	req.WorkDir = workDir
	resp, err := runtime.executor.PRMerge(requestFor(cmd, req))
	if err != nil {
		return err
	}
	if err := og.ValidatePRMergeResponse(resp, index); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogPRMergeJSON{Project: alias, Merge: *resp.Merge})
	}
	if alias != "" {
		cmd.Printf("Project %s:\n", alias)
	}
	cmd.Printf("PR #%d merge approval: %s\n", resp.Merge.Snapshot.PRNumber, resp.Merge.Status)
	cmd.Printf("  Action: %s\n", resp.Merge.ActionID)
	if resp.Merge.InboxURL != "" {
		cmd.Printf("  Impri inbox: %s\n", resp.Merge.InboxURL)
	}
	if resp.Merge.Detail != "" {
		cmd.Printf("  %s\n", resp.Merge.Detail)
	}
	return nil
}

func optionalPRID(cmd *cobra.Command) (int64, error) {
	raw, _ := cmd.Flags().GetString("pr-id")
	if raw == "" {
		return 0, nil
	}
	index, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --pr-id %q: %w", raw, err)
	}
	if err := og.ValidatePositivePRID(index); err != nil {
		return 0, err
	}
	return index, nil
}
