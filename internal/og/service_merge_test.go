package og

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if pending.Merge == nil || pending.Merge.Status != PRMergeStatusPending || pending.Merge.ActionID != "act-dry-run" ||
		!pending.Merge.Resumable || pending.Merge.Retryable || pending.Merge.NextAction != PRMergeNextWait ||
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
		!timedOut.Merge.Resumable || timedOut.Merge.NextAction != PRMergeNextWait ||
		!strings.Contains(timedOut.Merge.Detail, "timed out") {
		t.Fatalf("timeout response = %+v, err = %v", timedOut, err)
	}
	if atomic.LoadInt32(&dryRuns) != 0 {
		t.Fatalf("dry runs after timeout = %d, want 0", dryRuns)
	}
	status.Store(PRMergeStatusRejected)
	rejected, err := service.PRMerge(Request{WorkDir: repo, Index: 7, DryRun: true, ActionID: "act-dry-run"})
	if err != nil || rejected.Merge == nil || rejected.Merge.Status != PRMergeStatusRejected ||
		rejected.Merge.Resumable || rejected.Merge.Retryable || rejected.Merge.NextAction != PRMergeNextNewApproval {
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
	if executed.Merge == nil || executed.Merge.Status != PRMergeStatusExecuted ||
		executed.Merge.Resumable || executed.Merge.Retryable || executed.Merge.NextAction != PRMergeNextNone {
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
		mergedResponse.Merge.ReceiptError == "" || !mergedResponse.Merge.Resumable ||
		!mergedResponse.Merge.Retryable || mergedResponse.Merge.NextAction != PRMergeNextRepairReceipt {
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

func TestPRMergeRejectsWrongActionKindBeforeStatusDispatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/actions/act-kind" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		writeMergeActionWithFields(t, w, "act-kind", "other.kind", PRMergeStatusApproved,
			canonicalMergePayload()["pr_url"].(string), canonicalMergePayload())
	}))
	t.Cleanup(server.Close)

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	_, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-kind"})
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("PRMerge error = %v, want wrong-kind rejection", err)
	}
}

func TestPRMergeRejectsWrongTargetOnCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/actions" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		payload := body["payload"].(map[string]any)
		writeMergeActionWithTarget(t, w, "act-target-create", PRMergeStatusPending,
			payload["pr_url"].(string)+"/wrong", payload)
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

func TestPRMergeRejectsWrongTargetOnResumeAndAfterWait(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name string
		wait bool
	}{
		{name: "resume", wait: false},
		{name: "wait poll", wait: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/merge")
			payload := canonicalMergePayload()
			var actionGets int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/actions/act-target" && r.Method == http.MethodGet:
					actionGets++
					status := PRMergeStatusApproved
					target := payload["pr_url"].(string) + "/wrong"
					if tt.wait && actionGets == 1 {
						status = PRMergeStatusPending
						target = payload["pr_url"].(string)
					}
					writeMergeActionWithTarget(t, w, "act-target", status,
						target, payload)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			service := NewServiceWithConfig(nil, nil, ogconfig.Config{
				Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
			})
			_, err := service.PRMerge(Request{WorkDir: repo, Index: 7,
				ActionID: "act-target", Wait: tt.wait, Timeout: 2 * time.Second})
			if err == nil || !strings.Contains(err.Error(), "target_url") {
				t.Fatalf("PRMerge error = %v, want target rejection", err)
			}
		})
	}
}

func TestPRMergeReportsUnavailableWhenResumeReadFails(t *testing.T) { //nolint:gocyclo
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var providerCalls atomic.Int32
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		providerCalls.Add(1)
		return fakeProvider{getPR: exactMergePR}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/actions/act-unavailable" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		http.Error(w, "temporary Impri outage", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	response, err := service.PRMerge(Request{
		WorkDir: repo, Index: 7, ActionID: "act-unavailable",
	})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || response.Merge == nil {
		t.Fatalf("response = %+v, err = %v, want structured retryable outcome", response, err)
	}
	merge := response.Merge
	if merge.Status != PRMergeStatusUnavailable || merge.ActionID != "act-unavailable" ||
		!merge.Resumable || !merge.Retryable || merge.NextAction != PRMergeNextRetry ||
		merge.InboxURL == "" || merge.ReceiptError != "" {
		t.Fatalf("unavailable merge = %+v", merge)
	}
	for _, want := range []string{
		"approval state is temporarily unavailable", "no forge call was made", "retry the same action ID",
	} {
		if !strings.Contains(merge.Detail, want) {
			t.Fatalf("unavailable detail = %q, want %q", merge.Detail, want)
		}
	}
	if err := ValidatePRMergeResponse(response, 7); err != nil {
		t.Fatalf("validate unavailable response: %v", err)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d, want no forge call", providerCalls.Load())
	}
}

