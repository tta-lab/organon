package og

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
)

func TestPATBrokerOwnerAndFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "token")
	const secret = "fixture-secret-do-not-display"
	if err := os.WriteFile(file, []byte(secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	b := &patBroker{owner: "ExampleOrg", file: "token", configDir: dir}
	for _, purpose := range []githubapp.Purpose{
		githubapp.PurposeGitRead, githubapp.PurposeGitWrite, githubapp.PurposeAPI, githubapp.PurposeIssueWrite,
	} {
		token, err := b.Token(context.Background(), "exampleorg", "repo", purpose)
		if err != nil || token != secret {
			t.Fatalf("token for %s: %v", purpose, err)
		}
	}
	if _, err := b.Token(context.Background(), "other", "repo", githubapp.PurposeGitRead); err == nil ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("owner rejection: %v", err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Token(context.Background(), "exampleorg", "repo", githubapp.PurposeAPI); err == nil ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe file: %v", err)
	}
}

func TestGitHubRejectsMergeAndChecksBeforeProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restore := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		t.Fatal("provider must not be called")
		return nil, nil
	})
	defer restore()
	for _, broker := range []githubapp.CredentialBroker{&recordingBroker{}, &patBroker{owner: "tta-lab"}} {
		for _, policy := range []string{"impri", "none", "deny"} {
			for _, dry := range []bool{false, true} {
				s := NewServiceWithConfig(broker, nil, ogconfig.Config{Merge: ogconfig.MergeConfig{Approval: policy}})
				_, err := s.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: dry})
				if err == nil {
					t.Fatalf("merge %s dry=%v succeeded", policy, dry)
				}
				if !strings.Contains(err.Error(), "not supported") &&
					(policy != "deny" || !strings.Contains(err.Error(), "denied")) {
					t.Fatalf("merge %s dry=%v: %v", policy, dry, err)
				}
			}
		}
		_, err := NewService(broker).PRChecks(Request{WorkDir: repo, Index: 7})
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("checks: %v", err)
		}
	}

}

func TestGitHubPRViewDoesNotQueryCI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restore := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(_, _, _, _, _ string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{Index: 7, Head: "feature/x", Base: "main", State: "open", HeadSHA: "abc"}, nil
			},
			getPR: func(_, _ string, n int64) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{Index: n, HeadSHA: "abc"}, nil
			},
			getCombinedStatus: func(_, _, _ string) (*gitprovider.CombinedStatus, error) {
				t.Fatal("GitHub CI queried")
				return nil, nil
			},
		}, nil
	})
	defer restore()
	for _, broker := range []githubapp.CredentialBroker{&recordingBroker{}, &patBroker{owner: "tta-lab"}} {
		resp, err := NewService(broker).PRView(Request{WorkDir: repo})
		if err != nil || resp.PR == nil || resp.PR.CI != nil || resp.PR.CIFetchError != "" {
			t.Fatalf("view=%+v err=%v", resp.PR, err)
		}
	}
}

type patRoundTrip func(*http.Request) (*http.Response, error)

func (f patRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPATUsedForGitAndAPIsWithoutFallbackOrLeak(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const secret = "fixture-token-secret"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	broker := &patBroker{owner: "owner", configDir: dir, file: "token"}
	ctx := &repoContext{Provider: gitprovider.ProviderGitHub, Owner: "owner", Repo: "repo", githubBroker: broker}
	for _, purpose := range []githubapp.Purpose{githubapp.PurposeGitRead, githubapp.PurposeGitWrite} {
		auth, err := gitAuthenticationFor(ctx, purpose)
		if err != nil || auth.token != secret {
			t.Fatalf("git auth: %v", err)
		}
	}
	info := &gitprovider.RepoInfo{Owner: "other", Repo: "repo"}
	auth, err := NewService(broker).cloneAuthentication(context.Background(), cloneSource{
		provider: gitprovider.ProviderGitHub, info: info,
	})
	if err != nil || auth.token != "" {
		t.Fatalf("out-of-owner clone authentication: %+v, %v", auth, err)
	}
	ctx.Owner = "other"
	if auth, err := gitAuthenticationFor(ctx, githubapp.PurposeGitRead); err != nil || auth.token != "" {
		t.Fatalf("out-of-owner pull authentication: %+v, %v", auth, err)
	}
	if _, err := gitAuthenticationFor(ctx, githubapp.PurposeGitWrite); !errors.Is(err, githubapp.ErrOwnerNotAllowed) {
		t.Fatalf("out-of-owner write: %v", err)
	}
	if _, err := newProviderImpl(ctx); !errors.Is(err, githubapp.ErrOwnerNotAllowed) {
		t.Fatalf("out-of-owner API: %v", err)
	}
	if _, err := newIssueProvider(ctx, githubapp.PurposeIssueRead); !errors.Is(err, githubapp.ErrOwnerNotAllowed) {
		t.Fatalf("out-of-owner issue API: %v", err)
	}
	ctx.Owner = "owner"
	testPATAPIRequests(t, ctx, secret)
}

func testPATAPIRequests(t *testing.T, ctx *repoContext, secret string) {
	t.Helper()
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	var paths []string
	http.DefaultTransport = patRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing PAT authorization")
		}
		paths = append(paths, req.URL.Path)
		status, body := 200, `{"number":7,"title":"example","state":"open"}`
		if strings.HasSuffix(req.URL.Path, "/issues/9") {
			status, body = 403, `{"message":"`+secret+` denied"}`
		}
		return &http.Response{
			StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req,
		}, nil
	})
	provider, err := newProviderImpl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.GetPR("owner", "repo", 7); err != nil {
		t.Fatalf("PR: %v", err)
	}
	issues, err := newIssueProvider(ctx, githubapp.PurposeIssueRead)
	if err != nil {
		t.Fatal(err)
	}
	_, err = issues.GetIssue("owner", "repo", 9)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("issue rejection leaked token or succeeded: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/repos/owner/repo/pulls/7" || paths[1] != "/repos/owner/repo/issues/9" {
		t.Fatalf("paths: %v", paths)
	}
}

