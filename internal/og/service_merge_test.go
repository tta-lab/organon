package og

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
)

func TestPRMergeDryRunCreatesResumesAndReportsApproval(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")

	var status atomic.Value
	status.Store(PRMergeStatusPending)
	var posted map[string]any
	var resultReports int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/actions" && r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Fatal(err)
			}
			writeMergeAction(t, w, "act-dry-run", status.Load().(string), posted["payload"])
			return
		}
		if r.URL.Path == "/v1/actions/act-dry-run" && r.Method == http.MethodGet {
			writeMergeAction(t, w, "act-dry-run", status.Load().(string), posted["payload"])
			return
		}
		if r.URL.Path == "/v1/actions/act-dry-run/result" && r.Method == http.MethodPost {
			atomic.AddInt32(&resultReports, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
			return &gitprovider.PullRequest{
				Index: index, State: "open", Head: "feature/merge", HeadSHA: "abc123",
				Base: "main", HTMLURL: "https://github.com/tta-lab/organon/pull/7",
			}, nil
		}}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })

	var dryRuns int32
	service := NewServiceWithConfigAndDryRunExecutor(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	}, func(_ context.Context, snapshot PRMergeSnapshot) error {
		if snapshot.ExecutionMode != PRMergeModeDryRun {
			t.Fatalf("snapshot mode = %q", snapshot.ExecutionMode)
		}
		atomic.AddInt32(&dryRuns, 1)
		return nil
	})

	pending, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil {
		t.Fatalf("create dry-run action: %v", err)
	}
	if pending.Merge == nil || pending.Merge.Status != PRMergeStatusPending || pending.Merge.ActionID != "act-dry-run" {
		t.Fatalf("pending response = %+v", pending)
	}
	if posted["kind"] != PRMergeKind {
		t.Fatalf("posted kind = %#v", posted["kind"])
	}
	payload, ok := posted["payload"].(map[string]any)
	if !ok || payload["execution_mode"] != PRMergeModeDryRun || payload["merge_method"] != PRMergeMethodSquash {
		t.Fatalf("posted payload = %#v", posted["payload"])
	}
	if strings.Contains(pending.Message, "im_test") || strings.Contains(pending.Merge.Detail, "im_test") {
		t.Fatal("API key appeared in result")
	}
	timedOut, err := service.PRMerge(Request{
		WorkDir: repo, Index: 7, DryRun: true, Wait: true, Timeout: 10 * time.Millisecond,
	})
	if err != nil || timedOut.Merge == nil || timedOut.Merge.Status != PRMergeStatusPending ||
		!strings.Contains(timedOut.Merge.Detail, "timed out") {
		t.Fatalf("timeout response = %+v, err = %v", timedOut, err)
	}
	if atomic.LoadInt32(&dryRuns) != 0 {
		t.Fatalf("dry runs after timeout = %d, want 0", dryRuns)
	}
	status.Store(PRMergeStatusRejected)
	rejected, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true, ActionID: "act-dry-run"})
	if err != nil || rejected.Merge == nil || rejected.Merge.Status != PRMergeStatusRejected {
		t.Fatalf("rejected response = %+v, err = %v", rejected, err)
	}
	if atomic.LoadInt32(&dryRuns) != 0 {
		t.Fatalf("dry runs after rejection = %d, want 0", dryRuns)
	}

	status.Store(PRMergeStatusApproved)
	executed, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true, ActionID: "act-dry-run"})
	if err != nil {
		t.Fatalf("resume dry-run action: %v", err)
	}
	if executed.Merge == nil || executed.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("executed response = %+v", executed)
	}
	if atomic.LoadInt32(&dryRuns) != 1 || atomic.LoadInt32(&resultReports) != 1 {
		t.Fatalf("dry runs = %d, reports = %d", dryRuns, resultReports)
	}
}

