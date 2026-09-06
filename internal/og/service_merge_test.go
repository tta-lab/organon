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

func TestPRMergeDryRunCreatesPollsAndReportsApproval(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	checkoutBranch := gitOut(t, repo, "branch", "--show-current")
	var credentialedGitCalls [][]string
	restoreGit := stubRunGitWithCreds(t, func(_ *repoContext, args ...string) error {
		credentialedGitCalls = append(credentialedGitCalls, append([]string(nil), args...))
		return nil
	})
	t.Cleanup(restoreGit)

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
	if pending.Merge == nil || pending.Merge.Status != PRMergeStatusPending || pending.Merge.ActionID != "act-dry-run" ||
		pending.Merge.Retryable || pending.Merge.NextAction != PRMergeNextWait ||
		pending.Merge.Completion == "" {
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
		timedOut.Merge.NextAction != PRMergeNextWait ||
		!strings.Contains(timedOut.Merge.Detail, "timed out") {
		t.Fatalf("timeout response = %+v, err = %v", timedOut, err)
	}
	if atomic.LoadInt32(&dryRuns) != 0 {
		t.Fatalf("dry runs after timeout = %d, want 0", dryRuns)
	}
	status.Store(PRMergeStatusRejected)
	rejected, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil || rejected.Merge == nil || rejected.Merge.Status != PRMergeStatusRejected ||
		rejected.Merge.Retryable || rejected.Merge.NextAction != PRMergeNextNewApproval {
		t.Fatalf("rejected response = %+v, err = %v", rejected, err)
	}
	if atomic.LoadInt32(&dryRuns) != 0 {
		t.Fatalf("dry runs after rejection = %d, want 0", dryRuns)
	}

	status.Store(PRMergeStatusApproved)
	executed, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil {
		t.Fatalf("repeat dry-run request: %v", err)
	}
	if executed.Merge == nil || executed.Merge.Status != PRMergeStatusExecuted ||
		executed.Merge.Retryable || executed.Merge.NextAction != PRMergeNextNone {
		t.Fatalf("executed response = %+v", executed)
	}
	if atomic.LoadInt32(&dryRuns) != 1 || atomic.LoadInt32(&resultReports) != 1 {
		t.Fatalf("dry runs = %d, reports = %d", dryRuns, resultReports)
	}
	if got := gitOut(t, repo, "branch", "--show-current"); got != checkoutBranch {
		t.Fatalf("dry-run checkout branch = %q, want %q", got, checkoutBranch)
	}
	if len(credentialedGitCalls) != 0 {
		t.Fatalf("dry-run credentialed Git cleanup calls = %v, want none", credentialedGitCalls)
	}
}

func TestPRMergeCreateIncludesNormalizedTitleAndSafePreview(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name          string
		prTitle       string
		wantCardTitle string
		wantPreview   string
	}{
		{
			name:          "normal",
			prTitle:       "Add structured merge logging",
			wantCardTitle: "Approve squash merge: tta-lab/example#7 — Add structured merge logging",
			wantPreview:   "- PR title: ` Add structured merge logging `",
		},
		{
			name:          "newline markdown and backticks",
			prTitle:       "Fix\n**approval** [link](https://evil.example) and `ticks`",
			wantCardTitle: "Approve squash merge: tta-lab/example#7 — Fix **approval** [link](https://evil.example) and `ticks`",
			wantPreview:   "- PR title: `` Fix **approval** [link](https://evil.example) and `ticks` ``",
		},
		{
			name:    "length boundary",
			prTitle: strings.Repeat("x", 600),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/merge")
			var posted map[string]any
			var payload any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
					if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
						t.Fatal(err)
					}
					payload = posted["payload"]
					writeMergeAction(t, w, "act-title", PRMergeStatusPending, payload)
				case r.URL.Path == "/v1/actions/act-title" && r.Method == http.MethodGet:
					writeMergeAction(t, w, "act-title", PRMergeStatusPending, payload)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
				return fakeProvider{getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
					return &gitprovider.PullRequest{
						Index: index, Title: tt.prTitle, State: "open", Mergeable: true,
						Head: "feature/merge", HeadSHA: "abc123", Base: "main",
						HTMLURL: "https://github.com/tta-lab/example/pull/7",
					}, nil
				}}, nil
			})
			t.Cleanup(restoreProvider)
			service := NewServiceWithConfig(nil, nil, ogconfig.Config{
				Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
			})
			response, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
			if err != nil || response.Merge == nil {
				t.Fatalf("response = %+v, err = %v", response, err)
			}
			cardTitle, ok := posted["title"].(string)
			if !ok || strings.ContainsAny(cardTitle, "\r\n\u0085\u2028\u2029") ||
				len([]rune(cardTitle)) == 0 || len([]rune(cardTitle)) > 500 {
				t.Fatalf("card title = %q, want one line with 1..500 characters", cardTitle)
			}
			if tt.wantCardTitle != "" && cardTitle != tt.wantCardTitle {
				t.Fatalf("card title = %q, want %q", cardTitle, tt.wantCardTitle)
			}
			if tt.name == "length boundary" && len([]rune(cardTitle)) != 500 {
				t.Fatalf("card title length = %d, want exactly 500", len([]rune(cardTitle)))
			}
			postedPayload, ok := posted["payload"].(map[string]any)
			if !ok || postedPayload["title"] != tt.prTitle {
				t.Fatalf("posted title audit data = %#v, want %q", postedPayload["title"], tt.prTitle)
			}
			preview, ok := posted["preview"].(map[string]any)
			if !ok {
				t.Fatalf("preview = %#v, want object", posted["preview"])
			}
			previewBody, ok := preview["body"].(string)
			if !ok || (tt.wantPreview != "" && !strings.Contains(previewBody, tt.wantPreview)) {
				t.Fatalf("preview body = %q, want safe title line containing %q", previewBody, tt.wantPreview)
			}
			if response.Merge.Snapshot.Title != tt.prTitle {
				t.Fatalf("response snapshot title = %q, want %q", response.Merge.Snapshot.Title, tt.prTitle)
			}
		})
	}
}

