package og

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestPRCreatePushesCurrentBranchBeforeCreatingPR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	var calls []string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, "git:"+joinArgs(args))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		calls = append(calls, "provider")
		return fakeProvider{
			createPR: func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error) {
				calls = append(calls, "create")
				return &gitprovider.PullRequest{Index: 7, HTMLURL: "https://pr/7"}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := Service{}.PRCreate(Request{WorkDir: testRegisteredHTTPRepo(t, home, "feature/x"), Title: "title"})
	if err != nil {
		t.Fatalf("PRCreate: %v", err)
	}
	if resp.PR == nil || resp.PR.Index != 7 {
		t.Fatalf("response PR = %+v, want #7", resp.PR)
	}
	want := []string{"git:push -u origin feature/x", "provider", "create"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestFindPRUsesCommitLookupForGitHub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "commit", "--allow-empty", "-m", "feature")
	expectedSHA := gitOut(t, repo, "rev-parse", "HEAD")

	var gotSHA string
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeCommitProvider{
			fakeProvider: fakeProvider{
				findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
					t.Fatal("FindPRByState should not be called when GitHub commit lookup succeeds")
					return nil, nil
				},
			},
			findPRByCommit: func(owner, repo, sha string) (*gitprovider.PullRequest, error) {
				gotSHA = sha
				return &gitprovider.PullRequest{
					Index:   9,
					HTMLURL: "https://pr/9",
					Head:    "feature/x",
					Base:    branchMain,
					State:   stateAll,
					HeadSHA: sha,
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	ctx, err := resolveRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRepoContextFor: %v", err)
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		t.Fatalf("findPR: %v", err)
	}
	if pr.Index != 9 {
		t.Fatalf("PR index = %d, want 9", pr.Index)
	}
	if gotSHA != expectedSHA {
		t.Fatalf("commit lookup SHA = %q, want %q", gotSHA, expectedSHA)
	}
}

func TestFindPRFallsBackToBranchLookupWhenGitHubCommitLookupMisses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")

	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeCommitProvider{
			fakeProvider: fakeProvider{
				findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
					return &gitprovider.PullRequest{
						Index: 22,
						Head:  "feature/x",
						Base:  branchMain,
						State: stateAll,
					}, nil
				},
			},
			findPRByCommit: func(owner, repo, sha string) (*gitprovider.PullRequest, error) {
				return nil, fmt.Errorf("no PR found for commit %s", sha)
			},
		}, nil
	})
	defer restoreProvider()

	ctx, err := resolveRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRepoContextFor: %v", err)
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		t.Fatalf("findPR: %v", err)
	}
	if pr.Index != 22 {
		t.Fatalf("PR index = %d, want branch fallback PR #22", pr.Index)
	}
}

func TestFindPRFallsBackToBranchLookupWhenGitHubCommitLookupMismatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")

	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeCommitProvider{
			fakeProvider: fakeProvider{
				findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
					return &gitprovider.PullRequest{
						Index: 23,
						Head:  "feature/x",
						Base:  branchMain,
						State: stateAll,
					}, nil
				},
			},
			findPRByCommit: func(owner, repo, sha string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index: 1,
					Head:  "other-branch",
					Base:  branchMain,
					State: stateAll,
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	ctx, err := resolveRepoContextFor(repo)
	if err != nil {
		t.Fatalf("resolveRepoContextFor: %v", err)
	}
	pr, err := findPR(ctx, stateAll)
	if err != nil {
		t.Fatalf("findPR: %v", err)
	}
	if pr.Index != 23 {
		t.Fatalf("PR index = %d, want branch fallback PR #23", pr.Index)
	}
}

func TestPRFailuresFetchesFailureDetails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "commit", "--allow-empty", "-m", "feature")
	expectedSHA := gitOut(t, repo, "rev-parse", "HEAD")
	var gotTail int
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:   7,
					Head:    "feature/x",
					Base:    branchMain,
					State:   stateAll,
					HeadSHA: expectedSHA,
				}, nil
			},
			getCIFailureDetails: func(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error) {
				gotTail = tailLines
				if sha != expectedSHA {
					t.Fatalf("sha = %q, want %q", sha, expectedSHA)
				}
				return []*gitprovider.JobFailure{{
					WorkflowName: "check",
					JobName:      "test",
					LogTail:      "panic: bad\nexit status 1",
					HTMLURL:      "https://ci/job/1",
				}}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := (Service{}).PRFailures(Request{WorkDir: repo, Tail: 12})
	if err != nil {
		t.Fatalf("PRFailures: %v", err)
	}
	if gotTail != 12 {
		t.Fatalf("tail = %d, want 12", gotTail)
	}
	got := strings.Join(resp.Lines, "\n")
	for _, want := range []string{"check / test", "https://ci/job/1", "panic: bad", "exit status 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("failure lines = %q, want substring %q", got, want)
		}
	}
}

func TestPRLogPrintsStatusBeforeFailureDetails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:   7,
					Head:    "feature/x",
					Base:    branchMain,
					State:   stateAll,
					HeadSHA: "abc123456789",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				if ref != "abc123456789" {
					t.Fatalf("ref = %q, want abc123456789", ref)
				}
				return &gitprovider.CombinedStatus{
					State: gitprovider.StateFailure,
					Statuses: []*gitprovider.CommitStatus{{
						Context:     "check",
						State:       gitprovider.StateFailure,
						Description: "failed",
					}},
				}, nil
			},
			getCIFailureDetails: func(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error) {
				if sha != "abc123456789" {
					t.Fatalf("sha = %q, want abc123456789", sha)
				}
				if tailLines != 12 {
					t.Fatalf("tail = %d, want 12", tailLines)
				}
				return []*gitprovider.JobFailure{{
					WorkflowName: "check",
					JobName:      "test",
					LogTail:      "panic: bad\nexit status 1",
					HTMLURL:      "https://ci/job/1",
				}}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := (Service{}).PRLog(Request{WorkDir: repo, Tail: 12})
	if err != nil {
		t.Fatalf("PRLog: %v", err)
	}
	got := strings.Join(resp.Lines, "\n")
	for _, want := range []string{
		"CI Status for abc12345: failed",
		"check",
		"Failure Details:",
		"Workflow: check",
		"Job: test",
		"Log tail:",
		"panic: bad",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log lines = %q, want substring %q", got, want)
		}
	}
	if strings.Index(got, "CI Status") > strings.Index(got, "Failure Details:") {
		t.Fatalf("CI summary should appear before failure details:\n%s", got)
	}
}

