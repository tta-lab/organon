package og

import (
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

func (s Service) PRCreate(req Request) (Response, error) {
	title := stringValue(req.Title)
	if strings.TrimSpace(title) == "" {
		return Response{}, fmt.Errorf("PR title must not be blank")
	}
	ctx, err := s.resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctx, "create pull request"); err != nil {
		return Response{}, err
	}
	if err := runGitWithCreds(ctx, githubapp.PurposeGitWrite, "push", "-u", remoteOrigin, ctx.Branch); err != nil {
		return Response{}, err
	}
	pr, err := createPR(ctx, title, stringValue(req.Body))
	if err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("PR #%d created: %s", pr.Index, DisplayPRURL(pr)), PR: pr}), nil
}

func (s Service) PRView(req Request) (Response, error) {
	ctx, err := s.resolveRepoContextFor(req.WorkDir)
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
	ctx, err := s.resolveRepoContextFor(req.WorkDir)
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
	if req.Index <= 0 {
		return Response{}, fmt.Errorf("PR ID must be positive")
	}
	ctx, err := s.resolveRemoteRepoContextFor(req.WorkDir)
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
	if req.Index < 0 {
		return Response{}, fmt.Errorf("PR ID must not be negative")
	}
	if req.Title == nil && req.Body == nil {
		return Response{}, fmt.Errorf("nothing to update: provide title and/or body")
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		return Response{}, fmt.Errorf("PR title must not be blank")
	}
	ctx, err := s.resolvePRContext(req.WorkDir, req.Index)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctx, "modify pull request"); err != nil {
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
	if req.Index < 0 {
		return Response{}, fmt.Errorf("PR ID must not be negative")
	}
	if req.Body == nil || strings.TrimSpace(*req.Body) == "" {
		return Response{}, fmt.Errorf("comment body must not be blank")
	}
	ctx, err := s.resolvePRContext(req.WorkDir, req.Index)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctx, "comment on pull request"); err != nil {
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
	comment, err := commentPR(ctx, index, *req.Body)
	if err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("Commented on PR #%d", index), Comment: comment}), nil
}

func (s Service) PRChecks(req Request) (Response, error) {
	if req.Index < 0 {
		return Response{}, fmt.Errorf("PR ID must not be negative")
	}
	ctx, err := s.resolvePRContext(req.WorkDir, req.Index)
	if err != nil {
		return Response{}, err
	}
	pr, err := getRequestedPR(ctx, req.Index)
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
	return success(Response{PR: pr, Lines: lines}), nil
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
	if req.Index < 0 {
		return Response{}, fmt.Errorf("PR ID must not be negative")
	}
	ctx, err := s.resolvePRContext(req.WorkDir, req.Index)
	if err != nil {
		return Response{}, err
	}
	pr, err := getRequestedPR(ctx, req.Index)
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
	return success(Response{PR: pr, Lines: lines}), nil
}

func (s Service) PRLog(req Request) (Response, error) {
	if req.Index < 0 {
		return Response{}, fmt.Errorf("PR ID must not be negative")
	}
	ctx, err := s.resolvePRContext(req.WorkDir, req.Index)
	if err != nil {
		return Response{}, err
	}
	pr, err := getRequestedPR(ctx, req.Index)
	if err != nil {
		return Response{}, err
	}
	ci, err := getCIStatus(ctx, pr)
	if err != nil {
		return Response{}, err
	}
	lines := formatCIStatusLines(pr.SHA, ci)
	if !hasCIFailures(ci) {
		return success(Response{PR: pr, Lines: lines}), nil
	}
	tail := req.Tail
	if tail < 0 {
		tail = 0
	}
	failures, err := getCIFailureDetails(ctx, pr, tail)
	if err != nil {
		lines = append(lines, "warning: could not fetch failure logs: "+err.Error())
		return success(Response{PR: pr, Lines: lines}), nil
	}
	lines = append(lines, "")
	lines = append(lines, formatPRLogFailureDetails(failures)...)
	return success(Response{PR: pr, Lines: lines}), nil
}

func (s Service) resolvePRContext(workDir string, index int64) (*repoContext, error) {
	if index > 0 {
		return s.resolveRemoteRepoContextFor(workDir)
	}
	return s.resolveRepoContextFor(workDir)
}

func getRequestedPR(ctx *repoContext, index int64) (*PullRequest, error) {
	if index > 0 {
		return getPR(ctx, index)
	}
	return findPR(ctx, stateAll)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
