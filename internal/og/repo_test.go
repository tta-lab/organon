package og

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

func TestResolveRemoteRepoContextClassifiesGenericHTTPSWithoutForgejoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "must-not-leak")
	repo := testRegisteredRepo(t, home, branchMain, "https://codeberg.org/forgejo/forgejo.git", false)

	ctx, err := NewService(nil).resolveRemoteRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRemoteRepoContextFor: %v", err)
	}
	if ctx.Provider != gitprovider.ProviderGeneric || ctx.Token != "" || ctx.TokenEnv != "" {
		t.Fatalf("context = %+v, want generic provider without token metadata", ctx)
	}
}

func TestResolveRemoteRepoContextIgnoresAmbientURLRewrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, branchMain, "https://attacker.invalid/owner/repo.git", false)
	t.Setenv(
		"GIT_CONFIG_PARAMETERS",
		"'url.https://github.com/.insteadOf=https://attacker.invalid/'",
	)

	ctx, err := NewService(nil).resolveRemoteRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRemoteRepoContextFor: %v", err)
	}
	if ctx.Provider != gitprovider.ProviderGeneric || ctx.RemoteURL != "https://attacker.invalid/owner/repo.git" {
		t.Fatalf("context = %+v, want stored generic origin without ambient URL rewrite", ctx)
	}
}

func TestResolveRemoteRepoContextUsesOnlyAllowedForgejoBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "forge-token")
	repo := testRegisteredRepo(
		t, home, branchMain, "http://forgejo.localhost:17480/GuionAI/flicknote.git", false,
	)
	service := NewServiceWithConfig(nil, project.NewStore(config.ProjectsPath()), ogconfig.Config{
		Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"http://forgejo.localhost:17480"}},
	})

	ctx, err := service.resolveRemoteRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRemoteRepoContextFor: %v", err)
	}
	if ctx.Provider != gitprovider.ProviderForgejo || ctx.Token != "forge-token" {
		t.Fatalf("context = %+v, want allowed Forgejo token routing", ctx)
	}
}

func TestResolveRemoteRepoContextRejectsUnlistedHTTP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, branchMain, "http://forge.example/owner/repo.git", false)

	_, err := NewService(nil).resolveRemoteRepoContextFor(repo)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("resolveRemoteRepoContextFor error = %v, want unlisted HTTP refusal", err)
	}
}

func TestTokenEnvForUsesForgejoConfiguration(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("FORGEJO_ACCESS_TOKEN", "")
	t.Setenv("ORG_GITHUB_TOKEN", "gh-token")
	t.Setenv("GITEA_TOKEN", "forge-token")

	got := tokenEnvFor(gitprovider.ProviderForgejo)
	if got != "GITEA_TOKEN" {
		t.Fatalf("tokenEnvFor(Forgejo) = %q, want GITEA_TOKEN", got)
	}
}

func TestTokenEnvForIgnoresGitHubPATConfiguration(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ambient-github-token")
	t.Setenv("GH_TOKEN", "ambient-gh-token")
	t.Setenv("ORG_GITHUB_TOKEN", "project-token")

	got := tokenEnvFor(gitprovider.ProviderGitHub)
	if got != "" {
		t.Fatalf("tokenEnvFor(GitHub) = %q, want empty", got)
	}
}

func TestResolveRemoteRepoContextRejectsNestedRegisteredProjectPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	nested := filepath.Join(repo, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatalf("mkdir nested project: %v", err)
	}
	projects := "[outer]\npath = " + quoteTOMLString(repo) +
		"\n[nested]\npath = " + quoteTOMLString(nested) + "\n"
	projectsPath := filepath.Join(home, ".config", "ttal", "projects.toml")
	if err := os.WriteFile(projectsPath, []byte(projects), 0644); err != nil {
		t.Fatalf("write projects.toml: %v", err)
	}

	_, err := resolveRemoteRepoContextFor(nested)
	if err == nil || !strings.Contains(err.Error(), "must be the Git top-level") {
		t.Fatalf("resolveRemoteRepoContextFor error = %v, want registered-path mismatch", err)
	}
}

func TestResolveRemoteRepoContextAllowsUnregisteredSubdirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	subdir := filepath.Join(repo, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	ctx, err := resolveRemoteRepoContextFor(subdir)
	if err != nil {
		t.Fatalf("resolveRemoteRepoContextFor: %v", err)
	}
	if ctx.WorkDir != repo || ctx.ProjectAlias != "test" {
		t.Fatalf("context = workdir %q alias %q, want %q and test", ctx.WorkDir, ctx.ProjectAlias, repo)
	}
}

