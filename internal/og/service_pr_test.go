package og

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/gitprovider"
)

func TestPRCreatePushesCurrentBranchBeforeCreatingPR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "token")
	var calls []string
	broker := &recordingBroker{}
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		calls = append(calls, "git:"+joinArgs(args))
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(ctx *repoContext) (gitprovider.Provider, error) {
		calls = append(calls, "provider")
		if _, err := broker.Token(context.Background(), ctx.Owner, ctx.Repo, githubapp.PurposeAPI); err != nil {
			return nil, err
		}
		return fakeProvider{
			createPR: func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error) {
				calls = append(calls, "create")
				return &gitprovider.PullRequest{Index: 7, HTMLURL: "https://pr/7"}, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := NewService(broker).PRCreate(Request{
		WorkDir: testRegisteredHTTPRepo(t, home, "feature/x"), Title: ptrTo("title"),
	})
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
	wantPurposes := []githubapp.Purpose{githubapp.PurposeGitWrite, githubapp.PurposeAPI}
	gotPurposes := make([]githubapp.Purpose, 0, len(broker.tokenCalls))
	for _, call := range broker.tokenCalls {
		gotPurposes = append(gotPurposes, call.purpose)
	}
	if !reflect.DeepEqual(gotPurposes, wantPurposes) {
		t.Fatalf("broker purposes = %v, want %v", gotPurposes, wantPurposes)
	}
}

func TestPRCreateRejectsBlankTitleBeforeSideEffects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	var calls []string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
		calls = append(calls, "push")
		return nil
	})
	defer restoreGit()
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		calls = append(calls, "provider")
		return fakeProvider{createPR: func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error) {
			calls = append(calls, "create")
			return &gitprovider.PullRequest{Index: 7}, nil
		}}, nil
	})
	defer restoreProvider()

	blank := " \t\n"
	_, err := NewService(&recordingBroker{}).PRCreate(Request{WorkDir: repo, Title: &blank})
	if err == nil || !strings.Contains(err.Error(), "title must not be blank") {
		t.Fatalf("PRCreate error = %v, want blank-title rejection", err)
	}
	if len(calls) != 0 {
		t.Fatalf("PRCreate side-effect calls = %v, want none", calls)
	}
}

func TestArchivedAndGenericRepositoriesRejectPRWrites(t *testing.T) {
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
			gitCalled := false
			restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error {
				gitCalled = true
				return nil
			})
			defer restoreGit()
			providerCalled := false
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				providerCalled = true
				return fakeProvider{}, nil
			})
			defer restoreProvider()
			title, body := "title", "body"
			calls := map[string]func() error{
				"create": func() error { _, err := NewService(nil).PRCreate(Request{WorkDir: repo, Title: &title}); return err },
				"modify": func() error {
					_, err := NewService(nil).PRModify(Request{WorkDir: repo, Index: 7, Title: &title})
					return err
				},
				"comment": func() error {
					_, err := NewService(nil).PRComment(Request{WorkDir: repo, Index: 7, Body: &body})
					return err
				},
			}
			for name, call := range calls {
				if err := call(); err == nil || !strings.Contains(err.Error(), "read-only") {
					t.Fatalf("%s error = %v, want read-only refusal", name, err)
				}
			}
			if gitCalled || providerCalled {
				t.Fatalf("PR write reached side effect: git=%v provider=%v", gitCalled, providerCalled)
			}
		})
	}
}

func TestCreatePRRejectsInvalidProviderResult(t *testing.T) {
	for _, tt := range []struct {
		name string
		pr   *gitprovider.PullRequest
	}{
		{name: "nil"},
		{name: "non-positive ID", pr: &gitprovider.PullRequest{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{createPR: func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error) {
					return tt.pr, nil
				}}, nil
			})
			defer restoreProvider()

			_, err := createPR(&repoContext{}, "title", "body")
			if err == nil || !strings.Contains(err.Error(), "invalid PR") {
				t.Fatalf("createPR error = %v, want invalid provider PR", err)
			}
		})
	}
}

func TestFindPRRejectsInvalidProviderResult(t *testing.T) {
	for _, tt := range []struct {
		name string
		pr   *gitprovider.PullRequest
	}{
		{name: "nil"},
		{name: "non-positive ID", pr: &gitprovider.PullRequest{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{findPRByState: func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error) {
					return tt.pr, nil
				}}, nil
			})
			defer restoreProvider()

			_, err := findPR(&repoContext{Provider: gitprovider.ProviderForgejo}, stateAll)
			if err == nil || !strings.Contains(err.Error(), "invalid PR") {
				t.Fatalf("findPR error = %v, want invalid provider PR", err)
			}
		})
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

