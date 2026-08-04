package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryKeySource struct{ key *rsa.PrivateKey }

func (s memoryKeySource) PrivateKey() (*rsa.PrivateKey, error) { return s.key, nil }

func TestBrokerRejectsDisallowedOwnerBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})
	_, err := broker.Token(context.Background(), "outsider", "organon", PurposeGitRead)
	if err == nil || !strings.Contains(err.Error(), "allowed_owners") {
		t.Fatalf("Token error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("network requests = %d, want 0", got)
	}
}

func TestBrokerDiscoversInstallationAndMintsPurposeScopedTokens(t *testing.T) {
	state := &scopedTokenServer{t: t}
	server := httptest.NewServer(state)
	t.Cleanup(server.Close)

	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})
	ctx := context.Background()
	tests := []struct {
		purpose Purpose
		want    map[string]string
	}{
		{PurposeAPI, map[string]string{"actions": "read", "checks": "read", "contents": "read", "pull_requests": "write"}},
		{PurposeGitRead, map[string]string{"contents": "read"}},
		{PurposeGitWrite, map[string]string{"contents": "write", "workflows": "write"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.purpose), func(t *testing.T) {
			token, err := broker.Token(ctx, "tta-lab", "organon", tt.purpose)
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if token == "" {
				t.Fatal("Token returned empty token")
			}
		})
	}

	if got := state.installationRequests.Load(); got != 1 {
		t.Fatalf("installation requests = %d, want 1", got)
	}
	tokenRequests := state.requests()
	if len(tokenRequests) != len(tests) {
		t.Fatalf("token requests = %d, want %d", len(tokenRequests), len(tests))
	}
	for i, request := range tokenRequests {
		repos, ok := request["repositories"].([]any)
		if !ok || len(repos) != 1 || repos[0] != "organon" {
			t.Fatalf("request %d repositories = %#v", i, request["repositories"])
		}
		permissions, ok := request["permissions"].(map[string]any)
		if !ok || !equalPermissions(permissions, tests[i].want) {
			t.Fatalf("request %d permissions = %#v, want %#v", i, permissions, tests[i].want)
		}
	}
}

func TestBrokerReusesAndRefreshesTokens(t *testing.T) {
	var tokenRequests atomic.Int32
	var shortExpiry atomic.Bool
	server := testBrokerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := tokenRequests.Add(1)
		expiry := time.Now().Add(time.Hour)
		if shortExpiry.Load() {
			expiry = time.Now().Add(30 * time.Second)
		}
		writeJSON(t, w, map[string]any{
			"token":      fmt.Sprintf("token-%d", n),
			"expires_at": expiry.UTC().Format(time.RFC3339),
		})
	})
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})

	first, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	second, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || tokenRequests.Load() != 1 {
		t.Fatalf("cached tokens = %q, %q; requests = %d", first, second, tokenRequests.Load())
	}

	invalidationErr := broker.Invalidate("tta-lab", "organon", PurposeGitRead, first)
	if invalidationErr == nil || !strings.Contains(invalidationErr.Error(), "cached credential invalidated") {
		t.Fatalf("Invalidate error = %v", invalidationErr)
	}
	if tokenRequests.Load() != 1 {
		t.Fatalf("Invalidate retried the current operation; token requests = %d", tokenRequests.Load())
	}
	shortExpiry.Store(true)
	third, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	fourth, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	if third == fourth || tokenRequests.Load() != 3 {
		t.Fatalf("near-expiry tokens = %q, %q; requests = %d", third, fourth, tokenRequests.Load())
	}
}

func TestBrokerDoesNotInvalidateReplacementForStaleFailure(t *testing.T) {
	var tokenRequests atomic.Int32
	server := testBrokerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := tokenRequests.Add(1)
		writeJSON(t, w, map[string]any{
			"token":      fmt.Sprintf("token-%d", n),
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})

	first, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	_ = broker.Invalidate("tta-lab", "organon", PurposeGitRead, first)
	replacement, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	_ = broker.Invalidate("tta-lab", "organon", PurposeGitRead, first)
	stillCached, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	if stillCached != replacement || tokenRequests.Load() != 2 {
		t.Fatalf("stale failure replaced %q with %q; requests = %d", replacement, stillCached, tokenRequests.Load())
	}
}

func TestBrokerRediscoversInstallationAfterAuthenticationFailure(t *testing.T) {
	var installationRequests atomic.Int32
	var tokenRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/tta-lab/organon/installation":
			id := 40 + installationRequests.Add(1)
			writeJSON(t, w, map[string]any{"id": id, "permissions": map[string]string{"contents": "read"}})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/41/access_tokens":
			if tokenRequests.Add(1) > 1 {
				writeJSONStatus(t, w, http.StatusNotFound, map[string]string{"message": "installation not found"})
				return
			}
			writeJSON(t, w, installationTokenResponse("token-1"))
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens":
			tokenRequests.Add(1)
			writeJSON(t, w, installationTokenResponse("token-2"))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})

	first, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatal(err)
	}
	_ = broker.Invalidate("tta-lab", "organon", PurposeGitRead, first)
	second, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err != nil {
		t.Fatalf("Token after invalidation: %v", err)
	}
	if second != "token-2" || installationRequests.Load() != 2 {
		t.Fatalf("token = %q, installation requests = %d; want token-2 and 2", second, installationRequests.Load())
	}
}

