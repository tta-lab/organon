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

	readOut, writeOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = writeOut
	t.Cleanup(func() { os.Stdout = origStdout })

	cmd := newRootCmd()
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	if err := writeOut.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, readOut); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String(), execErr
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
