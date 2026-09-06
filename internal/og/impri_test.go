package og

import (
	"context"
	"encoding/json"
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
}
