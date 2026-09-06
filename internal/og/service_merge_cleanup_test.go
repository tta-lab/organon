package og

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
)

type mergeActionServerState struct {
	payload       any
	actionGets    atomic.Int32
	resultReports atomic.Int32
}

func newMergeActionServer(
	t *testing.T, actionID string, status func(gets, reports int32) string,
) (*httptest.Server, *mergeActionServerState) {
	t.Helper()
	state := &mergeActionServerState{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			state.payload = body["payload"]
			writeMergeAction(t, w, actionID, status(state.actionGets.Load(), state.resultReports.Load()), state.payload)
		case r.URL.Path == "/v1/actions/"+actionID && r.Method == http.MethodGet:
			state.actionGets.Add(1)
			writeMergeAction(t, w, actionID, status(state.actionGets.Load(), state.resultReports.Load()), state.payload)
		case r.URL.Path == "/v1/actions/"+actionID+"/result" && r.Method == http.MethodPost:
			state.resultReports.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, state
}

func TestPRMergeRealMergeRejectsUnsafeCleanupTargetsBeforeForge(t *testing.T) { //nolint:dupl,gocyclo
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, repo string)
	}{
		{name: "dirty worktree", mutate: func(t *testing.T, repo string) {
			t.Helper()
			if err := os.WriteFile(repo+"/dirty.txt", []byte("not committed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unrelated current branch", mutate: func(t *testing.T, repo string) {
			t.Helper()
			gitRun(t, repo, "switch", "-c", "other")
		}},
		{name: "moved local head", mutate: func(t *testing.T, repo string) {
			t.Helper()
			gitRun(t, repo, "commit", "--allow-empty", "-m", "moved")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredRepo(t, home, "feature/merge", "https://github.com/tta-lab/example.git", false)
			gitRun(t, repo, "branch", branchMain)
			mainSHA := gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
			gitRun(t, repo, "update-ref", "refs/remotes/origin/"+branchMain, mainSHA)
			gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
			gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", featureSHA)
			test.mutate(t, repo)

			var mergeCalls atomic.Int32
			server, actionState := newMergeActionServer(t, "act-unsafe", func(gets, reports int32) string {
				if gets <= 1 {
					return PRMergeStatusPending
				}
				return PRMergeStatusApproved
			})
			t.Cleanup(server.Close)
			restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
						return &gitprovider.PullRequest{
							Index: index, State: "open", Mergeable: true, Head: "feature/merge",
							HeadSHA: featureSHA, Base: branchMain,
							HTMLURL: "https://github.com/tta-lab/example/pull/7",
						}, nil
					},
					getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
						return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
					},
					mergePR: func(owner, repo string, index int64, headSHA string) error {
						mergeCalls.Add(1)
						return nil
					},
				}, nil
			})
			t.Cleanup(restoreProvider)
			restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, _ ...string) error { return nil })
			t.Cleanup(restoreGit)

			service := NewServiceWithConfig(&recordingBroker{}, nil, ogconfig.Config{
				Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
			})
			if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
				t.Fatalf("create merge action: %v", err)
			}
			response, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
			var retryErr *PRMergeRetryableError
			if err == nil || !errors.As(err, &retryErr) || response.Merge == nil ||
				!response.Merge.Retryable || response.Merge.NextAction != PRMergeNextRetry ||
				!strings.Contains(response.Merge.Detail, "remains approved") {
				t.Fatalf("unsafe target response = %+v, err = %v", response, err)
			}
			if mergeCalls.Load() != 0 || actionState.resultReports.Load() != 0 {
				t.Fatalf(
					"unsafe target reached forge/receipt: merges=%d reports=%d",
					mergeCalls.Load(), actionState.resultReports.Load(),
				)
			}
		})
	}
}

func TestPRMergeRealMergeRetriesPostMergeCleanupWithoutSecondForgeCall(t *testing.T) { //nolint:dupl,gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, "feature/merge", "https://github.com/tta-lab/example.git", false)
	gitRun(t, repo, "branch", branchMain)
	mainSHA := gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
	gitRun(t, repo, "update-ref", "refs/remotes/origin/"+branchMain, mainSHA)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", featureSHA)

	var actionPayload any
	var actionGets atomic.Int32
	var mergeCalls atomic.Int32
	var resultReports atomic.Int32
	var merged atomic.Bool
	var cleanupPullFailures atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actionPayload = body["payload"]
			status := PRMergeStatusPending
			if actionGets.Load() > 1 {
				status = PRMergeStatusExecuted
			}
			writeMergeAction(t, w, "act-retry-clean", status, actionPayload)
		case r.URL.Path == "/v1/actions/act-retry-clean" && r.Method == http.MethodGet:
			status := PRMergeStatusApproved
			if actionGets.Add(1) == 1 {
				status = PRMergeStatusPending
			} else if resultReports.Load() > 0 {
				status = PRMergeStatusExecuted
			}
			writeMergeAction(t, w, "act-retry-clean", status, actionPayload)
		case r.URL.Path == "/v1/actions/act-retry-clean/result" && r.Method == http.MethodPost:
			resultReports.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				state := "open"
				if merged.Load() {
					state = "closed"
				}
				return &gitprovider.PullRequest{
					Index: index, State: state, Merged: merged.Load(), Mergeable: true,
					Head: "feature/merge", HeadSHA: featureSHA, Base: branchMain,
					HTMLURL: "https://github.com/tta-lab/example/pull/7",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				mergeCalls.Add(1)
				merged.Store(true)
				return nil
			},
		}, nil
	})
	t.Cleanup(restoreProvider)
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		if len(args) >= 2 && args[0] == "pull" && cleanupPullFailures.Add(1) == 1 {
			return errors.New("temporary pull outage")
		}
		if len(args) >= 3 && args[0] == "push" && args[1] == remoteOrigin && args[2] == "--delete" {
			gitRun(t, repo, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	})
	t.Cleanup(restoreGit)

	service := NewServiceWithConfig(&recordingBroker{}, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create merge action: %v", err)
	}
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || first.Merge == nil ||
		first.Merge.Status != PRMergeStatusExecuted || !first.Merge.Retryable ||
		first.Merge.NextAction != PRMergeNextRetry || first.Merge.ReceiptError != "" ||
		first.Merge.CleanupError == "" || !strings.Contains(first.Merge.Detail, "must not run again") {
		t.Fatalf("partial cleanup response = %+v, err = %v", first, err)
	}
	if mergeCalls.Load() != 1 || resultReports.Load() != 1 {
		t.Fatalf("after partial cleanup: merges=%d reports=%d", mergeCalls.Load(), resultReports.Load())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted ||
		second.Merge.Retryable || second.Merge.NextAction != PRMergeNextNone ||
		second.Merge.CleanupError != "" {
		t.Fatalf("repaired cleanup response = %+v, err = %v", second, err)
	}
	if mergeCalls.Load() != 1 || resultReports.Load() != 1 {
		t.Fatalf("after cleanup retry: merges=%d reports=%d", mergeCalls.Load(), resultReports.Load())
	}
}

