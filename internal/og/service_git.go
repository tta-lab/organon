package og

import (
	"fmt"
	"strings"
)

func (s Service) GitPush(req Request) (Response, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
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
	if err != nil && !isNoPRFound(err) {
		return Response{}, err
	}
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
		tag, err = computeBumpedTag(ctx, req.Bump)
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

func isNoPRFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return (strings.HasPrefix(msg, "no ") && strings.Contains(msg, " pr found")) ||
		strings.Contains(msg, "no pr found") ||
		strings.Contains(msg, "no pull request found") ||
		strings.Contains(msg, "pull request not found")
}
