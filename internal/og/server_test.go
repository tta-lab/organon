package og

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMuxExposesCloneRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/git/clone", bytes.NewReader([]byte(`{}`)))
	resp := httptest.NewRecorder()

	NewMux(Service{}).ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatal("clone route is not registered")
	}
}

func TestHTTPHandlerPropagatesRequestContext(t *testing.T) {
	type key string
	ctx := context.WithValue(context.Background(), key("request"), "present")
	handler := HTTPHandler(func(req Request) (Response, error) {
		if req.Context == nil || req.Context.Value(key("request")) != "present" {
			t.Fatalf("request context = %#v", req.Context)
		}
		return Response{Message: "accepted"}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/git/clone", bytes.NewReader([]byte(`{}`))).WithContext(ctx)
	resp := httptest.NewRecorder()

	handler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestHTTPHandlerRejectsTokenFields(t *testing.T) {
	handler := HTTPHandler(func(req Request) (Response, error) {
		return Response{Message: "accepted"}, nil
	})
	body := []byte(`{"work_dir":"/tmp/repo","token":"secret"}`)
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

func TestMuxDoesNotExposePRMergeRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/pr/merge", bytes.NewReader([]byte(`{}`)))
	resp := httptest.NewRecorder()

	NewMux(Service{}).ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
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

func TestListenAndServeUnixReadyRunsCallbackAfterSocketBound(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "og.sock")
	ready := make(chan struct{}, 1)
	errc := make(chan error, 1)

	go func() {
		errc <- ListenAndServeUnixReady(socketPath, http.NewServeMux(), func() {
			if conn, err := net.Dial("unix", socketPath); err != nil {
				t.Errorf("dial ready socket: %v", err)
			} else {
				_ = conn.Close()
			}
			ready <- struct{}{}
		})
	}()

	select {
	case <-ready:
	case err := <-errc:
		t.Fatalf("ListenAndServeUnixReady returned before ready: %v", err)
	}
}