func TestPRMergeRealMergeUsesGuardsAndRepairsReceiptWithoutMergingTwice(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var actionPayload any
	var merged atomic.Bool
	var mergeCalls atomic.Int32
	var resultReports atomic.Int32
	var failReceipt atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actionPayload = body["payload"]
			writeMergeAction(t, w, "act-real", PRMergeStatusPending, actionPayload)
		case r.URL.Path == "/v1/actions/act-real" && r.Method == http.MethodGet:
			writeMergeAction(t, w, "act-real", PRMergeStatusApproved, actionPayload)
		case r.URL.Path == "/v1/actions/act-real/result" && r.Method == http.MethodPost:
			resultReports.Add(1)
			if failReceipt.Load() {
				http.Error(w, "temporary receipt outage", http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				state := "open"
				if merged.Load() {
					state = "closed"
				}
				return &gitprovider.PullRequest{
					Index: index, State: state, Merged: merged.Load(), Mergeable: true,
					Head: "feature/merge", HeadSHA: "abc123", Base: "main",
					HTMLURL: "https://github.com/tta-lab/organon/pull/7",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				if ref != "abc123" {
					t.Fatalf("CI ref = %q", ref)
				}
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				if index != 7 || headSHA != "abc123" {
					t.Fatalf("merge identity = %s/%s #%d %s", owner, repo, index, headSHA)
				}
				mergeCalls.Add(1)
				merged.Store(true)
				return nil
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	pending, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || pending.Merge == nil || pending.Merge.Status != PRMergeStatusPending {
		t.Fatalf("create response = %+v, err = %v", pending, err)
	}
	failReceipt.Store(true)
	mergedResponse, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-real"})
	if err != nil {
		t.Fatalf("approved merge with receipt failure: %v", err)
	}
	if mergedResponse.Merge == nil || mergedResponse.Merge.Status != PRMergeStatusExecuted ||
		mergedResponse.Merge.ReceiptError == "" {
		t.Fatalf("merged response = %+v", mergedResponse)
	}
	if mergeCalls.Load() != 1 {
		t.Fatalf("merge calls = %d, want 1", mergeCalls.Load())
	}
	failReceipt.Store(false)
	repaired, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-real"})
	if err != nil || repaired.Merge == nil || repaired.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("receipt repair response = %+v, err = %v", repaired, err)
	}
	if mergeCalls.Load() != 1 || resultReports.Load() != 2 {
		t.Fatalf("merge calls = %d, result reports = %d", mergeCalls.Load(), resultReports.Load())
	}
}

func TestPRMergeRejectsActionIDMismatchBeforeExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/actions/act-requested" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		writeMergeAction(t, w, "act-returned", PRMergeStatusPending, map[string]any{})
	}))
	t.Cleanup(server.Close)

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	_, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true, ActionID: "act-requested"})
	if err == nil || !strings.Contains(err.Error(), "action ID") {
		t.Fatalf("PRMerge error = %v, want action ID mismatch", err)
	}
}

func TestPRMergeApprovedRealGuardsFailClosed(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name       string
		mutate     func(*gitprovider.PullRequest)
		ciState    string
		mergeErr   error
		wantMerge  int
		wantDetail string
	}{
		{
			name: "stale head SHA", mutate: func(pr *gitprovider.PullRequest) { pr.HeadSHA = "new-sha" },
			wantDetail: "identity changed",
		},
		{
			name: "closed PR", mutate: func(pr *gitprovider.PullRequest) { pr.State = "closed" },
			wantDetail: "not open",
		},
		{
			name: "non-mergeable PR", mutate: func(pr *gitprovider.PullRequest) { pr.Mergeable = false },
			wantDetail: "not mergeable",
		},
		{
			name: "non-green CI", ciState: gitprovider.StateFailure,
			wantDetail: "CI is not green",
		},
		{
			name: "forge failure", mergeErr: errors.New("head moved"), wantMerge: 1,
			wantDetail: "forge squash merge failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, mergeCalls := runApprovedRealMergeScenario(t, tt.mutate, tt.ciState, tt.mergeErr)
			if response.Merge == nil || response.Merge.Status != PRMergeStatusExecuteFailed ||
				!strings.Contains(response.Merge.Detail, tt.wantDetail) {
				t.Fatalf("response = %+v, want execute_failed containing %q", response, tt.wantDetail)
			}
			if mergeCalls != tt.wantMerge {
				t.Fatalf("merge calls = %d, want %d", mergeCalls, tt.wantMerge)
			}
		})
	}
}

func runApprovedRealMergeScenario(
	t *testing.T, mutate func(*gitprovider.PullRequest), ciState string, mergeErr error,
) (Response, int) {
	t.Helper()
	if ciState == "" {
		ciState = gitprovider.StateSuccess
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var actionPayload any
	var mergeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actionPayload = body["payload"]
			writeMergeAction(t, w, "act-guard", PRMergeStatusPending, actionPayload)
		case r.URL.Path == "/v1/actions/act-guard" && r.Method == http.MethodGet:
			writeMergeAction(t, w, "act-guard", PRMergeStatusApproved, actionPayload)
		case r.URL.Path == "/v1/actions/act-guard/result" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	oldProvider := newProviderFunc
	providerCalls := 0
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				providerCalls++
				pr := &gitprovider.PullRequest{
					Index: index, State: "open", Mergeable: true,
					Head: "feature/merge", HeadSHA: "abc123", Base: "main",
					HTMLURL: "https://github.com/tta-lab/organon/pull/7",
				}
				if providerCalls > 1 && mutate != nil {
					mutate(pr)
				}
				return pr, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: ciState}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				mergeCalls++
				return mergeErr
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-guard"})
	if err != nil {
		t.Fatalf("resume approved action: %v", err)
	}
	return response, mergeCalls
}

func writeMergeAction(t *testing.T, w http.ResponseWriter, id, status string, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": id, "status": status, "inbox_url": "http://impri.example/actions",
		"payload": payload,
	})
}
