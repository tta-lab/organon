package og

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
)

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

	if _, err := (Service{}).GitPush(Request{WorkDir: repo, Force: true}); err != nil {
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

	if _, err := (Service{}).GitPull(Request{WorkDir: repo}); err != nil {
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

	if _, err := (Service{}).GitPull(Request{WorkDir: repo}); err != nil {
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

	_, err := (Service{}).GitPull(Request{WorkDir: repo})
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

	if _, err := (Service{}).GitPull(Request{WorkDir: repo}); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	want := [][]string{
		{"fetch", "--prune", remoteOrigin},
		{"switch", branchMain},
		{"pull", "--ff-only", remoteOrigin, branchMain},
		{"branch", "-D", "feature/x"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("git calls = %v, want %v", calls, want)
	}
}
