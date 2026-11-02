package og

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tta-lab/organon/internal/gitprovider"
	"github.com/tta-lab/organon/internal/ogconfig"
)

func TestDenyMergeBeforeExternalOperations(t *testing.T) {
	for _, tc := range []struct {
		name, remote string
		dry, impri   bool
	}{
		{name: "github real without impri", remote: "https://github.com/tta-lab/example.git"},
		{name: "github dry run", remote: "https://github.com/tta-lab/example.git", dry: true, impri: true},
		{name: "forgejo real", remote: "https://forge.example/tta-lab/example.git", impri: true},
		{name: "forgejo dry run without impri", remote: "https://forge.example/tta-lab/example.git", dry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			repo := testRegisteredRepo(t, home, "feature/merge", tc.remote, false)
			var impriCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				impriCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			providerCalls, gitCalls, dryRunCalls := 0, 0, 0
			t.Cleanup(stubNewProvider(t, func(*repoContext) (gitprovider.Provider, error) {
				providerCalls++
				return nil, errors.New("unexpected provider initialization")
			}))
			t.Cleanup(stubRunGitWithCreds(t, func(*repoContext, ...string) error {
				gitCalls++
				return errors.New("unexpected Git mutation")
			}))
			cfg := ogconfig.Config{
				Merge:   ogconfig.MergeConfig{Approval: ogconfig.MergeApprovalDeny},
				Forgejo: ogconfig.ForgejoConfig{AllowedBaseURLs: []string{"https://forge.example"}},
			}
			if tc.impri {
				cfg.Impri = &ogconfig.ImpriConfig{BaseURL: server.URL, APIKey: "fixture"}
			}
			service := NewServiceWithConfigAndDryRunExecutor(nil, nil, cfg,
				func(context.Context, PRMergeSnapshot) error { dryRunCalls++; return nil })
			var index int64
			if !tc.dry {
				index = 7
			}
			response, err := service.PRMerge(Request{WorkDir: repo, Index: index, DryRun: tc.dry})
			var retryErr *PRMergeRetryableError
			if err == nil || !strings.Contains(err.Error(), "merge.approval=deny") || errors.As(err, &retryErr) {
				t.Fatalf("expected non-retryable deny error, got %v", err)
			}
			if response.Merge != nil || providerCalls != 0 || gitCalls != 0 || dryRunCalls != 0 || impriCalls.Load() != 0 {
				t.Fatalf("response %+v; provider %d, git %d, dry-run %d, Impri %d",
					response, providerCalls, gitCalls, dryRunCalls, impriCalls.Load())
			}
		})
	}
}
