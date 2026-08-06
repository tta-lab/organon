package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

const stateAll = "all"

func runPRCreate(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	body, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	title := strings.Join(args, " ")
	bodyText := strings.TrimRight(string(body), "\n")
	resp, err := daemonCall("/pr/create", og.Request{
		WorkDir: workDir,
		Title:   &title,
		Body:    &bodyText,
	})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRView(cmd *cobra.Command, args []string) error {
	return runPRDaemonWithOutput(cmd, "/pr/view", og.Request{State: stateAll})
}

func runPRFind(cmd *cobra.Command, args []string) error {
	state, _ := cmd.Flags().GetString("state")
	return runPRDaemonWithOutput(cmd, "/pr/find", og.Request{State: state})
}

func runPRGet(cmd *cobra.Command, args []string) error {
	index, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PR index %q: %w", args[0], err)
	}
	return runPRDaemonWithOutput(cmd, "/pr/get", og.Request{Index: index})
}

func runPRModify(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	title, _ := cmd.Flags().GetString("title")
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	body := strings.TrimRight(string(bodyBytes), "\n")
	if title == "" && body == "" {
		return fmt.Errorf("nothing to update: provide --title and/or pipe body via stdin")
	}
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	var titleInput, bodyInput *string
	if cmd.Flags().Changed("title") {
		titleInput = &title
	}
	if body != "" {
		bodyInput = &body
	}
	resp, err := daemonCall("/pr/modify", og.Request{
		WorkDir: workDir, Index: index, Title: titleInput, Body: bodyInput,
	})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRComment(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR comment: %w", err)
	}
	body := strings.TrimSpace(string(bodyBytes))
	if body == "" {
		return fmt.Errorf("comment body is required on stdin")
	}
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	resp, err := daemonCall("/pr/comment", og.Request{WorkDir: workDir, Index: index, Body: &body})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRChecks(cmd *cobra.Command, args []string) error {
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	return runLinesDaemon(cmd, "/pr/checks", og.Request{Index: index, State: stateAll})
}

func runPRLog(cmd *cobra.Command, args []string) error {
	tail, _ := cmd.Flags().GetInt("tail")
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	return runLinesDaemon(cmd, "/pr/log", og.Request{Index: index, State: stateAll, Tail: tail})
}

func runPRFailures(cmd *cobra.Command, args []string) error {
	tail, _ := cmd.Flags().GetInt("tail")
	index, err := optionalPRID(cmd)
	if err != nil {
		return err
	}
	return runLinesDaemon(cmd, "/pr/failures", og.Request{Index: index, State: stateAll, Tail: tail})
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
	if index <= 0 {
		return 0, fmt.Errorf("--pr-id must be positive")
	}
	return index, nil
}
