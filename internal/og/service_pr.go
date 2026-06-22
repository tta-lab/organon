package og

import (
	"fmt"
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