func TestCleanupMergedBranchSkipsMissingRemoteBranch(t *testing.T) {
	repo := testGitRepoWithMissingRemoteFeature(t)

	if err := cleanupClosedPRBranch(&repoContext{
		Provider:    gitprovider.ProviderForgejo,
		WorkDir:     repo,
		RemoteURL:   "file://" + filepath.Join(repo, "..", "origin.git"),
		Token:       "test-token",
		DefaultBase: branchMain,
		Branch:      "feature",
	}, true); err != nil {
		t.Fatalf("cleanupClosedPRBranch: %v", err)
	}

	current := gitOut(t, repo, "branch", "--show-current")
	if current != branchMain {
		t.Fatalf("current branch = %q, want main", current)
	}
	if err := gitCmd(repo, "rev-parse", "--verify", "feature"); err == nil {
		t.Fatal("feature branch still exists locally")
	}
}

func TestComputeBumpedTagReusesUnpushedLocalLatestTag(t *testing.T) {
	repo := testGitRepoWithRemote(t)
	gitRun(t, repo, "tag", "v1.2.3")

	tag, err := computeBumpedTag(&repoContext{
		Provider: gitprovider.ProviderForgejo,
		WorkDir:  repo,
		Token:    "test-token",
	}, "patch")
	if err != nil {
		t.Fatalf("computeBumpedTag: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("computeBumpedTag = %q, want unpushed local tag v1.2.3", tag)
	}
}

func TestShouldBumpLatestTagUsesCredentialAwareGit(t *testing.T) {
	repo := testGitRepoWithRemote(t)
	ctx := &repoContext{
		Provider:  gitprovider.ProviderForgejo,
		WorkDir:   repo,
		RemoteURL: "https://git.example.test/org/repo.git",
		Token:     "test-token",
	}
	var gotArgs []string
	restore := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	})
	defer restore()

	shouldBump, err := shouldBumpLatestTag(ctx, "v1.2.3")
	if err != nil {
		t.Fatalf("shouldBumpLatestTag: %v", err)
	}
	if !shouldBump {
		t.Fatal("shouldBumpLatestTag = false, want true when remote tag exists")
	}
	if strings.Join(gotArgs, " ") != "ls-remote --exit-code --tags origin refs/tags/v1.2.3" {
		t.Fatalf("credential-aware git args = %v", gotArgs)
	}
}

func TestGitPushAllowsMainAndMasterToReachRemote(t *testing.T) {
	for _, branch := range []string{branchMain, branchMaster} {
		t.Run(branch, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("GH_TOKEN", "")
			repo := testRegisteredHTTPRepo(t, home, branch)

			_, err := NewService(&recordingBroker{}).GitPush(Request{WorkDir: repo})
			if err == nil {
				t.Fatal("expected remote push error")
			}
			if strings.Contains(err.Error(), "refusing to push protected branch") {
				t.Fatalf("main/master should reach remote, got local policy error: %v", err)
			}
		})
	}
}

func TestRunGitWithCredsFailsFastWithoutToken(t *testing.T) {
	err := runGitWithCreds(&repoContext{
		Provider:  gitprovider.ProviderForgejo,
		WorkDir:   t.TempDir(),
		TokenEnv:  "GITHUB_TOKEN",
		RemoteURL: "https://github.com/tta-lab/example.git",
	}, githubapp.PurposeGitRead, "ls-remote", remoteOrigin)
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if !strings.Contains(err.Error(), "missing token: set GITHUB_TOKEN") {
		t.Fatalf("error = %v, want missing token message", err)
	}
}

func TestGitHubReadFallsBackToAnonymousOnlyForScopeErrors(t *testing.T) {
	for _, tokenErr := range []error{githubapp.ErrOwnerNotAllowed, githubapp.ErrInstallationNotFound} {
		t.Run(tokenErr.Error(), func(t *testing.T) {
			broker := &recordingBroker{tokenErr: tokenErr}
			ctx := &repoContext{
				Provider: gitprovider.ProviderGitHub, Owner: "tta-lab", Repo: "organon",
				githubBroker: broker,
			}
			var gotToken string
			restore := stubRunGitWithAuthentication(t, func(_ *repoContext, auth gitAuthentication, _ ...string) error {
				gotToken = auth.token
				return nil
			})
			defer restore()

			if err := runGitWithCreds(ctx, githubapp.PurposeGitRead, "fetch", "origin"); err != nil {
				t.Fatalf("anonymous read: %v", err)
			}
			if gotToken != "" {
				t.Fatalf("anonymous read token = %q", gotToken)
			}
		})
	}
}