func TestPRGetWithExplicitIDWorksOnDetachedHEAD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "remote", "set-url", "--push", remoteOrigin, "https://attacker.invalid/tta-lab/example.git")
	gitRun(t, repo, "checkout", "--detach")

	restoreProvider := stubNewProvider(t, func(ctx *repoContext) (gitprovider.Provider, error) {
		if ctx.Branch != "" || ctx.DefaultBase != "" {
			t.Fatalf("explicit PR resolver populated branch fields: %+v", ctx)
		}
		return fakeProvider{getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
			if index != 41 {
				t.Fatalf("GetPR index = %d, want 41", index)
			}
			return &gitprovider.PullRequest{Index: index, Title: "remote", HTMLURL: "https://pr/41"}, nil
		}}, nil
	})
	defer restoreProvider()

	resp, err := NewService(&recordingBroker{}).PRGet(Request{WorkDir: repo, Index: 41})
	if err != nil {
		t.Fatalf("PRGet on detached HEAD: %v", err)
	}
	if resp.PR == nil || resp.PR.Index != 41 {
		t.Fatalf("response PR = %+v, want #41", resp.PR)
	}
}

func TestPRModifyMergesAbsentFieldsAndCanClearBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "remote", "set-url", "--push", remoteOrigin, "https://attacker.invalid/tta-lab/example.git")
	gitRun(t, repo, "checkout", "--detach")

	var edits [][2]string
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{Index: index, Title: "old title", Body: "old body"}, nil
			},
			editPR: func(owner, repo string, index int64, title, body string) (*gitprovider.PullRequest, error) {
				edits = append(edits, [2]string{title, body})
				return &gitprovider.PullRequest{Index: index, Title: title, Body: body, HTMLURL: "https://pr/7"}, nil
			},
		}, nil
	})
	defer restoreProvider()

	newTitle := "new title"
	resp, err := NewService(&recordingBroker{}).PRModify(Request{WorkDir: repo, Index: 7, Title: &newTitle})
	if err != nil {
		t.Fatalf("PRModify title: %v", err)
	}
	if resp.PR == nil || resp.PR.Title != newTitle || resp.PR.Body != "old body" {
		t.Fatalf("title response = %+v", resp.PR)
	}
	emptyBody := ""
	resp, err = NewService(&recordingBroker{}).PRModify(Request{WorkDir: repo, Index: 7, Body: &emptyBody})
	if err != nil {
		t.Fatalf("PRModify clear body: %v", err)
	}
	if resp.PR == nil || resp.PR.Title != "old title" || resp.PR.Body != "" {
		t.Fatalf("clear response = %+v", resp.PR)
	}
	want := [][2]string{{"new title", "old body"}, {"old title", ""}}
	if !reflect.DeepEqual(edits, want) {
		t.Fatalf("provider edits = %#v, want %#v", edits, want)
	}
}

func TestPRModifyRejectsInvalidProviderResult(t *testing.T) {
	tests := []struct {
		name string
		edit func(index int64, title, body string) *gitprovider.PullRequest
		want string
	}{
		{name: "PR ID", want: "PR ID", edit: func(index int64, title, body string) *gitprovider.PullRequest {
			return &gitprovider.PullRequest{Index: 99, Title: title, Body: body}
		}},
		{name: "title", want: "title", edit: func(index int64, title, body string) *gitprovider.PullRequest {
			return &gitprovider.PullRequest{Index: index, Title: "stale", Body: body}
		}},
		{name: "body", want: "body", edit: func(index int64, title, body string) *gitprovider.PullRequest {
			return &gitprovider.PullRequest{Index: index, Title: title, Body: "stale"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/x")
			title, body := "new title", "new body"
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
						return &gitprovider.PullRequest{Index: index, Title: "old", Body: "old body"}, nil
					},
					editPR: func(owner, repo string, index int64, gotTitle, gotBody string) (*gitprovider.PullRequest, error) {
						return tt.edit(index, gotTitle, gotBody), nil
					},
				}, nil
			})
			defer restoreProvider()

			_, err := NewService(&recordingBroker{}).PRModify(Request{
				WorkDir: repo, Index: 7, Title: &title, Body: &body,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PRModify error = %v, want %s validation", err, tt.want)
			}
		})
	}
}

func TestPRCommentReturnsValidatedProviderComment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "checkout", "--detach")
	body := " review this exactly \n"
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		createComment := func(owner, repo string, index int64, gotBody string) (*gitprovider.Comment, error) {
			return &gitprovider.Comment{
				ID: 88, PRID: index, Body: gotBody, User: "reviewer", HTMLURL: "https://comment/88",
			}, nil
		}
		return fakeProvider{createComment: createComment}, nil
	})
	defer restoreProvider()

	resp, err := NewService(&recordingBroker{}).PRComment(Request{WorkDir: repo, Index: 7, Body: &body})
	if err != nil {
		t.Fatalf("PRComment: %v", err)
	}
	if resp.Comment == nil || resp.Comment.ID != 88 || resp.Comment.PRID != 7 ||
		resp.Comment.Body != body || resp.Comment.URL == "" {
		t.Fatalf("comment response = %+v", resp.Comment)
	}
}

