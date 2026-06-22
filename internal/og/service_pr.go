package og

import (
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func (s Service) PRCreate(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if err := runGitWithCreds(ctx, "push", "-u", remoteOrigin, ctx.Branch); err != nil {
		return Response{}, err
	}
	pr, err := createPR(ctx, req.Title, req.Body)
	if err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("PR #%d created: %s", pr.Index, DisplayPRURL(pr)), PR: pr}), nil
}

func (s Service) PRView(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		return Response{}, err
	}
	full, err := getPR(ctx, pr.Index)
	if err == nil {
		pr = full
	}
	attachCIStatus(ctx, pr)
	return success(Response{PR: pr}), nil
}

func (s Service) PRFind(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, req.State)
	if err != nil {
		return Response{}, err
	}
	return success(Response{PR: pr}), nil
}

func (s Service) PRGet(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := getPR(ctx, req.Index)
	if err != nil {
		return Response{}, err
	}
	return success(Response{PR: pr}), nil
}

func (s Service) PRModify(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	index := req.Index
	if index == 0 {
		pr, err := findPR(ctx, stateAll)
		if err != nil {
			return Response{}, err
		}
		index = pr.Index
	}
	pr, err := updatePR(ctx, index, req.Title, req.Body)
	if err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("PR #%d updated: %s", pr.Index, DisplayPRURL(pr)), PR: pr}), nil
}

func (s Service) PRComment(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		return Response{}, err
	}
	if err := commentPR(ctx, pr.Index, req.Body); err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("Commented on PR #%d", pr.Index)}), nil
}

func (s Service) PRChecks(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		return Response{}, err
	}
	lines, err := getChecks(ctx, pr)
	if err != nil {
		return Response{}, err
	}
	if len(lines) == 0 {
		lines = []string{"No checks found."}
	}
	return success(Response{Lines: lines}), nil
}

func attachCIStatus(ctx *repoContext, pr *PullRequest) {
	if pr == nil || pr.SHA == "" {
		return
	}
	ci, err := getCIStatus(ctx, pr)
	if err != nil {
		pr.CIFetchError = err.Error()
		return
	}
	pr.CI = ci
}

func (s Service) PRFailures(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		return Response{}, err
	}
	lines, err := getCIFailures(ctx, pr, req.Tail)
	if err != nil {
		return Response{}, err
	}
	if len(lines) == 0 {
		lines = []string{"No failing checks found."}
	}
	return success(Response{Lines: lines}), nil
}

func (s Service) PRLog(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		return Response{}, err
	}
	ci, err := getCIStatus(ctx, pr)
	if err != nil {
		return Response{}, err
	}
	lines := formatCIStatusLines(pr.SHA, ci)
	if !hasCIFailures(ci) {
		return success(Response{Lines: lines}), nil
	}
	tail := req.Tail
	if tail < 0 {
		tail = 0
	}
	failures, err := getCIFailureDetails(ctx, pr, tail)
	if err != nil {
		lines = append(lines, "warning: could not fetch failure logs: "+err.Error())
		return success(Response{Lines: lines}), nil
	}
	lines = append(lines, "")
	lines = append(lines, formatPRLogFailureDetails(failures)...)
	return success(Response{Lines: lines}), nil
}

func formatCIStatusLines(sha string, ci *CIStatusResponse) []string {
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}
	lines := []string{"CI Status for " + shortSHA + ": " + formatCIState(ci.State)}
	if len(ci.Statuses) == 0 {
		return append(lines, "  No checks found.")
	}
	for _, status := range ci.Statuses {
		line := "  " + ciStateIcon(status.State) + " " + status.Context
		if status.Description != "" && status.Description != status.State {
			line += " - " + status.Description
		}
		lines = append(lines, line)
	}
	return lines
}

func formatPRLogFailureDetails(failures []*gitprovider.JobFailure) []string {
	if len(failures) == 0 {
		return []string{"No failure details available."}
	}
	lines := []string{"Failure Details:"}
	for _, failure := range failures {
		lines = append(lines, "")
		lines = append(lines, "  Workflow: "+failure.WorkflowName)
		lines = append(lines, "  Job: "+failure.JobName)
		if failure.HTMLURL != "" {
			lines = append(lines, "  URL: "+failure.HTMLURL)
		}
		if failure.LogTail != "" {
			lines = append(lines, "  Log tail:")
			for _, line := range strings.Split(failure.LogTail, "\n") {
				lines = append(lines, "    "+line)
			}
		}
	}
	return lines
}

func hasCIFailures(ci *CIStatusResponse) bool {
	return ci.State == gitprovider.StateFailure || ci.State == gitprovider.StateError
}

func formatCIState(state string) string {
	switch state {
	case gitprovider.StateSuccess:
		return "passed"
	case gitprovider.StateFailure:
		return "failed"
	default:
		return state
	}
}

func ciStateIcon(state string) string {
	switch state {
	case gitprovider.StateSuccess:
		return "ok"
	case gitprovider.StateFailure, gitprovider.StateError:
		return "x"
	case gitprovider.StatePending:
		return "."
	default:
		return "?"
	}
}
