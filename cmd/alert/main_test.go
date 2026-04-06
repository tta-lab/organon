package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipeStdin replaces os.Stdin with a pipe that contains content, runs fn, then restores.
func pipeStdin(t *testing.T, content []byte, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	old := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = old
		r.Close()
	}()
	fn()
}

// withEnv sets ALERT_ENDPOINT and runs fn.
func withEnv(t *testing.T, url string, fn func()) {
	t.Helper()
	require.NoError(t, os.Setenv("ALERT_ENDPOINT", url))
	defer func() { _ = os.Unsetenv("ALERT_ENDPOINT") }()
	fn()
}

// recvPayload parses JSON from an httptest handler.
func recvPayload(r *http.Request, v interface{}) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	_ = json.Unmarshal(body, v)
}

func TestAlert_HappyPath_PositionalArg(t *testing.T) {
	var received struct {
		Message string `json:"message"`
		From    string `json:"from"`
	}
	var contentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		recvPayload(r, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent", "test msg"})
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "test msg", received.Message)
	assert.Equal(t, "agent", received.From)
}

func TestAlert_HappyPath_Stdin(t *testing.T) {
	var received struct {
		Message string `json:"message"`
		From    string `json:"from"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvPayload(r, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "flick"})
		pipeStdin(t, []byte("the db is gone\n"), func() {
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	})
	assert.Equal(t, "the db is gone", received.Message)
	assert.Equal(t, "flick", received.From)
}

func TestAlert_MissingEndpoint(t *testing.T) {
	_ = os.Unsetenv("ALERT_ENDPOINT")
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--from", "agent", "hello"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALERT_ENDPOINT")
}

func TestAlert_MissingFrom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"test msg"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--from")
	})
}

func TestAlert_EmptyFrom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "", "test msg"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--from")
	})
}

func TestAlert_NoMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no message")
	})
}

func TestAlert_Non2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("something went wrong"))
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent", "boom"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "something went wrong")
	})
}

func TestAlert_EmptyStdin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent"})
		pipeStdin(t, []byte(""), func() {
			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no message")
		})
	})
}

func TestAlert_FromWithSpaces(t *testing.T) {
	var received struct {
		From string `json:"from"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvPayload(r, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "fn-agent", "hello"})
		err := cmd.Execute()
		assert.NoError(t, err)
		assert.Equal(t, "fn-agent", received.From)
	})
}

func TestAlert_StdinTrimsTrailingNewline(t *testing.T) {
	var received struct {
		Message string `json:"message"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvPayload(r, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "flick"})
		pipeStdin(t, []byte("the db is gone\n"), func() {
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	})
	assert.Equal(t, "the db is gone", received.Message)
	assert.False(t, strings.HasSuffix(received.Message, "\n"))
}

func TestAlert_Status400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
	}))
	defer srv.Close()

	withEnv(t, srv.URL, func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent", "boom"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "400")
	})
}

func TestAlert_ConnectionFailure(t *testing.T) {
	withEnv(t, "http://localhost:1/nowhere", func() {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--from", "agent", "hello"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request failed")
	})
}