func TestPRMergeRealMergeCleansApprovedCheckout(t *testing.T) { //nolint:dupl,gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredRepo(t, home, "feature/merge", "https://github.com/tta-lab/example.git", false)
	gitRun(t, repo, "branch", branchMain)
	mainSHA := gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
	gitRun(t, repo, "update-ref", "refs/remotes/origin/"+branchMain, mainSHA)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", featureSHA)

	var mergeCalls atomic.Int32
	var merged atomic.Bool
	var gitCalls [][]string
	var postMergeSHA string
	server, actionState := newMergeActionServer(t, "act-clean", func(gets, reports int32) string {
		if reports > 0 {
			return PRMergeStatusExecuted
		}
		if gets <= 1 {
			return PRMergeStatusPending
		}
		return PRMergeStatusApproved
	})
	t.Cleanup(server.Close)

	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				state := "open"
				if merged.Load() {
					state = "closed"
				}
				return &gitprovider.PullRequest{
					Index: index, State: state, Merged: merged.Load(), Mergeable: true,
					Head: "feature/merge", HeadSHA: featureSHA, Base: branchMain,
					HTMLURL: "https://github.com/tta-lab/example/pull/7",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				if ref != featureSHA {
					t.Fatalf("CI ref = %q, want %q", ref, featureSHA)
				}
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				mergeCalls.Add(1)
				if headSHA != featureSHA {
					t.Fatalf("merge SHA = %q, want %q", headSHA, featureSHA)
				}
				merged.Store(true)
				return nil
			},
		}, nil
	})
	t.Cleanup(restoreProvider)
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		gitCalls = append(gitCalls, append([]string(nil), args...))
		if len(args) == 4 && args[0] == "pull" && args[1] == "--ff-only" &&
			args[2] == remoteOrigin && args[3] == branchMain && postMergeSHA == "" {
			gitRun(t, repo, "commit", "--allow-empty", "-m", "post-merge")
			postMergeSHA = gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
		}
		if len(args) >= 3 && args[0] == "push" && args[1] == remoteOrigin && args[2] == "--delete" {
			gitRun(t, repo, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	})
	t.Cleanup(restoreGit)

	service := NewServiceWithConfig(&recordingBroker{}, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create merge action: %v", err)
	}
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || response.Merge == nil {
		t.Fatalf("merge response = %+v, err = %v", response, err)
	}
	if response.Merge.Status != PRMergeStatusExecuted || response.Merge.Retryable ||
		response.Merge.NextAction != PRMergeNextNone || response.Merge.CleanupError != "" {
		t.Fatalf("merge result = %+v", response.Merge)
	}
	if mergeCalls.Load() != 1 || actionState.resultReports.Load() != 1 {
		t.Fatalf("merge calls = %d, result reports = %d", mergeCalls.Load(), actionState.resultReports.Load())
	}
	if got := gitOut(t, repo, "branch", "--show-current"); got != branchMain {
		t.Fatalf("current branch = %q, want %q", got, branchMain)
	}
	if err := gitCmd(repo, "rev-parse", "--verify", "refs/heads/feature/merge"); err == nil {
		t.Fatal("approved local head still exists")
	}
	if err := gitCmd(repo, "show-ref", "--verify", "--quiet", "refs/remotes/origin/feature/merge"); err == nil {
		t.Fatal("approved remote-tracking head still exists")
	}
	defaultSHA := gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
	if postMergeSHA == "" || defaultSHA != postMergeSHA {
		t.Fatalf("default branch SHA = %q, want post-merge SHA %q", defaultSHA, postMergeSHA)
	}
	lease := "--force-with-lease=refs/heads/feature/merge:" + featureSHA
	if len(gitCalls) != 4 || strings.Join(gitCalls[2], " ") != "pull --ff-only origin main" ||
		!strings.Contains(strings.Join(gitCalls[3], " "), lease) {
		t.Fatalf("credentialed git calls = %v", gitCalls)
	}
	replayed, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || replayed.Merge == nil || replayed.Merge.Status != PRMergeStatusExecuted ||
		replayed.Merge.Retryable || replayed.Merge.CleanupError != "" {
		t.Fatalf("already-cleaned replay response = %+v, err = %v", replayed, err)
	}
	if mergeCalls.Load() != 1 || actionState.resultReports.Load() != 1 {
		t.Fatalf("already-cleaned replay calls: merges=%d reports=%d", mergeCalls.Load(), actionState.resultReports.Load())
	}
}
