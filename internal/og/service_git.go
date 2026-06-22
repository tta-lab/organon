package og

import "fmt"

func (s Service) GitPush(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if ctx.Branch == branchMain || ctx.Branch == branchMaster {
		return Response{}, fmt.Errorf("refusing to push protected branch %q", ctx.Branch)
	}
	gitArgs := []string{"push", "-u", remoteOrigin, ctx.Branch}
	if req.Force {
		gitArgs = append(gitArgs, "--force-with-lease")
	}
	if err := runGitWithCreds(ctx, gitArgs...); err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("Pushed %s -> origin/%s", ctx.Branch, ctx.Branch)}), nil
}

func (s Service) GitPull(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if ctx.Branch == ctx.DefaultBase {
		if err := runGitWithCreds(ctx, "pull", "--ff-only", remoteOrigin, ctx.DefaultBase); err != nil {
			return Response{}, err
		}
		return success(Response{Message: "Pulled " + ctx.DefaultBase}), nil
	}

	pr, err := findPR(ctx, stateAll)
	if err == nil && pr.Merged {
		if err := cleanupMergedBranch(ctx); err != nil {
			return Response{}, err
		}
		return success(Response{
			Message: fmt.Sprintf("Pulled %s. Deleted %s locally and remotely", ctx.DefaultBase, ctx.Branch),
		}), nil
	}

	if err := runGitWithCreds(ctx, "pull", "--ff-only", remoteOrigin, ctx.Branch); err != nil {
		return Response{}, err
	}
	return success(Response{Message: "Pulled " + ctx.Branch}), nil
}

func (s Service) GitTag(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if req.Bump != "" && req.Tag != "" {
		return Response{}, fmt.Errorf("--bump and a positional version are mutually exclusive")
	}
	tag := req.Tag
	if req.Bump != "" {
		tag, err = computeBumpedTag(ctx.WorkDir, req.Bump)
		if err != nil {
			return Response{}, err
		}
	}
	if tag == "" {
		return Response{}, fmt.Errorf("either a version argument or --bump is required")
	}
	if !semverTagRe.MatchString(tag) {
		return Response{}, fmt.Errorf("invalid semver tag %q", tag)
	}
	if !localTagExists(ctx.WorkDir, tag) {
		if err := runGit(ctx.WorkDir, "tag", "--", tag); err != nil {
			return Response{}, err
		}
	}
	if err := runGitWithCreds(ctx, "push", remoteOrigin, "--", tag); err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("Tagged %s -> pushed to origin", tag)}), nil
}
