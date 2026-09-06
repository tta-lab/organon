package og

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/ogconfig"
)

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
