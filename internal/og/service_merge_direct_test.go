package og

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
)

func TestDirectMergeGuardsAndRecovery(t *testing.T) { //nolint:gocyclo
	for _, tc := range []struct {
		name, ci, state, want                                                                 string
		dry, changed, dirty, merged, uncertain, cleanupFailure, ciError, nilCI, initialOutage bool
	}{
		{name: "success", ci: "success", want: "executed"},
		{name: "no checks", ci: "not_configured", want: "executed"},
		{name: "initial fetch outage", initialOutage: true, want: "blocked"},
		{name: "pending", ci: "pending", want: "blocked"},
		{name: "unknown", ci: "unknown", want: "blocked"},
		{name: "missing", want: "blocked"},
		{name: "nil CI", nilCI: true, want: "blocked"},
		{name: "unreadable CI", ciError: true, want: "blocked"},
		{name: "failure", ci: "failure", want: "failed"},
		{name: "error", ci: "error", want: "failed"},
		{name: "nonmergeable", ci: "success", want: "failed"},
		{name: "closed", state: "closed", want: "failed"},
		{name: "unknown state", state: "unknown", want: "blocked"},
		{name: "changed head", changed: true, want: "failed"},
		{name: "unsafe checkout", dirty: true, ci: "success", want: "blocked"},
		{name: "dry run", dry: true, ci: "failure", want: "executed"},
		{name: "already merged", merged: true, want: "executed"},
		{name: "uncertain response", uncertain: true, ci: "success", want: "executed"},
		{name: "partial cleanup", cleanupFailure: true, ci: "success", want: "executed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredRepo(t, home, "feature/merge", "https://github.com/tta-lab/example.git", false)
			gitRun(t, repo, "branch", branchMain)
			sha := gitOut(t, repo, "rev-parse", "HEAD")
			gitRun(t, repo, "update-ref", "refs/remotes/origin/main", sha)
			gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", sha)
			if tc.dirty {
				if err := os.WriteFile(repo+"/dirty.txt", []byte("dirty"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			gets, merges, gitCalls, pulls, impriCalls := 0, 0, 0, 0, 0
			merged := tc.merged
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				impriCalls++
				w.WriteHeader(500)
			}))
			t.Cleanup(server.Close)
			restore := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
				return fakeProvider{
					getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
						gets++
						if tc.initialOutage && gets == 1 {
							return nil, errors.New("initial PR outage")
						}
						headSHA := sha
						if tc.changed && gets > 1 {
							headSHA = "changed"
						}
						state := tc.state
						if state == "" {
							state = "open"
						}
						if merged {
							state = "closed"
						}
						return &gitprovider.PullRequest{Index: index, Head: "feature/merge", HeadSHA: headSHA, Base: branchMain,
							State: state, Merged: merged, Mergeable: tc.name != "nonmergeable",
							HTMLURL: "https://github.com/tta-lab/example/pull/7"}, nil
					},
					getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
						if ref != sha {
							t.Fatalf("CI head %s", ref)
						}
						if tc.ciError {
							return nil, errors.New("CI outage")
						}
						if tc.nilCI {
							return nil, nil
						}
						return &gitprovider.CombinedStatus{State: tc.ci}, nil
					},
					mergePR: func(owner, repo string, index int64, headSHA string) error {
						merges++
						if index != 7 || headSHA != sha {
							t.Fatalf("merge identity %d %s", index, headSHA)
						}
						merged = true
						if tc.uncertain {
							return errors.New("lost response")
						}
						return nil
					},
				}, nil
			})
			t.Cleanup(restore)
			restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
				gitCalls++
				if args[0] == "pull" {
					pulls++
					if tc.cleanupFailure && pulls == 1 {
						return errors.New("pull outage")
					}
				}
				if len(args) > 2 && args[0] == "push" && args[2] == "--delete" {
					gitRun(t, repo, "update-ref", "-d", "refs/remotes/origin/feature/merge")
				}
				return nil
			})
			t.Cleanup(restoreGit)
			cfg := ogconfig.Config{Merge: ogconfig.MergeConfig{Approval: "none"}}
			// Both omission and a configured but unused Impri endpoint must work.
			if tc.name == "success" {
				cfg.Impri = &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "fixture"}
			}
			service := NewServiceWithConfig(&recordingBroker{}, nil, cfg)
			req := Request{WorkDir: repo, Index: 7, DryRun: tc.dry, Wait: true, Timeout: time.Millisecond}
			response, err := service.PRMerge(req)
			if response.Merge == nil || response.Merge.Status != tc.want {
				t.Fatalf("result %+v error %v", response, err)
			}
			if tc.want == "blocked" && err == nil {
				t.Fatal("missing retry error")
			}
			if err != nil && tc.want != "blocked" && !tc.cleanupFailure {
				t.Fatal(err)
			}
			checkDirectResult(t, response)
			if tc.initialOutage {
				known := PRMergeSnapshot{PRNumber: 7, PRURL: "https://github.com/tta-lab/example/pull/7",
					ExecutionMode: PRMergeModeReal}
				if response.Merge.Snapshot != known || gets != 1 || gitCalls != 0 {
					t.Fatalf("initial outage result %+v, fetches %d, Git calls %d", response.Merge, gets, gitCalls)
				}
			}
			if tc.want != "executed" || tc.dry || tc.merged {
				if merges != 0 {
					t.Fatalf("unexpected merges %d", merges)
				}
			} else if merges != 1 {
				t.Fatalf("merges %d", merges)
			}
			if tc.dry {
				if gitCalls != 0 || gitOut(t, repo, "branch", "--show-current") != "feature/merge" {
					t.Fatal("dry run mutated checkout")
				}
				if response.Merge.Completion !=
					"the dry-run mock completed; the forge is unchanged; a subsequent real request can merge" {
					t.Fatalf("dry-run completion %q", response.Merge.Completion)
				}
				tc.ci = gitprovider.StateSuccess
				realReq := req
				realReq.DryRun = false
				realResponse, realErr := service.PRMerge(realReq)
				if realErr != nil || realResponse.Merge == nil || realResponse.Merge.Status != PRMergeStatusExecuted ||
					merges != 1 {
					t.Fatalf("real request after dry-run %+v, error %v, merges %d", realResponse, realErr, merges)
				}
				checkDirectResult(t, realResponse)
			}
			if tc.cleanupFailure {
				if response.Merge.CleanupError == "" || !response.Merge.Retryable {
					t.Fatal("missing cleanup retry")
				}
				response, err = service.PRMerge(req)
				if err != nil || response.Merge.CleanupError != "" {
					t.Fatalf("cleanup retry %+v %v", response, err)
				}
				checkDirectResult(t, response)
			}
			if tc.want == "executed" && !tc.dry {
				response, err = service.PRMerge(req)
				if err != nil {
					t.Fatal(err)
				}
				checkDirectResult(t, response)
				if gitOut(t, repo, "branch", "--show-current") != branchMain {
					t.Fatal("default branch not checked out")
				}
				if got := gitOut(t, repo, "branch", "--list", "feature/merge"); got != "" {
					t.Fatalf("head remains %s", got)
				}
				wantMerges := 1
				if tc.merged {
					wantMerges = 0
				}
				if merges != wantMerges {
					t.Fatal("merged twice")
				}
			}
			if impriCalls != 0 {
				t.Fatalf("Impri requests %d", impriCalls)
			}
		})
	}
}

func checkDirectResult(t *testing.T, response Response) {
	t.Helper()
	if err := ValidatePRMergeResponse(response, 7); err != nil {
		t.Fatal(err)
	}
	result := response.Merge
	if result.ApprovalPolicy != "none" || result.ActionID != "" || result.InboxURL != "" || result.ReceiptError != "" {
		t.Fatalf("approval metadata %+v", result)
	}
	for _, value := range []string{result.Detail, result.Completion} {
		if strings.Contains(value, "Impri") || strings.Contains(value, "approved") || strings.Contains(value, "receipt") {
			t.Fatalf("misleading direct outcome %s", value)
		}
	}
}
