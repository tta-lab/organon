package og

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

func (s Service) GitPush(req Request) (Response, error) {
	ctx, err := s.resolveRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctx, "push"); err != nil {
		return Response{}, err
	}
	if req.Force && !ctx.DefaultBaseKnown {
		return Response{}, fmt.Errorf("refusing to force push because the default branch is unknown")
	}
	if req.Force && ctx.Branch == ctx.DefaultBase {
		return Response{}, fmt.Errorf("refusing to force push default branch %q", ctx.Branch)
	}
	gitArgs := []string{"push", "-u", remoteOrigin, ctx.Branch}
	if req.Force {
		gitArgs = append(gitArgs, "--force-with-lease")
	}
	if err := runGitWithCreds(ctx, githubapp.PurposeGitWrite, gitArgs...); err != nil {
		return Response{}, err
	}
	return success(Response{Message: fmt.Sprintf("Pushed %s -> origin/%s", ctx.Branch, ctx.Branch)}), nil
}

func (s Service) GitPull(req Request) (Response, error) {
	ctx, err := s.resolveRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if ctx.Archived {
		return pullArchived(ctx)
	}
	if ctx.Provider == gitprovider.ProviderGeneric {
		return pullNamedBranch(ctx, ctx.Branch)
	}
	return pullActive(ctx)
}

func pullArchived(ctx *repoContext) (Response, error) {
	if !ctx.DefaultBaseKnown {
		return Response{}, fmt.Errorf("refusing archived pull because the default branch is unknown")
	}
	if ctx.Branch != ctx.DefaultBase {
		return Response{}, fmt.Errorf(
			"refusing archived pull outside the default branch %q", ctx.DefaultBase,
		)
	}
	return pullNamedBranch(ctx, ctx.DefaultBase)
}

func pullNamedBranch(ctx *repoContext, branch string) (Response, error) {
	if err := runGitWithCreds(
		ctx, githubapp.PurposeGitRead, "pull", "--ff-only", remoteOrigin, branch,
	); err != nil {
		return Response{}, err
	}
	return success(Response{Message: "Pulled " + branch}), nil
}

func pullActive(ctx *repoContext) (Response, error) {
	if ctx.Branch == ctx.DefaultBase {
		return pullNamedBranch(ctx, ctx.DefaultBase)
	}
	if err := validateCurrentRemoteTargets(ctx, false); err != nil {
		return Response{}, err
	}

	pr, err := findPR(ctx, stateAll)
	if err != nil && !isNoPRFound(err) && !isAnonymousGitHubReadScopeError(ctx, err) {
		return Response{}, err
	}
	if err == nil && pr.State == "closed" {
		if err := cleanupClosedPRBranch(ctx, pr.Merged); err != nil {
			return Response{}, err
		}
		prOutcome := "Closed PR"
		if pr.Merged {
			prOutcome = "Merged PR"
		}
		return success(Response{
			Message: fmt.Sprintf(
				"%s. Pulled %s. Deleted %s locally and remotely",
				prOutcome, ctx.DefaultBase, ctx.Branch,
			),
		}), nil
	}

	return pullNamedBranch(ctx, ctx.Branch)
}

func isAnonymousGitHubReadScopeError(ctx *repoContext, err error) bool {
	return ctx.Provider == gitprovider.ProviderGitHub &&
		(errors.Is(err, githubapp.ErrOwnerNotAllowed) || errors.Is(err, githubapp.ErrInstallationNotFound))
}

func (s Service) GitTag(req Request) (Response, error) {
	ctx, err := s.resolveRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if err := requireRemoteWrite(ctx, "tag"); err != nil {
		return Response{}, err
	}
	if err := requireGitPushTarget(ctx, "tag"); err != nil {
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
	if !localTagExists(operationContext(ctx), ctx.WorkDir, tag) {
		if err := runGit(operationContext(ctx), ctx.WorkDir, "tag", "--", tag); err != nil {
			return Response{}, err
		}
	}
	if err := runGitWithCreds(ctx, githubapp.PurposeGitWrite, "push", remoteOrigin, "--", tag); err != nil {
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
