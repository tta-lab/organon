package og

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestGitPushPassesForceWithLease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
				return nil, fmt.Errorf("no PR found")
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

func TestGitPullMergedBranchCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