func TestPRCommentRejectsInvalidProviderResult(t *testing.T) {
	tests := []struct {
		name    string
		comment *gitprovider.Comment
	}{
		{name: "nil"},
		{name: "missing ID", comment: &gitprovider.Comment{Body: "body", HTMLURL: "https://comment/1"}},
		{name: "mismatched body", comment: &gitprovider.Comment{ID: 1, Body: "other", HTMLURL: "https://comment/1"}},
		{name: "mismatched PR", comment: &gitprovider.Comment{ID: 1, PRID: 9, Body: "body", HTMLURL: "https://comment/1"}},
		{name: "missing URL", comment: &gitprovider.Comment{ID: 1, Body: "body"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/x")
			body := "body"
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				createComment := func(owner, repo string, index int64, gotBody string) (*gitprovider.Comment, error) {
					return tt.comment, nil
				}
				return fakeProvider{createComment: createComment}, nil
			})
			defer restoreProvider()

			_, err := NewService(&recordingBroker{}).PRComment(Request{WorkDir: repo, Index: 7, Body: &body})
			if err == nil || !strings.Contains(err.Error(), "invalid comment") {
				t.Fatalf("PRComment error = %v, want invalid comment", err)
			}
		})
	}
}

func TestExplicitPROperationsValidateIDsAndReturnPRIdentity(t *testing.T) {
	for _, index := range []int64{0, -1} {
		_, err := (Service{}).PRGet(Request{Index: index})
		if err == nil || !strings.Contains(err.Error(), "positive") {
			t.Fatalf("PRGet(%d) error = %v, want positive ID error", index, err)
		}
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")
	gitRun(t, repo, "checkout", "--detach")
	restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				return &gitprovider.PullRequest{Index: index, HeadSHA: "abc123"}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			getCIFailureDetails: func(owner, repo, sha string, tailLines int) ([]*gitprovider.JobFailure, error) {
				return nil, nil
			},
		}, nil
	})
	defer restoreProvider()

	resp, err := NewService(&recordingBroker{}).PRChecks(Request{WorkDir: repo, Index: 12})
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if resp.PR == nil || resp.PR.Index != 12 {
		t.Fatalf("checks response PR = %+v, want #12", resp.PR)
	}
	for name, call := range map[string]func() (Response, error){
		"log": func() (Response, error) {
			return NewService(&recordingBroker{}).PRLog(Request{WorkDir: repo, Index: 12})
		},
		"failures": func() (Response, error) {
			return NewService(&recordingBroker{}).PRFailures(Request{WorkDir: repo, Index: 12})
		},
	} {
		resp, err := call()
		if err != nil {
			t.Fatalf("PR%s: %v", name, err)
		}
		if resp.PR == nil || resp.PR.Index != 12 {
			t.Fatalf("%s response PR = %+v, want #12", name, resp.PR)
		}
	}
}

func TestExplicitPRChecksRejectInvalidProviderSnapshotBeforeCI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/x")

	for _, tt := range []struct {
		name string
		pr   *gitprovider.PullRequest
	}{
		{name: "nil snapshot", pr: nil},
		{name: "wrong ID", pr: &gitprovider.PullRequest{Index: 99, HeadSHA: "wrong"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreProvider := stubNewProvider(t, func(_ *repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
						return tt.pr, nil
					},
					getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
						t.Fatal("CI lookup must not run for an invalid PR snapshot")
						return nil, nil
					},
				}, nil
			})
			defer restoreProvider()

			_, err := NewService(&recordingBroker{}).PRChecks(Request{WorkDir: repo, Index: 12})
			if err == nil || !strings.Contains(err.Error(), "invalid PR") {
				t.Fatalf("PRChecks error = %v, want invalid PR snapshot", err)
			}
		})
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

func ptrTo(value string) *string { return &value }

func stubNewProvider(t *testing.T, fn func(*repoContext) (gitprovider.Provider, error)) func() {
	t.Helper()
	old := newProviderFunc
	newProviderFunc = fn
	return func() { newProviderFunc = old }
}

type fakeProvider struct {
	createPR            func(owner, repo, head, base, title, body string) (*gitprovider.PullRequest, error)
	findPRByState       func(owner, repo, head, base, state string) (*gitprovider.PullRequest, error)
	editPR              func(owner, repo string, index int64, title, body string) (*gitprovider.PullRequest, error)
	getPR               func(owner, repo string, index int64) (*gitprovider.PullRequest, error)
	createComment       func(owner, repo string, index int64, body string) (*gitprovider.Comment, error)
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
	if p.editPR != nil {
		return p.editPR(owner, repo, index, title, body)
	}
	panic("not implemented")
}

func (p fakeProvider) GetPR(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
	if p.getPR != nil {
		return p.getPR(owner, repo, index)
	}
	panic("not implemented")
}

func (p fakeProvider) CreateComment(owner, repo string, index int64, body string) (*gitprovider.Comment, error) {
	if p.createComment != nil {
		return p.createComment(owner, repo, index, body)
	}
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
