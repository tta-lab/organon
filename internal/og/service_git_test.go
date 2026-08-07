package og

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestGitPushRequestsWritePurpose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	broker := &recordingBroker{}
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error { return nil })
	defer restoreGit()

	if _, err := NewService(broker).GitPush(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPush: %v", err)
	}
	want := brokerTokenCall{owner: "tta-lab", repo: "example", purpose: githubapp.PurposeGitWrite}
	if len(broker.tokenCalls) != 1 || broker.tokenCalls[0] != want {
		t.Fatalf("broker calls = %+v, want [%+v]", broker.tokenCalls, want)
	}
}

func TestGitPullRequestsReadPurpose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, branchMain)
	broker := &recordingBroker{}
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error { return nil })
	defer restoreGit()

	if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := brokerTokenCall{owner: "tta-lab", repo: "example", purpose: githubapp.PurposeGitRead}
	if len(broker.tokenCalls) != 1 || broker.tokenCalls[0] != want {
		t.Fatalf("broker calls = %+v, want [%+v]", broker.tokenCalls, want)
	}
}

func TestGitPullFeatureBranchFallsBackToAnonymousForUnmanagedRepository(t *testing.T) {
	for _, tokenErr := range []error{githubapp.ErrOwnerNotAllowed, githubapp.ErrInstallationNotFound} {
		t.Run(tokenErr.Error(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/x")
			broker := &recordingBroker{tokenErr: tokenErr}
			var calls [][]string
			restoreGit := stubRunGitWithAuthentication(t, func(
				_ *repoContext, auth gitAuthentication, args ...string,
			) error {
				if auth.token != "" {
					t.Fatalf("anonymous pull received token %q", auth.token)
				}
				calls = append(calls, append([]string(nil), args...))
				return nil
			})
			defer restoreGit()

			if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err != nil {
				t.Fatalf("GitPull: %v", err)
			}
			wantCalls := [][]string{{"pull", "--ff-only", remoteOrigin, "feature/x"}}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("git calls = %v, want %v", calls, wantCalls)
			}
			wantPurposes := []githubapp.Purpose{githubapp.PurposeAPI, githubapp.PurposeGitRead}
			gotPurposes := make([]githubapp.Purpose, 0, len(broker.tokenCalls))
			for _, call := range broker.tokenCalls {
				gotPurposes = append(gotPurposes, call.purpose)
			}
			if !reflect.DeepEqual(gotPurposes, wantPurposes) {
				t.Fatalf("broker purposes = %v, want %v", gotPurposes, wantPurposes)
			}
		})
	}
}

func TestGitTagRequestsWritePurpose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, branchMain)
	broker := &recordingBroker{}
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error { return nil })
	defer restoreGit()

	if _, err := NewService(broker).GitTag(Request{WorkDir: repo, Tag: "v1.2.3"}); err != nil {
		t.Fatalf("GitTag: %v", err)
	}
	want := brokerTokenCall{owner: "tta-lab", repo: "example", purpose: githubapp.PurposeGitWrite}
	if len(broker.tokenCalls) != 1 || broker.tokenCalls[0] != want {
		t.Fatalf("broker calls = %+v, want [%+v]", broker.tokenCalls, want)
	}
}

func TestGitPushPassesForceWithLease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	var got []string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		got = append([]string(nil), args...)
		return nil
	})
	defer restoreGit()

	if _, err := NewService(&recordingBroker{}).GitPush(Request{WorkDir: repo, Force: true}); err != nil {
		t.Fatalf("GitPush: %v", err)
	}
	want := []string{"push", "-u", remoteOrigin, "feature/x", "--force-with-lease"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("git args = %v, want %v", got, want)
	}
}

func TestGitPushRejectsForceOnDefaultBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, branchMain)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	called := false
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
		called = true
		return nil
	})
	defer restoreGit()

	_, err := NewService(&recordingBroker{}).GitPush(Request{WorkDir: repo, Force: true})
	if err == nil || !strings.Contains(err.Error(), "refusing to force push default branch") {
		t.Fatalf("GitPush error = %v, want default-branch refusal", err)
	}
	if called {
		t.Fatal("GitPush called git after rejecting force push to default branch")
	}
}

func TestGitPushRejectsForceWhenDefaultBranchIsUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, branchMaster)
	called := false
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
		called = true
		return nil
	})
	defer restoreGit()

	_, err := NewService(&recordingBroker{}).GitPush(Request{WorkDir: repo, Force: true})
	if err == nil || !strings.Contains(err.Error(), "default branch is unknown") {
		t.Fatalf("GitPush error = %v, want unknown-default refusal", err)
	}
	if called {
		t.Fatal("GitPush called git when force-push safety could not identify the default branch")
	}
}

func TestGitPullDefaultBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, branchMain)
	var calls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	defer restoreGit()

	if _, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := [][]string{{"pull", "--ff-only", remoteOrigin, branchMain}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %v, want %v", calls, want)
	}
}

func TestGitPullArchivedDefaultBranchIsReadOnlyFastForward(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, branchMain, "https://github.com/tta-lab/example.git", true)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	var calls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		t.Fatal("archived pull must not call provider API")
		return nil, nil
	})
	defer restoreProvider()

	if _, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := [][]string{{"pull", "--ff-only", remoteOrigin, branchMain}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %v, want %v", calls, want)
	}
}

func TestGitPullArchivedRepositoryRejectsUnsafeCheckout(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		setDefault bool
		want       string
	}{
		{"feature branch", "feature/x", true, "default branch"},
		{"unknown default", branchMain, false, "default branch is unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredRepo(t, home, tt.branch, "https://github.com/tta-lab/example.git", true)
			if tt.setDefault {
				gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			}
			called := false
			restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
				called = true
				return nil
			})
			defer restoreGit()

			_, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("GitPull error = %v, want containing %q", err, tt.want)
			}
			if called {
				t.Fatal("GitPull reached the network after archived checkout refusal")
			}
		})
	}
}

