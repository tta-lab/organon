package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefuddleBackend_FetchHTML(t *testing.T) {
	if _, err := exec.LookPath("defuddle"); err != nil {
		t.Skip("defuddle not installed")
	}

	htmlBody := "<html><head><title>T</title></head><body><h1>H</h1><p>content</p></body></html>"

	tests := []struct {
		name        string
		contentType string
		wantHTML    bool
	}{
		{name: "lowercase", contentType: "text/html; charset=utf-8", wantHTML: true},
		{name: "mixed case", contentType: "Text/HTML; charset=utf-8", wantHTML: true},
		{name: "uppercase", contentType: "TEXT/HTML", wantHTML: true},
		{name: "missing", contentType: "", wantHTML: true},
		{name: "plain text", contentType: "text/plain", wantHTML: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				_, _ = w.Write([]byte(htmlBody))
			}))
			defer srv.Close()

			backend := NewDefuddleCLIBackend()
			content, err := backend.Fetch(context.Background(), srv.URL)
			require.NoError(t, err)

			if tt.wantHTML {
				assert.NotContains(t, content, "<html>")
				assert.Contains(t, content, "## H")
				assert.Contains(t, content, "content")
			} else {
				assert.Contains(t, content, "<html>")
			}
		})
	}
}

func TestDefuddleBackend_FetchNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"))
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
	if testing.Short() || os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping live network test in short mode or CI")
	}

	url := "https://raw.githubusercontent.com/tta-lab/organon/main/internal/fetch/doc.go"
	backend := NewDefuddleCLIBackend()
	content, err := backend.Fetch(context.Background(), url)
	require.NoError(t, err)
	assert.Contains(t, content, "package fetch")
	assert.NotContains(t, content, "Not an HTML page")
}