func TestPRMergeClassifiesImpriHTTPReadFailures(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusForbidden, retryable: false},
		{status: http.StatusNotFound, retryable: false},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusBadGateway, retryable: true},
	} {
		t.Run(fmt.Sprintf("http-%d", test.status), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredHTTPRepo(t, home, "feature/merge")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/actions/act-http-status" || r.Method != http.MethodGet {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				http.Error(w, `{"error":"im_test_secret"}`, test.status)
			}))
			defer server.Close()
			service := NewServiceWithConfig(nil, nil, ogconfig.Config{
				Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test_secret"},
			})
			response, err := service.PRMerge(Request{
				WorkDir: repo, Index: 7, ActionID: "act-http-status",
			})
			if err == nil || strings.Contains(err.Error(), "im_test_secret") {
				t.Fatalf("HTTP %d response = %+v, error = %v", test.status, response, err)
			}
			var retryErr *PRMergeRetryableError
			gotRetryable := errors.As(err, &retryErr)
			if gotRetryable != test.retryable {
				t.Fatalf("HTTP %d response = %+v, error = %v, retryable = %v, want %v",
					test.status, response, err, gotRetryable, test.retryable)
			}
			if test.retryable {
				if response.Merge == nil || response.Merge.Status != PRMergeStatusUnavailable ||
					response.Merge.NextAction != PRMergeNextRetry {
					t.Fatalf("HTTP %d retry response = %+v", test.status, response)
				}
			} else if response.Merge != nil || strings.Contains(err.Error(), "temporarily unavailable") {
				t.Fatalf("HTTP %d ordinary failure response = %+v, error = %v", test.status, response, err)
			}
		})
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
		!merge.Resumable || !merge.Retryable || merge.NextAction != PRMergeNextRetry ||
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
			writeMergeAction(t, w, "act-failure-receipt", PRMergeStatusApproved, actionPayload)
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
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-failure-receipt"})
	var retryErr *PRMergeRetryableError
	if err == nil || !errors.As(err, &retryErr) || first.Merge == nil ||
		first.Merge.Status != PRMergeStatusApproved || !first.Merge.Resumable ||
		!first.Merge.Retryable || first.Merge.NextAction != PRMergeNextRetry ||
		first.Merge.ReceiptError != "" || !strings.Contains(first.Merge.Detail, "could not be recorded") {
		t.Fatalf("unrecorded failure response = %+v, err = %v", first, err)
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-failure-receipt"})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuteFailed ||
		second.Merge.Resumable || second.Merge.Retryable || second.Merge.NextAction != PRMergeNextNewApproval {
		t.Fatalf("recorded failure response = %+v, err = %v", second, err)
	}
	if len(receiptStatuses) != 2 || receiptStatuses[0] != PRMergeStatusExecuteFailed ||
		receiptStatuses[1] != PRMergeStatusExecuteFailed {
		t.Fatalf("receipt statuses = %v, want two execute_failed attempts", receiptStatuses)
	}
}

