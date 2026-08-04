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