func TestPRLogDoesNotFetchFailureDetailsWhenCIPasses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:   7,
					Head:    "feature/x",
					Base:    branchMain,
					State:   stateAll,
					HeadSHA: "abc123456789",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{
					State: gitprovider.StateSuccess,
					Statuses: []*gitprovider.CommitStatus{{
						Context: "check",
						State:   gitprovider.StateSuccess,
					}},
				}, nil
			},
			getCIFailureDetails: func(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error) {
				t.Fatal("failure details should not be fetched when CI passes")
				return nil, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := (Service{}).PRLog(Request{WorkDir: repo, Tail: 12})
	if err != nil {
		t.Fatalf("PRLog: %v", err)
	}
	got := strings.Join(resp.Lines, "\n")
	if !strings.Contains(got, "CI Status for abc12345: passed") {
		t.Fatalf("log lines = %q, want passed status", got)
	}
	if strings.Contains(got, "Failure Details:") {
		t.Fatalf("log lines should not include failure details when CI passes:\n%s", got)
	}
}

func TestPRViewIncludesCISummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:   9,
					Head:    "feature/x",
					Base:    branchMain,
					State:   stateAll,
					HeadSHA: "abc123",
				}, nil
			},
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{
					Index:   index,
					Title:   "title",
					Head:    "feature/x",
					Base:    branchMain,
					State:   "open",
					HeadSHA: "abc123",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				if ref != "abc123" {
					t.Fatalf("ref = %q, want abc123", ref)
				}
				return &gitprovider.CombinedStatus{
					State: gitprovider.StateSuccess,
					Statuses: []*gitprovider.CommitStatus{{
						Context:     "check",
						State:       gitprovider.StateSuccess,
						Description: "passed",
						TargetURL:   "https://ci/job/1",
					}},
				}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := (Service{}).PRView(Request{WorkDir: repo})
	if err != nil {
		t.Fatalf("PRView: %v", err)
	}
	if resp.PR == nil || resp.PR.CI == nil {
		t.Fatalf("PR CI = nil, response = %+v", resp.PR)
	}
	if resp.PR.CI.State != gitprovider.StateSuccess {
		t.Fatalf("CI state = %q, want success", resp.PR.CI.State)
	}
	if len(resp.PR.CI.Statuses) != 1 || resp.PR.CI.Statuses[0].Context != "check" {
		t.Fatalf("CI statuses = %+v, want check", resp.PR.CI.Statuses)
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

func stubNewProvider(t *testing.T, fn func(*repoContext) (gitprovider.Provider, error)) func() {
	t.Helper()
	old := newProviderFunc
	newProviderFunc = fn
	return func() { newProviderFunc = old }
}

type fakeProvider struct {
	createPR            func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error)
	findPRByState       func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error)
	getPR               func(owner, repo string, index int64) (*gitprovider.PullRequest, error)
	getCombinedStatus   func(owner, repo, ref string) (*gitprovider.CombinedStatus, error)
	getCIFailureDetails func(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error)
}

func (p fakeProvider) Name() string { return "fake" }

func (p fakeProvider) CreatePR(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error) {
	return p.createPR(owner, repo, head, base, title, body)
}

func (p fakeProvider) FindPR(owner, repo, head, base string) (*gitprovider.PullRequest, error) {
	return p.FindPRByState(owner, repo, head, base, "open")
}

func (p fakeProvider) FindPRByState(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
	return p.findPRByState(owner, repo, head, base, state)
}

func (p fakeProvider) EditPR(owner, repo string, index int64, title, body string) (*gitprovider.PullRequest, error) {
	panic("not implemented")
}

func (p fakeProvider) GetPR(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
	if p.getPR != nil {
		return p.getPR(owner, repo, index)
	}
	panic("not implemented")
}

func (p fakeProvider) MergePR(owner, repo string, index int64, deleteBranch bool) error {
	panic("not implemented")
}

func (p fakeProvider) CreateComment(owner, repo string, index int64, body string) (*gitprovider.Comment, error) {
	panic("not implemented")
}

func (p fakeProvider) ListComments(owner, repo string, index int64) ([]*gitprovider.Comment, error) {
	panic("not implemented")
}

func (p fakeProvider) GetCombinedStatus(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
	if p.getCombinedStatus != nil {
		return p.getCombinedStatus(owner, repo, ref)
	}
	panic("not implemented")
}

func (p fakeProvider) GetCIFailureDetails(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error) {
	if p.getCIFailureDetails != nil {
		return p.getCIFailureDetails(owner, repo, sha, tailLines)
	}
	panic("not implemented")
}

type fakeCommitProvider struct {
	fakeProvider
	findPRByCommit func(owner, repo, sha string) (*gitprovider.PullRequest, error)
}

func (p fakeCommitProvider) FindPRByCommit(owner, repo, sha string) (*gitprovider.PullRequest, error) {
	return p.findPRByCommit(owner, repo, sha)
}