//nolint:gocyclo
func TestPRMergeKeepsApprovalOnTemporaryRefetchFailure(t *testing.T) {
	service, repo, resultReports, _, mergeCalls, providerGets := newRetryMergeFixture(t, "act-refetch")
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-refetch"})
	if err == nil || first.Merge == nil || !first.Merge.Resumable || !first.Merge.Retryable ||
		first.Merge.Status != PRMergeStatusApproved || first.Merge.NextAction != PRMergeNextRetry ||
		!strings.Contains(err.Error(), "act-refetch") || !strings.Contains(err.Error(), "remains approved") {
		t.Fatalf("first retry response = %+v merge=%+v, err = %v", first, first.Merge, err)
	}
	if resultReports() != 0 || mergeCalls() != 0 || providerGets() != 2 {
		t.Fatalf("after temporary refetch failure: reports=%d merges=%d provider_gets=%d",
			resultReports(), mergeCalls(), providerGets())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-refetch"})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("resumed response = %+v, err = %v", second, err)
	}
	if resultReports() != 1 || mergeCalls() != 1 {
		t.Fatalf("after recovery: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

//nolint:gocyclo
func TestPRMergeKeepsApprovalOnTemporaryCIFailure(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls, _ := newRetryMergeFixture(t, "act-ci")
	var ciCalls int
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: exactMergePR,
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
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-ci"})
	if err == nil || first.Merge == nil || !first.Merge.Resumable || !first.Merge.Retryable ||
		first.Merge.NextAction != PRMergeNextRetry || !strings.Contains(err.Error(), "CI status") {
		t.Fatalf("first CI response = %+v, err = %v", first, err)
	}
	if resultReports() != 0 || mergeCalls() != 0 {
		t.Fatalf("after temporary CI failure: reports=%d merges=%d", resultReports(), mergeCalls())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-ci"})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("resumed CI response = %+v, err = %v", second, err)
	}
	if resultReports() != 1 || mergeCalls() != 1 {
		t.Fatalf("after CI recovery: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

//nolint:gocyclo
func TestPRMergeRetriesAmbiguousMergeFailureWithoutConsumingApproval(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls, _ := newRetryMergeFixture(t, "act-merge")
	var calls int
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: exactMergePR,
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
	first, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-merge"})
	if err == nil || first.Merge == nil || !first.Merge.Resumable || !first.Merge.Retryable ||
		first.Merge.NextAction != PRMergeNextRetry || !strings.Contains(err.Error(), "ambiguous") ||
		!strings.Contains(err.Error(), "remains approved") {
		t.Fatalf("first merge response = %+v, err = %v", first, err)
	}
	if resultReports() != 0 || mergeCalls() != 1 {
		t.Fatalf("after ambiguous merge failure: reports=%d merges=%d", resultReports(), mergeCalls())
	}
	second, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-merge"})
	if err != nil || second.Merge == nil || second.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("resumed merge response = %+v, err = %v", second, err)
	}
	if resultReports() != 1 || mergeCalls() != 2 {
		t.Fatalf("after merge recovery: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

func TestPRMergeRepairsReceiptWhenMergeErrorWasActuallySuccessful(t *testing.T) {
	service, repo, resultReports, addMergeCall, mergeCalls, _ := newRetryMergeFixture(t, "act-merged")
	var merged bool
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		return fakeProvider{
			getPR: func(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
				pr, err := exactMergePR(owner, repo, index)
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
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-merged"})
	if err != nil || response.Merge == nil || response.Merge.Status != PRMergeStatusExecuted {
		t.Fatalf("merge recovery response = %+v, err = %v", response, err)
	}
	if resultReports() != 1 || mergeCalls() != 1 {
		t.Fatalf("merge recovery counts: reports=%d merges=%d", resultReports(), mergeCalls())
	}
}

func TestPRMergeUnsupportedProviderSetupConsumesApproval(t *testing.T) {
	service, repo, resultReports, _, mergeCalls, _ := newRetryMergeFixture(t, "act-unsupported")
	var providerCalls int
	oldProvider := newProviderFunc
	newProviderFunc = func(*repoContext) (gitprovider.Provider, error) {
		providerCalls++
		if providerCalls > 1 {
			return nil, errors.New("unsupported provider method")
		}
		return fakeProvider{getPR: exactMergePR}, nil
	}
	t.Cleanup(func() { newProviderFunc = oldProvider })
	if _, err := service.PRMerge(Request{WorkDir: repo, Index: 7}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	response, err := service.PRMerge(Request{WorkDir: repo, Index: 7, ActionID: "act-unsupported"})
	if err != nil || response.Merge == nil || response.Merge.Status != PRMergeStatusExecuteFailed ||
		response.Merge.NextAction != PRMergeNextNewApproval || response.Merge.Resumable ||
		!strings.Contains(response.Merge.Detail, "unsupported merge provider") {
		t.Fatalf("unsupported provider response = %+v, err = %v", response, err)
	}
	if resultReports() != 1 || mergeCalls() != 0 {
		t.Fatalf("unsupported provider counts: reports=%d merges=%d", resultReports(), mergeCalls())
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
		wantRetry  bool
	}{
		{
			name: "stale head SHA", mutate: func(pr *gitprovider.PullRequest) { pr.HeadSHA = "new-sha" },
			wantDetail: "identity changed",
		},
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
			name: "forge failure", mergeErr: errors.New("head moved"), wantMerge: 1,
			wantDetail: "forge squash merge failed", wantRetry: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, mergeCalls, err := runApprovedRealMergeScenario(t, tt.mutate, tt.ciState, tt.mergeErr)
			if tt.wantRetry {
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

func canonicalMergePayload() map[string]any {
	return map[string]any{
		"provider": "github", "forge_base_url": "https://github.com",
		"owner": "tta-lab", "repo": "example", "pr_number": 7,
		"head_sha": "abc123", "base_branch": "main", "merge_method": PRMergeMethodSquash,
		"execution_mode": PRMergeModeReal, "pr_url": "https://github.com/tta-lab/example/pull/7",
	}
}

func exactMergePR(owner, repo string, index int64) (*gitprovider.PullRequest, error) {
	return &gitprovider.PullRequest{
		Index: index, State: "open", Mergeable: true, Head: "feature/merge", HeadSHA: "abc123",
		Base: "main", HTMLURL: "https://github.com/tta-lab/organon/pull/7",
	}, nil
}

func newRetryMergeFixture(t *testing.T, actionID string) (
	Service, string, func() int, func(), func() int, func() int,
) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := testRegisteredHTTPRepo(t, home, "feature/merge")
	var actionPayload any
	var resultReports int
	var mergeCalls int
	var providerGets int
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
			writeMergeAction(t, w, actionID, PRMergeStatusApproved, actionPayload)
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
				providerGets++
				if actionID == "act-refetch" && providerGets == 2 {
					return nil, errors.New("temporary provider outage")
				}
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
	service := NewServiceWithConfig(nil, nil, ogconfig.Config{
		Impri: &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "im_test"},
	})
	return service, repo, func() int { return resultReports }, func() { mergeCalls++ },
		func() int { return mergeCalls }, func() int { return providerGets }
}

func runApprovedRealMergeScenario(
	t *testing.T, mutate func(*gitprovider.PullRequest), ciState string, mergeErr error,
) (Response, int, error) {
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
