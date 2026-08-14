package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

func runPRCreate(cmd *cobra.Command, args []string) error {
	body, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	title := strings.Join(args, " ")
	if err := og.ValidatePRTitle(title); err != nil {
		return err
	}
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	bodyText := string(body)
	resp, err := daemonCall("/pr/create", og.Request{
		WorkDir: workDir,
		Title:   &title,
		Body:    &bodyText,
	})
	if err != nil {
		return err
	}
	if err := og.ValidatePRResponse(resp, 0); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogPRJSON{Project: alias, PR: *resp.PR})
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRView(cmd *cobra.Command, args []string) error {
	return runPRDaemonWithOutput(cmd, "/pr/view", og.Request{State: og.PRStateAll})
}

func runPRFind(cmd *cobra.Command, args []string) error {
	state, _ := cmd.Flags().GetString("state")
	state, err := og.NormalizePRState(state)
	if err != nil {
		return err
	}
	return runPRDaemonWithOutput(cmd, "/pr/find", og.Request{State: state})
}

func runPRGet(cmd *cobra.Command, args []string) error {
	index, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PR index %q: %w", args[0], err)
	}
	if err := og.ValidatePositivePRID(index); err != nil {
		return err
	}
	return runPRDaemonWithOutput(cmd, "/pr/get", og.Request{Index: index})
}

func runPRModify(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	clearBody, _ := cmd.Flags().GetBool("clear-body")
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	var titleInput, bodyInput *string
	if cmd.Flags().Changed("title") {
		titleInput = &title
	}
	if clearBody {
		empty := ""
		bodyInput = &empty
	} else if len(bodyBytes) > 0 {
		body := string(bodyBytes)
		bodyInput = &body
	}
	if err := og.ValidatePRModifyInput(titleInput, bodyInput); err != nil {
		return err
	}
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	resp, err := daemonCall("/pr/modify", og.Request{
		WorkDir: workDir, Index: index, Title: titleInput, Body: bodyInput,
	})
	if err != nil {
		return err
	}
	if err := og.ValidatePRModifyResponse(resp, index, titleInput, bodyInput); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogPRJSON{Project: alias, PR: *resp.PR})
	}
	printDaemonResponse(cmd, resp)
	return nil
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
	workDir, alias, err := resolveDaemonWorkDir(cmd)
	if err != nil {
		return err
	}
	resp, err := daemonCall("/pr/comment", og.Request{WorkDir: workDir, Index: index, Body: &body})
	if err != nil {
		return err
	}
	if err := og.ValidateCommentResponse(resp, index, body); err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogCommentJSON{Project: alias, Comment: *resp.Comment})
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRChecks(cmd *cobra.Command, args []string) error {
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	return runLinesDaemon(cmd, "/pr/checks", og.Request{Index: index, State: og.PRStateAll})
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
	return runLinesDaemon(cmd, "/pr/log", og.Request{Index: index, State: og.PRStateAll, Tail: tail})
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
	return runLinesDaemon(cmd, "/pr/failures", og.Request{Index: index, State: og.PRStateAll, Tail: tail})
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
