package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runNDPlaylist(t *testing.T, args []string) (string, error) {
	t.Helper()
	stdout, _, err := runNDPlaylistFull(t, args)
	return stdout, err
}

func runNDPlaylistFull(t *testing.T, args []string) (stdout string, stderr string, err error) {
	t.Helper()

	readOut, writeOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	readErr, writeErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = writeOut
	os.Stderr = writeErr
	t.Cleanup(func() { os.Stdout = origStdout })
	t.Cleanup(func() { os.Stderr = origStderr })

	cmd := newRootCmd()
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	if err := writeOut.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	if err := writeErr.Close(); err != nil {
		t.Fatalf("close stderr: %v", err)
	}
	var outBuf bytes.Buffer
	if _, err := io.Copy(&outBuf, readOut); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	var errBuf bytes.Buffer
	if _, err := io.Copy(&errBuf, readErr); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return outBuf.String(), errBuf.String(), execErr
}

func writeConfig(t *testing.T, dir, server string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	content := `server = "` + server + `"
username = "ooneil"
password = "secret"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeSpec(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "playlist.yaml")
	content := `name: Night
public: false
tracks:
  - title: Song
    artist: Artist
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

func TestResolveJSONPrintsResolvedSongs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/search3.view" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status": "ok",
				"searchResult3": map[string]any{
					"song": []map[string]string{{
						"id": "song-1", "title": "Song", "artist": "Artist",
					}},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	tmp := t.TempDir()
	stdout, err := runNDPlaylist(t, []string{
		"--config", writeConfig(t, tmp, server.URL),
		"resolve", "--json", writeSpec(t, tmp),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.Contains(stdout, `"id": "song-1"`) {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestSearchJSONPrintsSongs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/search3.view" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "Song Artist" {
			t.Fatalf("query = %q", r.URL.Query().Get("query"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status": "ok",
				"searchResult3": map[string]any{
					"song": []map[string]string{{
						"id": "song-1", "title": "Song", "artist": "Artist",
					}},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	tmp := t.TempDir()
	stdout, err := runNDPlaylist(t, []string{
		"--config", writeConfig(t, tmp, server.URL),
		"search", "--json", "Song Artist",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(stdout, `"id": "song-1"`) {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestResolvePrintsMissingTrackDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status":        "ok",
				"searchResult3": map[string]any{"song": []map[string]string{}},
			},
		})
	}))
	t.Cleanup(server.Close)

	tmp := t.TempDir()
	_, stderr, err := runNDPlaylistFull(t, []string{
		"--config", writeConfig(t, tmp, server.URL),
		"resolve", writeSpec(t, tmp),
	})
	if err == nil {
		t.Fatal("resolve succeeded, want missing-track error")
	}
	for _, want := range []string{"missing tracks:", "title=Song", "artist=Artist"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %s", want, stderr)
		}
	}
}

func TestResolveJSONPrintsAmbiguousCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status": "ok",
				"searchResult3": map[string]any{
					"song": []map[string]string{
						{"id": "a", "title": "Song", "artist": "Artist"},
						{"id": "b", "title": "Song", "artist": "Artist"},
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	tmp := t.TempDir()
	stdout, _, err := runNDPlaylistFull(t, []string{
		"--config", writeConfig(t, tmp, server.URL),
		"resolve", "--json", writeSpec(t, tmp),
	})
	if err == nil {
		t.Fatal("resolve succeeded, want ambiguous-track error")
	}
	for _, want := range []string{`"ambiguous"`, `"id": "a"`, `"id": "b"`, `"title": "Song"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestApplyDryRunDoesNotMutate(t *testing.T) {
	var mutateCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/search3.view":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"subsonic-response": map[string]any{
					"status": "ok",
					"searchResult3": map[string]any{
						"song": []map[string]string{{"id": "song-1", "title": "Song", "artist": "Artist"}},
					},
				},
			})
		case "/rest/getPlaylists.view":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"subsonic-response": map[string]any{
					"status":    "ok",
					"playlists": map[string]any{"playlist": []map[string]string{}},
				},
			})
		case "/rest/createPlaylist.view", "/rest/updatePlaylist.view":
			mutateCalled = true
			t.Fatalf("dry-run called mutation endpoint %s", r.URL.Path)
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	tmp := t.TempDir()
	stdout, err := runNDPlaylist(t, []string{
		"--config", writeConfig(t, tmp, server.URL),
		"apply", "--dry-run", writeSpec(t, tmp),
	})
	if err != nil {
		t.Fatalf("apply --dry-run: %v", err)
	}
	if mutateCalled {
		t.Fatal("mutation endpoint was called")
	}
	if !strings.Contains(stdout, "would create playlist") {
		t.Fatalf("stdout = %s", stdout)
	}
}