func TestGenericGitPullSkipsProviderAndBranchCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "must-not-leak")
	repo := testRegisteredRepo(t, home, "feature/x", "https://codeberg.org/forgejo/forgejo.git", false)
	var gotAuth gitAuthentication
	var gotArgs []string
	restoreGit := stubRunGitWithAuthentication(t, func(
		_ *repoContext, auth gitAuthentication, args ...string,
	) error {
		gotAuth = auth
		gotArgs = append([]string(nil), args...)
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		t.Fatal("generic pull must not call provider API")
		return nil, nil
	})
	defer restoreProvider()

	if _, err := NewService(nil).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	if gotAuth.token != "" {
		t.Fatalf("generic pull token = %q, want anonymous", gotAuth.token)
	}
	want := []string{"pull", "--ff-only", remoteOrigin, "feature/x"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("git args = %v, want %v", gotArgs, want)
	}
}

func TestArchivedAndGenericRepositoriesRejectGitWrites(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		archived bool
	}{
		{"archived GitHub", "https://github.com/tta-lab/example.git", true},
		{"generic HTTPS", "https://codeberg.org/forgejo/forgejo.git", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredRepo(t, home, "feature/x", tt.remote, tt.archived)
			called := false
			restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
				called = true
				return nil
			})
			defer restoreGit()

			for name, call := range map[string]func() error{
				"push": func() error { _, err := NewService(nil).GitPush(Request{WorkDir: repo}); return err },
				"tag":  func() error { _, err := NewService(nil).GitTag(Request{WorkDir: repo, Tag: "v1.0.0"}); return err },
			} {
				if err := call(); err == nil || !strings.Contains(err.Error(), "read-only") {
					t.Fatalf("%s error = %v, want read-only refusal", name, err)
				}
			}
			if called {
				t.Fatal("Git write reached network after policy refusal")
			}
		})
	}
}

func TestGitPullFeatureBranchFallsBackToBranchPullWhenNoPR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	var calls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return nil, fmt.Errorf("no all PR found for %s -> %s", head, base)
			},
		}, nil
	})
	defer restoreProvider()

	if _, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := [][]string{{"pull", "--ff-only", remoteOrigin, "feature/x"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %v, want %v", calls, want)
	}
}

func TestGitPullFeatureBranchReturnsPRLookupError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		t.Fatalf("git should not run after PR lookup auth failure: %v", args)
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return nil, fmt.Errorf("401 unauthorized")
			},
		}, nil
	})
	defer restoreProvider()

	_, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo})
	if err == nil {
		t.Fatal("expected PR lookup error")
	}
	if !strings.Contains(err.Error(), "401 unauthorized") {
		t.Fatalf("error = %v, want provider error", err)
	}
}

func TestGitPullMergedBranchCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "branch", branchMain)
	featureSHA := gitOut(t, repo, "rev-parse", "feature/x")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/x", featureSHA)
	broker := &recordingBroker{}
	var calls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:  5,
					Head:   "feature/x",
					Base:   branchMain,
					State:  "closed",
					Merged: true,
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	if _, err := NewService(broker).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := [][]string{
		{"fetch", "--prune", remoteOrigin},
		{"pull", "--ff-only", remoteOrigin, branchMain},
		{"push", remoteOrigin, "--delete", "feature/x"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %v, want %v", calls, want)
	}
	wantPurposes := []githubapp.Purpose{
		githubapp.PurposeGitRead, githubapp.PurposeGitRead, githubapp.PurposeGitWrite,
	}
	gotPurposes := make([]githubapp.Purpose, 0, len(broker.tokenCalls))
	for _, call := range broker.tokenCalls {
		gotPurposes = append(gotPurposes, call.purpose)
	}
	if !reflect.DeepEqual(gotPurposes, wantPurposes) {
		t.Fatalf("broker purposes = %v, want %v", gotPurposes, wantPurposes)
	}
}

func TestGitPullClosedUnmergedBranchCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "branch", branchMain)
	featureSHA := gitOut(t, repo, "rev-parse", "feature/x")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/x", featureSHA)
	var calls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index: 5, Head: "feature/x", Base: branchMain, State: "closed", Merged: false,
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	wantCalls := [][]string{
		{"fetch", "--prune", remoteOrigin},
		{"pull", "--ff-only", remoteOrigin, branchMain},
		{"push", remoteOrigin, "--delete", "feature/x"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("git calls = %v, want %v", calls, wantCalls)
	}
	if !strings.Contains(resp.Message, "Closed PR") {
		t.Fatalf("message = %q, want closed PR cleanup", resp.Message)
	}
}

func TestGitPullClosedUnmergedBranchKeepsOnlyRemainingLocalRef(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "branch", branchMain)
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error { return nil })
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index: 5, Head: "feature/x", Base: branchMain, State: "closed", Merged: false,
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	_, err := NewService(&recordingBroker{}).GitPull(Request{WorkDir: repo})
	if err == nil || !strings.Contains(err.Error(), "remote branch is missing") {
		t.Fatalf("GitPull error = %v, want missing remote branch refusal", err)
	}
	if got := gitOut(t, repo, "branch", "--show-current"); got != "feature/x" {
		t.Fatalf("current branch = %q, want feature/x retained", got)
	}
	if err := gitCmd(repo, "rev-parse", "--verify", "feature/x"); err != nil {
		t.Fatalf("feature branch was deleted: %v", err)
	}
}