func TestPRMergeRepeatingRequestPreservesApprovalTitleInResult(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	prTitle := "Keep\n**approval** `title` in the audit trail"
	var payload map[string]any
	var resultPayload map[string]any
	var actionGets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			payload = body["payload"].(map[string]any)
			writeMergeAction(t, w, "act-title", PRMergeStatusPending, payload)
		case r.URL.Path == "/v1/actions/act-title" && r.Method == http.MethodGet:
			status := PRMergeStatusApproved
			if actionGets == 0 {
				status = PRMergeStatusPending
			}
			actionGets++
			writeMergeAction(t, w, "act-title", status, payload)
		case r.URL.Path == "/v1/actions/act-title/result" && r.Method == http.MethodPost:
			var body struct {
				Payload map[string]any `json:"payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			resultPayload = body.Payload
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
			return &gitprovider.PullRequest{
				Index: index, Title: prTitle, State: "open", Mergeable: true,
				Head: "feature/merge", HeadSHA: "abc123", Base: "main",
				HTMLURL: "https://github.com/tta-lab/example/pull/7",
			}, nil
		}}, nil
	})
	t.Cleanup(restoreProvider)
	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	created, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil || created.Merge == nil || created.Merge.Snapshot.Title != prTitle {
		t.Fatalf("created response = %+v, err = %v", created, err)
	}
	repeated, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil || repeated.Merge == nil || repeated.Merge.Status != PRMergeStatusExecuted ||
		repeated.Merge.Snapshot.Title != prTitle {
		t.Fatalf("repeated request response = %+v, err = %v", repeated, err)
	}
	if payload["title"] != prTitle || resultPayload["title"] != prTitle {
		t.Fatalf("title audit data = posted %#v, result %#v, want %q", payload["title"], resultPayload["title"], prTitle)
	}
}

func TestPRMergeRealMergeUsesGuardsAndRepairsReceiptWithoutMergingTwice(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	featureSHA := setupMergeCleanupRepo(t, repo)
	restoreGit := stubRunGitWithCreds(t, func(ctxInfo *repoContext, args ...string) error {
		if len(args) >= 3 && args[0] == "push" && args[1] == remoteOrigin && args[2] == "--delete" {
			gitRun(t, ctxInfo.WorkDir, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	})
	t.Cleanup(restoreGit)
	var actionPayload any
	var actionGets int
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
			status := PRMergeStatusApproved
			if actionGets == 0 {
				status = PRMergeStatusPending
			}
			actionGets++
			writeMergeAction(t, w, "act-real", status, actionPayload)
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
					Head: "feature/merge", HeadSHA: featureSHA, Base: "main",
					HTMLURL: "https://github.com/tta-lab/organon/pull/7",
				}, nil
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				if ref != featureSHA {
					t.Fatalf("CI ref = %q", ref)
				}
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				if index != 7 || headSHA != featureSHA {
					t.Fatalf("merge identity = %s/%s #%d %s", owner, repo, index, headSHA)
				}
				mergeCalls.Add(1)
				merged.Store(true)
				return nil
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })

	service := NewServiceWithConfig(&recordingBroker{token: "test-token"}, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	pending, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || pending.Merge == nil || pending.Merge.Status != PRMergeStatusPending {
		t.Fatalf("create response = %+v, err = %v", pending, err)
	}
	failReceipt.Store(true)
	mergedResponse, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil {
		t.Fatalf("approved merge with receipt failure: %v", err)
	}
	if mergedResponse.Merge == nil || mergedResponse.Merge.Status != PRMergeStatusExecuted ||
		mergedResponse.Merge.ReceiptError == "" ||
		!mergedResponse.Merge.Retryable || mergedResponse.Merge.NextAction != PRMergeNextRepairReceipt {
		t.Fatalf("merged response = %+v", mergedResponse)
	}
	if mergeCalls.Load() != 1 {
		t.Fatalf("merge calls = %d, want 1", mergeCalls.Load())
	}
	failReceipt.Store(false)
	repaired, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || repaired.Merge == nil || repaired.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("receipt repair response = %+v, err = %v", repaired, err)
	}
	if mergeCalls.Load() != 1 || resultReports.Load() != 2 {
		t.Fatalf("merge calls = %d, result reports = %d", mergeCalls.Load(), resultReports.Load())
	}
}

func TestPRMergeRejectsWrongTargetOnCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			payload = body["payload"].(map[string]any)
			writeMergeActionWithTarget(t, w, "act-target-create", PRMergeStatusPending,
				payload["pr_url"].(string)+"/wrong", payload)
		case r.URL.Path == "/v1/actions/act-target-create" && r.Method == http.MethodGet:
			writeMergeActionWithTarget(t, w, "act-target-create", PRMergeStatusPending,
				payload["pr_url"].(string)+"/wrong", payload)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: exactMergePR}, nil
	})
	t.Cleanup(restoreProvider)

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	_, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err == nil || !strings.Contains(err.Error(), "target_url") {
		t.Fatalf("PRMerge error = %v, want create target rejection", err)
	}
}

func TestPRMergeCreateReceiptFollowupOutagePreservesRecoveryIdentity(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"created_at": "2026-09-06T00:00:00Z", "expires_at": "2026-09-06T00:05:00Z",
				"id": "act-create-followup", "inbox_url": "https://impri.example/actions",
				"status": PRMergeStatusPending,
			})
		case r.URL.Path == "/v1/actions/act-create-followup" && r.Method == http.MethodGet:
			gets.Add(1)
			http.Error(w, "temporary canonical action outage", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: exactMergePR}, nil
	})
	t.Cleanup(restoreProvider)
	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || response.Merge == nil {
		t.Fatalf("response = %+v, err = %v, want structured retryable outcome", response, err)
	}
	merge := response.Merge
	if merge.Status != PRMergeStatusUnavailable || merge.ActionID != "act-create-followup" ||
		merge.InboxURL != "https://impri.example/actions" ||
		!merge.Retryable || merge.NextAction != PRMergeNextRetry || merge.Snapshot.PRNumber != 7 ||
		merge.Snapshot.PRURL == "" {
		t.Fatalf("create follow-up recovery = %+v", merge)
	}
	if !strings.Contains(merge.Detail, "no forge call was made") || posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("detail = %q, POSTs = %d, GETs = %d", merge.Detail, posts.Load(), gets.Load())
	}
	if err := ValidatePRMergeResponse(response, 7); err != nil {
		t.Fatalf("validate create follow-up response: %v", err)
	}
}

func TestPRMergeCreateIdempotentResponseUsesCanonicalGETOnRepeatedRequest(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var payload any
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			posts.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			payload = body["payload"]
			writeMergeActionWithoutInbox(t, w, "act-idempotent", PRMergeStatusPending, payload)
		case r.URL.Path == "/v1/actions/act-idempotent" && r.Method == http.MethodGet:
			gets.Add(1)
			writeMergeActionWithoutInbox(t, w, "act-idempotent", PRMergeStatusPending, payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	restoreProvider := stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: exactMergePR}, nil
	})
	t.Cleanup(restoreProvider)
	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	created, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil || created.Merge == nil || created.Merge.Status != PRMergeStatusPending {
		t.Fatalf("created response = %+v, err = %v", created, err)
	}
	repeated, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true})
	if err != nil || repeated.Merge == nil || repeated.Merge.Status != PRMergeStatusPending {
		t.Fatalf("repeated request response = %+v, err = %v", repeated, err)
	}
	if created.Merge.InboxURL != server.URL+"/actions" || repeated.Merge.InboxURL != server.URL+"/actions" {
		t.Fatalf("inbox URLs = %q, %q; want client fallback %q", created.Merge.InboxURL,
			repeated.Merge.InboxURL, server.URL+"/actions")
	}
	if posts.Load() != 2 || gets.Load() != 2 {
		t.Fatalf("POSTs = %d, GETs = %d, want two idempotent creates and two canonical reads", posts.Load(), gets.Load())
	}
}

func TestPRMergeReportsUnavailableWhenWaitPollFails(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var payload any
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			payload = body["payload"]
			writeMergeAction(t, w, "act-wait-unavailable", PRMergeStatusPending, payload)
		case r.URL.Path == "/v1/actions/act-wait-unavailable" && r.Method == http.MethodGet:
			if gets.Add(1) == 1 {
				writeMergeAction(t, w, "act-wait-unavailable", PRMergeStatusPending, payload)
				return
			}
			http.Error(w, "temporary Impri poll outage", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{getPR: exactMergePR}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	var dryRuns atomic.Int32
	service := NewServiceWithConfigAndDryRunExecutor(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	}, func(context.Context, PRMergeSnapshot) error {
		dryRuns.Add(1)
		return nil
	})
	response, err := service.PRMerge(Request{
		WorkDir: repo, Index: 7, DryRun: true, Wait: true, Timeout: time.Second,
	})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || response.Merge == nil {
		t.Fatalf("response = %+v, err = %v, want structured retryable outcome", response, err)
	}
	merge := response.Merge
	if merge.Status != PRMergeStatusUnavailable || merge.ActionID != "act-wait-unavailable" ||
		!merge.Retryable || merge.NextAction != PRMergeNextRetry ||
		merge.Snapshot.PRNumber != 7 || merge.Snapshot.PRURL == "" {
		t.Fatalf("unavailable wait merge = %+v", merge)
	}
	if !strings.Contains(merge.Detail, "no forge call was made") || dryRuns.Load() != 0 {
		t.Fatalf("unavailable wait detail = %q, dry runs = %d", merge.Detail, dryRuns.Load())
	}
	if err := ValidatePRMergeResponse(response, 7); err != nil {
		t.Fatalf("validate unavailable wait response: %v", err)
	}
}

func TestPRMergeKeepsApprovedStateWhenExecuteFailedReceiptCannotBeRecorded(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var actionPayload any
	var actionGets int
	var receiptAttempts atomic.Int32
	var receiptStatuses []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actionPayload = body["payload"]
			writeMergeAction(t, w, "act-failure-receipt", PRMergeStatusPending, actionPayload)
		case r.URL.Path == "/v1/actions/act-failure-receipt" && r.Method == http.MethodGet:
			status := PRMergeStatusApproved
			if actionGets == 0 {
				status = PRMergeStatusPending
			}
			actionGets++
			writeMergeAction(t, w, "act-failure-receipt", status, actionPayload)
		case r.URL.Path == "/v1/actions/act-failure-receipt/result" && r.Method == http.MethodPost:
			var body struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			receiptStatuses = append(receiptStatuses, body.Status)
			if receiptAttempts.Add(1) == 1 {
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
		return fakeProvider{getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
			pr, err := exactMergePR(owner, repo, index)
			pr.Mergeable = false
			return pr, err
		}}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || first.Merge == nil ||
		first.Merge.Status != PRMergeStatusApproved ||
		!first.Merge.Retryable || first.Merge.NextAction != PRMergeNextRetry ||
		first.Merge.ReceiptError != "" || !strings.Contains(first.Merge.Detail, "could not be recorded") {
		t.Fatalf("unrecorded failure response = %+v, err = %v", first, err)
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuteFailed ||
		second.Merge.Retryable || second.Merge.NextAction != PRMergeNextNewApproval {
		t.Fatalf("recorded failure response = %+v, err = %v", second, err)
	}
	if len(receiptStatuses) != 2 || receiptStatuses[0] != PRMergeStatusExecuteFailed ||
		receiptStatuses[1] != PRMergeStatusExecuteFailed {
		t.Fatalf("receipt statuses = %v, want two execute_failed attempts", receiptStatuses)
	}
}

//nolint:gocyclo
func TestPRMergeKeepsApprovalOnTemporaryCIFailure(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls := newRetryMergeFixture(t, "act-ci")
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	var ciCalls int
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				pr, err := exactMergePR(owner, repo, index)
				pr.HeadSHA = featureSHA
				return pr, err
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				ciCalls++
				if ciCalls == 1 {
					return nil, errors.New("temporary CI outage")
				}
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				addMergeCall()
				return nil
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err == nil || first.Merge == nil || !first.Merge.Retryable ||
		first.Merge.NextAction != PRMergeNextRetry || !strings.Contains(err.Error(), "CI status") {
		t.Fatalf("first CI response = %+v, err = %v", first, err)
	}
	if resultReports() != 0 || mergeCalls() != 0 {
		t.Fatalf("after temporary CI failure: reports=%d merges=%d", resultReports(), mergeCalls())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("repeated CI request response = %+v, err = %v", second, err)
	}
	if resultReports() != 1 || mergeCalls() != 1 {
		t.Fatalf("after CI recovery: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

//nolint:gocyclo
func TestPRMergeRetriesAmbiguousMergeFailureWithoutConsumingApproval(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls := newRetryMergeFixture(t, "act-merge")
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	var calls int
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				pr, err := exactMergePR(owner, repo, index)
				pr.HeadSHA = featureSHA
				return pr, err
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				calls++
				addMergeCall()
				if calls == 1 {
					return errors.New("ambiguous forge timeout")
				}
				return nil
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err == nil || first.Merge == nil || !first.Merge.Retryable ||
		first.Merge.NextAction != PRMergeNextRetry || !strings.Contains(err.Error(), "ambiguous") ||
		!strings.Contains(err.Error(), "remains approved") {
		t.Fatalf("first merge response = %+v, err = %v", first, err)
	}
	if resultReports() != 0 || mergeCalls() != 1 {
		t.Fatalf("after ambiguous merge failure: reports=%d merges=%d", resultReports(), mergeCalls())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("repeated merge request response = %+v, err = %v", second, err)
	}
	if resultReports() != 1 || mergeCalls() != 2 {
		t.Fatalf("after merge recovery: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

func TestPRMergeRepairsReceiptWhenMergeErrorWasActuallySuccessful(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls := newRetryMergeFixture(t, "act-merged")
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	var merged bool
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				pr, err := exactMergePR(owner, repo, index)
				pr.HeadSHA = featureSHA
				pr.Merged = merged
				if merged {
					pr.State = "merged"
				}
				return pr, err
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				addMergeCall()
				merged = true
				return errors.New("response lost after merge")
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	if err != nil || response.Merge == nil || response.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("merge recovery response = %+v, err = %v", response, err)
	}
	if resultReports() != 1 || mergeCalls() != 1 {
		t.Fatalf("merge recovery counts: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

func TestPRMergeApprovedRealGuardsFailClosed(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name        string
		mutate      func(*gitprovider.PullRequest)
		ciState     string
		mergeErr    error
		wantMerge   int
		wantDetail  string
		wantRetry   bool
		wantSuccess bool
	}{
		{
			name: "closed PR", mutate: func(pr *gitprovider.PullRequest) { pr.State = "closed" },
			wantDetail: "closed and unmerged",
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
			name: "unknown CI", ciState: "unknown",
			wantDetail: "CI status could not be verified", wantRetry: true,
		},
		{
			name: "CI not configured", ciState: gitprovider.StateNotConfigured, wantMerge: 1, wantSuccess: true,
		},
		{
			name: "forge failure", mergeErr: errors.New("head moved"), wantMerge: 1,
			wantDetail: "forge squash merge failed", wantRetry: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, mergeCalls, err := runApprovedRealMergeScenario(t, tt.mutate, tt.ciState, tt.mergeErr)
			if tt.wantSuccess {
				if err != nil || response.Merge == nil || response.Merge.Status != PRMergeStatusExecuted {
					t.Fatalf("response = %+v, err = %v, want executed success", response, err)
				}
			} else if tt.wantRetry {
				if err == nil || !strings.Contains(err.Error(), "remains approved") ||
					!strings.Contains(err.Error(), tt.wantDetail) {
					t.Fatalf("error = %v, want retryable error containing %q", err, tt.wantDetail)
				}
			} else if response.Merge == nil || response.Merge.Status != PRMergeStatusExecuteFailed ||
				!strings.Contains(response.Merge.Detail, tt.wantDetail) {
				t.Fatalf("response = %+v, err = %v, want execute_failed containing %q", response, err, tt.wantDetail)
			}
			if mergeCalls != tt.wantMerge {
				t.Fatalf("merge calls = %d, want %d", mergeCalls, tt.wantMerge)
			}
		})
	}
}

func exactMergePR(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
	return &gitprovider.PullRequest{
		Index: index, State: "open", Mergeable: true, Head: "feature/merge", HeadSHA: "abc123",
		Base: "main", HTMLURL: "https://github.com/tta-lab/organon/pull/7",
	}, nil
}

func setupMergeCleanupRepo(t *testing.T, repo string) string {
	t.Helper()
	gitRun(t, repo, "branch", branchMain)
	mainSHA := gitOut(t, repo, "rev-parse", "refs/heads/"+branchMain)
	gitRun(t, repo, "update-ref", "refs/remotes/origin/"+branchMain, mainSHA)
	gitRun(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+branchMain)
	featureSHA := gitOut(t, repo, "rev-parse", "refs/heads/feature/merge")
	gitRun(t, repo, "update-ref", "refs/remotes/origin/feature/merge", featureSHA)
	return featureSHA
}

func newRetryMergeFixture(t *testing.T, actionID string) (
	Service, string, func() int, func(), func() int,
) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	setupMergeCleanupRepo(t, repo)
	restoreGit := stubRunGitWithCreds(t, func(ctxInfo *repoContext, args ...string) error {
		if len(args) >= 3 && args[0] == "push" && args[1] == remoteOrigin && args[2] == "--delete" {
			gitRun(t, ctxInfo.WorkDir, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	})
	t.Cleanup(restoreGit)
	var actionPayload any
	var resultReports int
	var mergeCalls int
	var actionGets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			actionPayload = body["payload"]
			writeMergeAction(t, w, actionID, PRMergeStatusPending, actionPayload)
		case r.URL.Path == "/v1/actions/"+actionID && r.Method == http.MethodGet:
			status := PRMergeStatusApproved
			if actionGets == 0 {
				status = PRMergeStatusPending
			}
			actionGets++
			writeMergeAction(t, w, actionID, status, actionPayload)
		case r.URL.Path == "/v1/actions/"+actionID+"/result" && r.Method == http.MethodPost:
			resultReports++
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
				return exactMergePR(owner, repo, index)
			},
			getCombinedStatus: func(owner, repo, ref string) (*gitprovider.CombinedStatus, error) {
				return &gitprovider.CombinedStatus{State: gitprovider.StateSuccess}, nil
			},
			mergePR: func(owner, repo string, index int64, headSHA string) error {
				mergeCalls++
				return nil
			},
		}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	service := NewServiceWithConfig(&recordingBroker{token: "test-token"}, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	return service, repo, func() int { return resultReports }, func() { mergeCalls++ },
		func() int { return mergeCalls }
}

func runApprovedRealMergeScenario( //nolint:gocyclo
	t *testing.T, mutate func(*gitprovider.PullRequest), ciState string, mergeErr error,
) (Response, int, error) {
	t.Helper()
	if ciState == "" {
		ciState = gitprovider.StateSuccess
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	featureSHA := setupMergeCleanupRepo(t, repo)
	restoreGit := stubRunGitWithCreds(t, func(ctxInfo *repoContext, args ...string) error {
		if len(args) >= 3 && args[0] == "push" && args[1] == remoteOrigin && args[2] == "--delete" {
			gitRun(t, ctxInfo.WorkDir, "update-ref", "-d", "refs/remotes/origin/feature/merge")
		}
		return nil
	})
	t.Cleanup(restoreGit)
	var actionPayload any
	var mergeCalls int
	var actionGets int
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
			status := PRMergeStatusApproved
			if actionGets == 0 {
				status = PRMergeStatusPending
			}
			actionGets++
			writeMergeAction(t, w, "act-guard", status, actionPayload)
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
					Head: "feature/merge", HeadSHA: featureSHA, Base: "main",
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

	service := NewServiceWithConfig(&recordingBroker{token: "test-token"}, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7})
	return response, mergeCalls, err
}

func writeMergeAction(t *testing.T, w http.ResponseWriter, id, status string, payload any) {
	t.Helper()
	target := "https://github.com/tta-lab/organon/pull/7"
	if values, ok := payload.(map[string]any); ok {
		if value, ok := values["pr_url"].(string); ok && value != "" {
			target = value
		}
	}
	writeMergeActionWithFields(t, w, id, PRMergeKind, status, target, payload)
}

func writeMergeActionWithTarget(t *testing.T, w http.ResponseWriter, id, status, target string, payload any) {
	t.Helper()
	writeMergeActionWithFields(t, w, id, PRMergeKind, status, target, payload)
}

func writeMergeActionWithFields(t *testing.T, w http.ResponseWriter, id, kind, status, target string, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": id, "kind": kind, "status": status, "inbox_url": "http://impri.example/actions",
		"target_url": target, "payload": payload,
	})
}

func writeMergeActionWithoutInbox(t *testing.T, w http.ResponseWriter, id, status string, payload any) {
	t.Helper()
	target := "https://github.com/tta-lab/organon/pull/7"
	if values, ok := payload.(map[string]any); ok {
		if value, ok := values["pr_url"].(string); ok && value != "" {
			target = value
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"color": "blue", "created_at": "2026-09-06T00:00:00Z", "editable": true,
		"expires_at": "2026-09-06T00:05:00Z", "id": id, "idempotency_key": "idempotent-key",
		"kind": PRMergeKind, "payload": payload,
		"preview": map[string]any{"format": "markdown", "body": "preview"},
		"status":  status, "target_url": target, "title": "merge",
		"updated_at": "2026-09-06T00:00:00Z",
	})
}
