package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tta-lab/organon/internal/og"
)

type ogIssuesJSON struct {
	Project    string     `json:"project,omitempty"`
	Issues     []og.Issue `json:"issues"`
	HasNext    bool       `json:"has_next"`
	Incomplete bool       `json:"incomplete"`
}
type ogIssueJSON struct {
	Project string   `json:"project,omitempty"`
	Issue   og.Issue `json:"issue"`
}
type ogCommentsJSON struct {
	Project  string            `json:"project,omitempty"`
	Comments []og.IssueComment `json:"comments"`
	HasNext  bool              `json:"has_next"`
}

func issuePage(cmd *cobra.Command) (string, int, int) {
	s, _ := cmd.Flags().GetString("state")
	p, _ := cmd.Flags().GetInt("page")
	n, _ := cmd.Flags().GetInt("per-page")
	return s, p, n
}
func validateCLIPage(cmd *cobra.Command) error {
	for _, name := range []string{"page", "per-page"} {
		if cmd.Flags().Changed(name) {
			value, _ := cmd.Flags().GetInt(name)
			if value == 0 {
				return fmt.Errorf("issue %s must be positive", name)
			}
		}
	}
	return nil
}
func runIssueList(cmd *cobra.Command, args []string) error {
	if err := validateCLIPage(cmd); err != nil {
		return err
	}
	s, p, n := issuePage(cmd)
	return runIssues(cmd, og.Request{State: s, Page: p, PerPage: n}, og.Executor.IssueList)
}
func runIssueSearch(cmd *cobra.Command, args []string) error {
	if err := validateCLIPage(cmd); err != nil {
		return err
	}
	s, p, n := issuePage(cmd)
	return runIssues(cmd, og.Request{Query: args[0], State: s, Page: p, PerPage: n}, og.Executor.IssueSearch)
}
func runIssueGet(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid issue index %q: %w", args[0], err)
	}
	return runIssue(cmd, og.Request{Index: id}, og.Executor.IssueGet)
}
func runIssueComments(cmd *cobra.Command, args []string) error {
	if err := validateCLIPage(cmd); err != nil {
		return err
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid issue index %q: %w", args[0], err)
	}
	_, p, n := issuePage(cmd)
	return runIssueCommentsWith(cmd, og.Request{Index: id, Page: p, PerPage: n})
}
func runIssues(cmd *cobra.Command, req og.Request, op func(og.Executor, og.Request) (og.Response, error)) error {
	rt, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	wd, alias, err := resolveWorkDir(cmd, rt)
	if err != nil {
		return err
	}
	req.WorkDir = wd
	resp, err := op(rt.executor, requestFor(cmd, req))
	if err != nil {
		return err
	}
	if jsonFlag(cmd) {
		output := ogIssuesJSON{Project: alias, Issues: resp.Issues, HasNext: resp.HasNext, Incomplete: resp.Incomplete}
		return printJSON(cmd, output)
	}
	for _, i := range resp.Issues {
		cmd.Printf("#%d %s (%s) %s\n", i.Index, i.Title, i.State, i.URL)
	}
	if resp.Incomplete {
		cmd.Println("Results may be incomplete or capped; refine the search.")
	}
	if resp.HasNext {
		cmd.Println("More results may be available; request the next page.")
	}
	return nil
}
func runIssue(cmd *cobra.Command, req og.Request, op func(og.Executor, og.Request) (og.Response, error)) error {
	rt, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	wd, alias, err := resolveWorkDir(cmd, rt)
	if err != nil {
		return err
	}
	req.WorkDir = wd
	resp, err := op(rt.executor, requestFor(cmd, req))
	if err != nil {
		return err
	}
	if resp.Issue == nil {
		return fmt.Errorf("og returned no issue")
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogIssueJSON{Project: alias, Issue: *resp.Issue})
	}
	cmd.Printf("#%d %s\n%s\n%s\n", resp.Issue.Index, resp.Issue.Title, resp.Issue.URL, resp.Issue.Body)
	return nil
}
func runIssueCommentsWith(cmd *cobra.Command, req og.Request) error {
	rt, err := runtimeFor(cmd)
	if err != nil {
		return err
	}
	wd, alias, err := resolveWorkDir(cmd, rt)
	if err != nil {
		return err
	}
	req.WorkDir = wd
	resp, err := rt.executor.IssueComments(requestFor(cmd, req))
	if err != nil {
		return err
	}
	if jsonFlag(cmd) {
		return printJSON(cmd, ogCommentsJSON{Project: alias, Comments: resp.IssueComments, HasNext: resp.HasNext})
	}
	for _, c := range resp.IssueComments {
		cmd.Printf("#%d %s\n%s\n", c.ID, c.URL, c.Body)
	}
	if resp.HasNext {
		cmd.Println("More comments may be available; request the next page.")
	}
	return nil
}