func TestGitHubWriteFailsClosedWhenInstallationIsMissing(t *testing.T) {
	broker := &recordingBroker{tokenErr: githubapp.ErrInstallationNotFound}
	ctx := &repoContext{
		Provider: gitprovider.ProviderGitHub, Owner: "tta-lab", Repo: "organon",
		githubBroker: broker,
	}
	runs := 0
	restore := stubRunGitWithAuthentication(t, func(_ *repoContext, _ gitAuthentication, _ ...string) error {
		runs++
		return nil
	})
	defer restore()

	err := runGitWithCreds(ctx, githubapp.PurposeGitWrite, "push", "origin")
	if !errors.Is(err, githubapp.ErrInstallationNotFound) || runs != 0 {
		t.Fatalf("error = %v, git runs = %d", err, runs)
	}
}

func TestGitHubAuthFailureInvalidatesWithoutRetry(t *testing.T) {
	broker := &recordingBroker{token: "rejected-token"}
	ctx := &repoContext{
		Provider: gitprovider.ProviderGitHub, Owner: "tta-lab", Repo: "organon",
		githubBroker: broker,
	}
	runs := 0
	restore := stubRunGitWithAuthentication(t, func(_ *repoContext, _ gitAuthentication, _ ...string) error {
		runs++
		return errors.New("remote: HTTP 401")
	})
	defer restore()

	err := runGitWithCreds(ctx, githubapp.PurposeGitWrite, "push", "origin")
	if err == nil || runs != 1 {
		t.Fatalf("error = %v, git runs = %d; want error and one run", err, runs)
	}
	want := brokerInvalidation{
		owner: "tta-lab", repo: "organon", purpose: githubapp.PurposeGitWrite, token: "rejected-token",
	}
	if len(broker.invalidations) != 1 || broker.invalidations[0] != want {
		t.Fatalf("invalidations = %+v, want [%+v]", broker.invalidations, want)
	}
}

func TestConfirmedGitAuthenticationFailureRecognizesGitHub403(t *testing.T) {
	err := errors.New("fatal: unable to access repository: The requested URL returned error: 403")
	if !confirmedGitAuthenticationFailure(err) {
		t.Fatalf("confirmedGitAuthenticationFailure(%v) = false", err)
	}
}

func testGitRepoWithMissingRemoteFeature(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	gitRun(t, "", "init", "--bare", origin)
	gitRun(t, "", "clone", origin, repo)
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "switch", "-c", branchMain)
	gitRun(t, repo, "commit", "--allow-empty", "-m", "initial")
	gitRun(t, repo, "push", "-u", remoteOrigin, branchMain)
	gitRun(t, repo, "switch", "-c", "feature")
	gitRun(t, repo, "commit", "--allow-empty", "-m", "feature")
	gitRun(t, repo, "push", "-u", remoteOrigin, "feature")
	gitRun(t, repo, "push", remoteOrigin, "--delete", "feature")
	gitRun(t, repo, "remote", "set-head", remoteOrigin, branchMain)
	return repo
}

func testGitRepoWithRemote(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	gitRun(t, "", "init", "--bare", origin)
	gitRun(t, "", "clone", origin, repo)
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "switch", "-c", branchMain)
	gitRun(t, repo, "commit", "--allow-empty", "-m", "initial")
	gitRun(t, repo, "push", "-u", remoteOrigin, branchMain)
	return repo
}

func testRegisteredHTTPRepo(t *testing.T, home, branch string) string {
	return testRegisteredRepo(t, home, branch, "https://github.com/tta-lab/example.git", false)
}

func testRegisteredRepo(
	t *testing.T, home, branch, remote string, archived bool,
) string {
	t.Helper()

	repo := filepath.Join(t.TempDir(), "repo")
	gitRun(t, "", "init", repo)
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	gitRun(t, repo, "switch", "-c", branch)
	gitRun(t, repo, "commit", "--allow-empty", "-m", "initial")
	gitRun(t, repo, "remote", "add", remoteOrigin, remote)

	configDir := filepath.Join(home, ".config", "ttal")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	header := "[test]"
	if archived {
		header = "[archived.test]"
	}
	content := header + "\npath = " + quoteTOMLString(repo) + "\n"
	if err := os.WriteFile(filepath.Join(configDir, "projects.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write projects.toml: %v", err)
	}
	return repo
}

func quoteTOMLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitCmd(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	return cmd.Run()
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func stubRunGitWithCreds(t *testing.T, fn func(*repoContext, ...string) error) func() {
	t.Helper()
	old := runGitWithCredsFunc
	runGitWithCredsFunc = func(ctxInfo *repoContext, _ gitAuthentication, args ...string) error {
		return fn(ctxInfo, args...)
	}
	return func() { runGitWithCredsFunc = old }
}

func stubRunGitWithAuthentication(
	t *testing.T,
	fn func(*repoContext, gitAuthentication, ...string) error,
) func() {
	t.Helper()
	old := runGitWithCredsFunc
	runGitWithCredsFunc = fn
	return func() { runGitWithCredsFunc = old }
}
