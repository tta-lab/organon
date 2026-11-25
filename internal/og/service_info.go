package og

import (
	"fmt"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func (s Service) AuthStatus(req Request) (Response, error) {
	ctx, err := s.resolveRemoteRepoContextForRequest(req)
	if err != nil {
		return Response{}, err
	}
	if ctx.Provider == gitprovider.ProviderGitHub {
		return s.githubAuthStatus(ctx)
	}
	if ctx.Provider == gitprovider.ProviderGeneric {
		return success(Response{
			Auth: &AuthStatus{
				Project: ctx.ProjectAlias, Provider: string(ctx.Provider), Host: ctx.Host,
				Owner: ctx.Owner, Repo: ctx.Repo, AuthMode: "anonymous", Ready: true,
			},
			Message: fmt.Sprintf(
				"provider: generic\nhost: %s\nrepo: %s/%s\nproject: %s\nauth: anonymous\nremote: read-only",
				ctx.Host, ctx.Owner, ctx.Repo, ctx.ProjectAlias,
			),
		}), nil
	}
	status := "unset"
	if ctx.Token != "" {
		status = "set"
	}
	auth := &AuthStatus{
		Project: ctx.ProjectAlias, Provider: string(ctx.Provider), Host: ctx.Host,
		Owner: ctx.Owner, Repo: ctx.Repo, AuthMode: "token", Ready: ctx.Token != "",
		TokenEnv: ctx.TokenEnv, TokenSet: ctx.Token != "",
	}
	return success(Response{Auth: auth, Message: fmt.Sprintf(
		"provider: %s\nhost: %s\nrepo: %s/%s\nproject: %s\ntoken_env: %s (%s)",
		ctx.Provider, ctx.Host, ctx.Owner, ctx.Repo, ctx.ProjectAlias, ctx.TokenEnv, status,
	)}), nil
}

func (s Service) githubAuthStatus(ctx *repoContext) (Response, error) {
	return success(Response{Auth: &AuthStatus{Project: ctx.ProjectAlias, Provider: string(ctx.Provider), Host: ctx.Host,
		Owner: ctx.Owner, Repo: ctx.Repo, AuthMode: "git-credential"},
		Message: fmt.Sprintf("provider: github\nhost: %s\nrepo: %s/%s\nproject: %s\nauth: git-credential",
			ctx.Host, ctx.Owner, ctx.Repo, ctx.ProjectAlias)}), nil
}
