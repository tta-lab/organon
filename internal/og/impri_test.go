package og

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/ogconfig"
)

func TestDecodeImpriActionRequiresCanonicalKindAndTarget(t *testing.T) {
	payload := map[string]any{
		"provider": "github", "forge_base_url": "https://github.com",
		"owner": "tta-lab", "repo": "organon", "pr_number": 7,
		"head_sha": "abc123", "base_branch": "main", "merge_method": "squash",
		"execution_mode": "real", "pr_url": "https://github.com/tta-lab/organon/pull/7",
	}
	base := map[string]any{
		"id": "act-1", "kind": PRMergeKind, "status": PRMergeStatusApproved,
		"target_url": payload["pr_url"], "payload": payload,
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "wrong kind", mutate: func(action map[string]any) { action["kind"] = "other.kind" }},
		{name: "missing target", mutate: func(action map[string]any) { delete(action, "target_url") }},
		{name: "envelope", mutate: func(action map[string]any) {
			copy := map[string]any{}
			for key, value := range action {
				copy[key] = value
			}
			for key := range action {
				delete(action, key)
			}
			action["action"] = copy
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire := map[string]any{}
			for key, value := range base {
				wire[key] = value
			}
			test.mutate(wire)
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeImpriAction(data); err == nil {
				t.Fatalf("decodeImpriAction accepted %s", test.name)
			}
		})
	}
}

func TestImpriClientRequiresConfiguredSectionWithoutEnvironmentFallback(t *testing.T) {
	t.Setenv("IMPRI_BASE_URL", "http://impri.example")
	t.Setenv("IMPRI_API_KEY", "im_secret")
	if _, err := newImpriClient(context.Background(), nil); err == nil {
		t.Fatal("newImpriClient accepted missing configured [impri] section")
	} else if strings.Contains(err.Error(), "im_secret") {
		t.Fatalf("configuration error exposed API key: %v", err)
	}
}

func TestImpriClientRedactsAPIKeyFromHTTPErrors(t *testing.T) {
	const apiKey = "im_test_secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("Authorization = %q", got)
		}
		http.Error(w, `{"error":"im_test_secret"}`, http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	client, err := newImpriClientWithHTTPClient(context.Background(), &ogconfig.ImpriConfig{
		BaseURL: server.URL, APIKey: apiKey,
	}, server.Client())
	if err != nil {
		t.Fatalf("newImpriClient: %v", err)
	}
	_, err = client.getAction(context.Background(), "act-1")
	if err == nil || strings.Contains(err.Error(), apiKey) || !strings.Contains(err.Error(), "502") {
		t.Fatalf("getAction error = %v, want redacted HTTP status", err)
	}
}