func TestBrokerConcurrentRequestsMintOneToken(t *testing.T) {
	var tokenRequests atomic.Int32
	server := testBrokerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		tokenRequests.Add(1)
		writeJSON(t, w, map[string]any{
			"token":      "shared-token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitWrite)
			if err == nil && token != "shared-token" {
				err = fmt.Errorf("token = %q", token)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := tokenRequests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}

func TestBrokerStatusRefreshesInstallationPermissions(t *testing.T) {
	var installationRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			n := installationRequests.Add(1)
			checks := "read"
			if n > 1 {
				checks = "write"
			}
			response := installationResponse()
			response["permissions"].(map[string]string)["checks"] = checks
			writeJSON(t, w, response)
		case http.MethodPost:
			writeJSON(t, w, map[string]any{
				"token":      "cached-token",
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})
	if _, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead); err != nil {
		t.Fatalf("Token: %v", err)
	}

	status, err := broker.Status(context.Background(), "tta-lab", "organon")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.AppID != 7 || status.InstallationID != 41 || status.Repository != "tta-lab/organon" {
		t.Fatalf("status = %+v", status)
	}
	if status.Permissions["contents"] != "write" || status.Permissions["checks"] != "write" {
		t.Fatalf("permissions = %#v", status.Permissions)
	}
	if installationRequests.Load() != 2 {
		t.Fatalf("installation requests = %d, want 2", installationRequests.Load())
	}
}

func TestBrokerSanitizesGitHubErrors(t *testing.T) {
	const secret = "SERVER-ECHOED-TEST-SECRET"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})
	_, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitRead)
	if err == nil || !strings.Contains(err.Error(), "transient GitHub failure") {
		t.Fatalf("Token error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked server body: %v", err)
	}
}

func TestBrokerClassifiesGitHubFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		failLookup bool
		wantErr    string
	}{
		{"missing installation", http.StatusNotFound, true, "not installed"},
		{"denied permission", http.StatusForbidden, false, "lacks permission"},
		{"JWT or clock failure", http.StatusUnauthorized, true, "system clock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.failLookup || r.Method == http.MethodPost {
					writeJSONStatus(t, w, tt.statusCode, map[string]string{"message": "untrusted details"})
					return
				}
				writeJSON(t, w, map[string]any{"id": 41, "permissions": map[string]string{"contents": "write"}})
			}))
			t.Cleanup(server.Close)
			broker := newTestBroker(t, server, Config{AppID: 7, AllowedOwners: []string{"tta-lab"}})
			_, err := broker.Token(context.Background(), "tta-lab", "organon", PurposeGitWrite)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Token error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func newTestBroker(t *testing.T, server *httptest.Server, cfg Config) CredentialBroker {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	broker, err := NewBroker(
		cfg,
		memoryKeySource{key: key},
		WithAPIBaseURL(server.URL),
		WithHTTPTransport(server.Client().Transport),
	)
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	return broker
}

func testBrokerServer(t *testing.T, tokenHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/tta-lab/organon/installation":
			writeJSON(t, w, installationResponse())
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/41/access_tokens":
			tokenHandler(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

type scopedTokenServer struct {
	t                    *testing.T
	installationRequests atomic.Int32
	mu                   sync.Mutex
	tokenRequests        []map[string]any
}

func (s *scopedTokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/tta-lab/organon/installation":
		s.installationRequests.Add(1)
		writeJSON(s.t, w, installationResponse())
	case r.Method == http.MethodPost && r.URL.Path == "/app/installations/41/access_tokens":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.t.Errorf("decode token request: %v", err)
		}
		s.mu.Lock()
		s.tokenRequests = append(s.tokenRequests, body)
		n := len(s.tokenRequests)
		s.mu.Unlock()
		writeJSON(s.t, w, map[string]any{
			"token":      fmt.Sprintf("installation-token-%d", n),
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *scopedTokenServer) requests() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.tokenRequests...)
}

func installationResponse() map[string]any {
	return map[string]any{
		"id": 41,
		"permissions": map[string]string{
			"contents": "write", "pull_requests": "write", "checks": "read",
			"actions": "read", "workflows": "write",
		},
	}
}

func installationTokenResponse(token string) map[string]any {
	return map[string]any{
		"token":      token,
		"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func writeJSONStatus(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func equalPermissions(got map[string]any, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
