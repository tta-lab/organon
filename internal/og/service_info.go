package og

import (
	"context"
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/gitprovider"
)

type requiredGitHubPermission struct {
	name   string
	access string
}

const (
	permissionRead  = "read"
	permissionWrite = "write"
)

var requiredGitHubPermissions = []requiredGitHubPermission{
	{name: "contents", access: permissionWrite},
	{name: "pull_requests", access: permissionWrite},
	{name: "checks", access: permissionRead},
	{name: "actions", access: permissionRead},
	{name: "workflows", access: permissionWrite},
}

func (s Service) AuthStatus(req Request) (Response, error) {
	ctx, err := s.resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return Response{}, err
	}
	if ctx.Provider == gitprovider.ProviderGitHub {
		return s.githubAuthStatus(ctx)
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

func (s Service) githubAuthStatus(ctx *repoContext) (Response, error) {
	if s.githubBroker == nil {
		return Response{}, fmt.Errorf("GitHub App authentication is not configured")
	}
	status, err := s.githubBroker.Status(context.Background(), ctx.Owner, ctx.Repo)
	if err != nil {
		return Response{}, err
	}
	lines := []string{
		"provider: github",
		"host: " + ctx.Host,
		"repo: " + ctx.Owner + "/" + ctx.Repo,
		"project: " + ctx.ProjectAlias,
		"auth: github-app",
		fmt.Sprintf("app_id: %d", status.AppID),
		"installation: ready",
		"repository_scope: " + status.Repository,
		"key_source: file",
	}
	missing := false
	for _, required := range requiredGitHubPermissions {
		ready := permissionSatisfies(status.Permissions[required.name], required.access)
		state := "missing"
		if ready {
			state = "ready"
		} else {
			missing = true
		}
		lines = append(lines, required.name+":"+required.access+": "+state)
	}
	message := strings.Join(lines, "\n")
	if missing {
		return Response{}, fmt.Errorf(
			"%s\ninstallation owner must approve the updated GitHub App permissions",
			message,
		)
	}
	return success(Response{Message: message}), nil
}

func permissionSatisfies(actual, required string) bool {
	return actual == required || (required == permissionRead && actual == permissionWrite)
}