func TestImpriReadErrorRetryClassification(t *testing.T) {
	const apiKey = "im_test_secret"
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"im_test_secret"}`, test.status)
			}))
			defer server.Close()
			client, err := newImpriClientWithHTTPClient(context.Background(), &ogconfig.ImpriConfig{
				BaseURL: server.URL, APIKey: apiKey,
			}, server.Client())
			if err != nil {
				t.Fatalf("newImpriClient: %v", err)
			}
			_, err = client.getAction(context.Background(), "act-1")
			gotRetryable := err != nil && retryableImpriReadError(err)
			if err == nil || gotRetryable != test.retryable {
				t.Fatalf("getAction error = %v, retryable = %v, want %v",
					err, gotRetryable, test.retryable)
			}
			if strings.Contains(err.Error(), apiKey) {
				t.Fatalf("HTTP %d error exposed API key: %v", test.status, err)
			}
			var httpErr *impriHTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode() != test.status {
				t.Fatalf("getAction error type = %T, status = %v, want HTTP %d", err, httpErr, test.status)
			}
		})
	}
}

func TestImpriDecodeErrorIsNotRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()
	client, err := newImpriClientWithHTTPClient(context.Background(), &ogconfig.ImpriConfig{
		BaseURL: server.URL, APIKey: "im_test_secret",
	}, server.Client())
	if err != nil {
		t.Fatalf("newImpriClient: %v", err)
	}
	_, err = client.getAction(context.Background(), "act-1")
	gotRetryable := err != nil && retryableImpriReadError(err)
	if err == nil || gotRetryable {
		t.Fatalf("decode error = %v, retryable = %v, want ordinary fail-closed error",
			err, gotRetryable)
	}
}

func TestImpriCreateActionReadsReceiptThenCanonicalAction(t *testing.T) { //nolint:gocyclo
	payload := map[string]any{
		"provider": "github", "forge_base_url": "https://github.com",
		"owner": "tta-lab", "repo": "organon", "pr_number": 7,
		"head_sha": "abc123", "base_branch": "main", "merge_method": "squash",
		"execution_mode": "dry-run", "pr_url": "https://github.com/tta-lab/organon/pull/7",
	}
	body := impriCreateAction{Kind: PRMergeKind, Title: "merge", Preview: impriPreview{
		Format: "markdown", Body: "preview",
	}, Payload: payload, TargetURL: payload["pr_url"].(string), ExpiresIn: 300, IdempotencyKey: "key"}
	for _, test := range []struct {
		name       string
		postStatus int
	}{
		{name: "created receipt", postStatus: http.StatusCreated},
		{name: "idempotent full response", postStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			var postCount, getCount int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/actions" && r.Method == http.MethodPost:
					postCount++
					w.WriteHeader(test.postStatus)
					if test.postStatus == http.StatusCreated {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"created_at": "2026-09-06T00:00:00Z", "expires_at": "2026-09-06T00:05:00Z",
							"id": "act-receipt", "inbox_url": "https://impri.example/actions",
							"status": PRMergeStatusPending,
						})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"color": "blue", "created_at": "2026-09-06T00:00:00Z", "editable": true,
							"expires_at": "2026-09-06T00:05:00Z", "id": "act-receipt",
							"idempotency_key": "key", "kind": PRMergeKind, "payload": payload,
							"preview": map[string]any{"format": "markdown", "body": "preview"},
							"status":  PRMergeStatusPending, "target_url": payload["pr_url"],
							"title": "merge", "updated_at": "2026-09-06T00:00:00Z",
						})
					}
				case r.URL.Path == "/v1/actions/act-receipt" && r.Method == http.MethodGet:
					getCount++
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id": "act-receipt", "kind": PRMergeKind, "status": PRMergeStatusPending,
						"inbox_url": "https://impri.example/actions", "target_url": payload["pr_url"],
						"payload": payload,
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := newImpriClientWithHTTPClient(context.Background(), &ogconfig.ImpriConfig{
				BaseURL: server.URL, APIKey: "im_test",
			}, server.Client())
			if err != nil {
				t.Fatalf("newImpriClient: %v", err)
			}
			action, err := client.createAction(context.Background(), body)
			if err != nil {
				t.Fatalf("createAction: %v", err)
			}
			if action.ID != "act-receipt" || action.Kind != PRMergeKind ||
				action.Status != PRMergeStatusPending || action.TargetURL != payload["pr_url"] ||
				action.Payload == nil {
				t.Fatalf("canonical action = %+v", action)
			}
			if postCount != 1 || getCount != 1 {
				t.Fatalf("POST count = %d, GET count = %d, want 1 each", postCount, getCount)
			}
		})
	}
}

func TestImpriCreateActionRejectsMalformedReceiptBeforeGET(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing id", mutate: func(receipt map[string]any) { delete(receipt, "id") }},
		{name: "invalid status", mutate: func(receipt map[string]any) { receipt["status"] = "unknown" }},
		{name: "missing status", mutate: func(receipt map[string]any) { delete(receipt, "status") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt := map[string]any{
				"id": "act-malformed", "status": PRMergeStatusPending,
				"inbox_url": "https://impri.example/actions",
			}
			test.mutate(receipt)
			getCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					getCount++
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(receipt)
			}))
			defer server.Close()
			client, err := newImpriClientWithHTTPClient(context.Background(), &ogconfig.ImpriConfig{
				BaseURL: server.URL, APIKey: "im_test",
			}, server.Client())
			if err != nil {
				t.Fatalf("newImpriClient: %v", err)
			}
			_, err = client.createAction(context.Background(), impriCreateAction{Title: "merge"})
			if err == nil || getCount != 0 {
				t.Fatalf("createAction error = %v, GET count = %d, want receipt rejection before GET", err, getCount)
			}
		})
	}
}

func TestMergeIdempotencyKeyIncludesImmutableModeAndHead(t *testing.T) {
	base := PRMergeSnapshot{
		Provider: "github", ForgeBaseURL: "https://github.com", Owner: "o", Repo: "r",
		PRNumber: 7, HeadSHA: "sha-a", BaseBranch: "main", MergeMethod: PRMergeMethodSquash,
		ExecutionMode: PRMergeModeDryRun,
	}
	first, err := mergeIdempotencyKey(base)
	if err != nil {
		t.Fatalf("mergeIdempotencyKey: %v", err)
	}
	changedHead := base
	changedHead.HeadSHA = "sha-b"
	second, err := mergeIdempotencyKey(changedHead)
	if err != nil {
		t.Fatalf("mergeIdempotencyKey changed head: %v", err)
	}
	changedMode := base
	changedMode.ExecutionMode = PRMergeModeReal
	third, err := mergeIdempotencyKey(changedMode)
	if err != nil {
		t.Fatalf("mergeIdempotencyKey changed mode: %v", err)
	}
	if first == second || first == third || !strings.HasPrefix(first, "og-pr-merge-") {
		t.Fatalf("keys = %q, %q, %q; want distinct immutable identities", first, second, third)
	}
	changedTitle := base
	changedTitle.Title = "descriptive title changed after approval"
	fourth, err := mergeIdempotencyKey(changedTitle)
	if err != nil {
		t.Fatalf("mergeIdempotencyKey changed title: %v", err)
	}
	if first != fourth {
		t.Fatalf("title changed idempotency key = %q, want unchanged %q", fourth, first)
	}
}
