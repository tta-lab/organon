package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefuddleBackend_FetchHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><head><title>Test Page</title></head><body><h1>Test</h1><p>content</p></body></html>"))
	}))
	defer srv.Close()

	backend := NewDefuddleCLIBackend()
	content, err := backend.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.NotContains(t, content, "<html>")
	assert.Contains(t, content, "## Test")
	assert.Contains(t, content, "content")
}

func TestDefuddleBackend_FetchNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"))
	}))
	defer srv.Close()

	backend := NewDefuddleCLIBackend()
	content, err := backend.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Contains(t, content, "package main")
	assert.Contains(t, content, "func main()")
}

func TestDefuddleBackend_FetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	backend := NewDefuddleCLIBackend()
	_, err := backend.Fetch(context.Background(), srv.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestDefuddleBackend_FetchLiveNonHTML_SkipOnCI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test in short mode")
	}

	backend := NewDefuddleCLIBackend()
	content, err := backend.Fetch(context.Background(), "https://raw.githubusercontent.com/tta-lab/organon/main/internal/fetch/doc.go")
	require.NoError(t, err)
	assert.Contains(t, content, "package fetch")
	assert.NotContains(t, content, "Not an HTML page")
}
