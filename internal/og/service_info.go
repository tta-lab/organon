package og

import "fmt"

func (s Service) AuthStatus(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	status := "unset"
	if ctx.Token != "" {
		status = "set"
	}
	return success(Response{Message: fmt.Sprintf(
		"provider: %s\nhost: %s\nrepo: %s/%s\nproject: %s\ntoken_env: %s (%s)",
		ctx.Provider, ctx.Host, ctx.Owner, ctx.Repo, ctx.ProjectAlias, ctx.TokenEnv, status,
	)}), nil
}

func (s Service) PolicyExplain(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf(
		"repo: %s/%s\nworkdir: %s\nregistered_project: true\n"+
			"protected_branch: %t\narbitrary_git_args: false\narbitrary_api_paths: false",
		ctx.Owner, ctx.Repo, ctx.WorkDir, ctx.Branch == branchMain || ctx.Branch == branchMaster,
	)}), nil
}
