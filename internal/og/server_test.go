package og

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPHandlerRejectsTokenFields(t *testing.T) {
	handler := HTTPHandler(func(req Request) (Response, error) {
		return Response{Message: "accepted"}, nil
	})
	body, err := json.Marshal(Request{WorkDir: "/tmp/repo", Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/git/push", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	handler(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
	if !strings.Contains(resp.Body.String(), "token fields are not accepted") {
		t.Fatalf("body = %q", resp.Body.String())
	}
}

func TestListenAndServeUnixCreatesOwnerOnlySocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "og.sock")
	listener, err := listenUnix(socketPath)
	if err != nil {
		t.Fatalf("listenUnix: %v", err)
	}
	defer func() { _ = listener.Close() }()

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("socket mode = %o, want 0600", got)
	}
}