func TestLoadServicePATStatusChecksOnlyLocalScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	dir := filepath.Join(home, ".config", "ttal")
	tokenDir := filepath.Join(dir, "og")
	if err := os.Mkdir(tokenDir, 0700); err != nil {
		t.Fatal(err)
	}
	const secret = "fixture-secret-never-render"
	if err := os.WriteFile(filepath.Join(tokenDir, "pat"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	config := "[github_pat]\nowner = \"tta-lab\"\ntoken_file = \"og/pat\"\n"
	path := filepath.Join(dir, "og.toml")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := LoadService(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := service.AuthStatus(Request{WorkDir: repo})
	if err != nil || resp.Auth == nil || resp.Auth.AuthMode != "github-pat" || resp.Auth.Ready ||
		!strings.Contains(resp.Message, "not verified") || strings.Contains(resp.Message, secret) {
		t.Fatalf("status=%+v err=%v", resp, err)
	}
}

func TestPATOutOfOwnerAnonymousServiceReads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A nonexistent token file proves the owner check precedes any attempt to read it.
	broker := &patBroker{owner: "another-owner", configDir: home, file: "missing-token"}
	sentinel := errors.New("fixture stopped before clone")
	calls := 0
	withCloneRunner(t, func(_ context.Context, invocation cloneInvocation) error {
		calls++
		if invocation.Token != "" || invocation.Owner != "tta-lab" {
			t.Fatalf("clone authentication: %+v", invocation)
		}
		return sentinel
	})
	service := NewServiceWithConfig(broker, writeCloneProjects(t, ""), ogconfig.Config{})
	_, err := service.GitClone(Request{URL: "https://github.com/tta-lab/example.git", Reference: true})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("clone calls=%d error=%v", calls, err)
	}

	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	var pullCalls int
	restore := stubRunGitWithAuthentication(t, func(_ *repoContext, auth gitAuthentication, args ...string) error {
		pullCalls++
		if auth.token != "" || strings.Join(args, " ") != "pull --ff-only origin feature/x" {
			t.Fatalf("pull auth=%+v args=%v", auth, args)
		}
		return nil
	})
	defer restore()
	if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err != nil || pullCalls != 1 {
		t.Fatalf("pull calls=%d error=%v", pullCalls, err)
	}
	_, err = NewService(broker).GitPush(Request{WorkDir: repo})
	if !errors.Is(err, githubapp.ErrOwnerNotAllowed) || pullCalls != 1 {
		t.Fatalf("push calls=%d error=%v", pullCalls, err)
	}
	_, err = NewService(broker).PRGet(Request{WorkDir: repo, Index: 1})
	if !errors.Is(err, githubapp.ErrOwnerNotAllowed) {
		t.Fatalf("PR get error=%v", err)
	}
}

func TestPATInOwnerFailureDoesNotFallBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, branchMain)
	broker := &patBroker{owner: "tta-lab", configDir: home, file: "missing-token"}
	withCloneRunner(t, func(context.Context, cloneInvocation) error {
		t.Fatal("in-scope missing PAT must not invoke clone")
		return nil
	})
	if _, err := NewServiceWithConfig(broker, writeCloneProjects(t, ""), ogconfig.Config{}).GitClone(
		Request{URL: "https://github.com/tta-lab/example.git", Reference: true},
	); err == nil {
		t.Fatal("in-scope clone with missing PAT succeeded")
	}
	calls := 0
	restore := stubRunGitWithAuthentication(t, func(_ *repoContext, auth gitAuthentication, _ ...string) error {
		calls++
		if auth.token == "" {
			t.Fatal("in-scope failure fell back to anonymous")
		}
		return errors.New("HTTP 401")
	})
	defer restore()
	if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err == nil || calls != 0 {
		t.Fatalf("missing credential: calls=%d error=%v", calls, err)
	}
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "missing-token"), []byte("fixture-token"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err == nil || calls != 1 {
		t.Fatalf("rejected credential: calls=%d error=%v", calls, err)
	}
}
