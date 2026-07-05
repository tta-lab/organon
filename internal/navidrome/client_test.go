package navidrome

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTokenAuthAddsSubsonicParameters(t *testing.T) {
	salt := "abc123"
	values := url.Values{}
	applyAuth(values, Config{
		Username:   "ooneil",
		Password:   "secret",
		Client:     "nd-playlist",
		APIVersion: "1.16.1",
	}, salt)

	sum := md5.Sum([]byte("secret" + salt))
	wantToken := hex.EncodeToString(sum[:])
	if values.Get("u") != "ooneil" || values.Get("s") != salt || values.Get("t") != wantToken {
		t.Fatalf("auth values = %v", values)
	}
	if values.Get("v") != "1.16.1" || values.Get("c") != "nd-playlist" || values.Get("f") != "json" {
		t.Fatalf("default API values = %v", values)
	}
	if strings.Contains(values.Encode(), "secret") {
		t.Fatalf("encoded auth leaked plaintext password: %s", values.Encode())
	}
}

func TestCreatePlaylistSendsRepeatedSongIDsWithPostForm(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/createPlaylist.view" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(subsonicResponse{Subsonic: responseStatus{Status: "ok"}})
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{
		Server:     server.URL,
		Username:   "u",
		Password:   "p",
		Client:     "nd-playlist",
		APIVersion: "1.16.1",
	})
	client.salt = func() string { return "salt" }

	err := client.CreateOrReplacePlaylist(
		context.Background(),
		"playlist-1",
		"Night",
		[]string{"s1", "s2", "s3"},
	)
	if err != nil {
		t.Fatalf("CreateOrReplacePlaylist: %v", err)
	}

	if got := gotForm["songId"]; strings.Join(got, ",") != "s1,s2,s3" {
		t.Fatalf("songId values = %v", got)
	}
	if gotForm.Get("playlistId") != "playlist-1" || gotForm.Get("name") != "Night" {
		t.Fatalf("playlist form = %v", gotForm)
	}
}

func TestGetSongUsesGetSongEndpointForPinnedIDs(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/getSong.view" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status": "ok",
				"song": map[string]string{
					"id": "song-1", "title": "Song", "artist": "Artist",
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{
		Server:     server.URL,
		Username:   "u",
		Password:   "p",
		Client:     "nd-playlist",
		APIVersion: "1.16.1",
	})
	client.salt = func() string { return "salt" }

	song, err := client.GetSong(context.Background(), "song-1")
	if err != nil {
		t.Fatalf("GetSong: %v", err)
	}
	if song.ID != "song-1" {
		t.Fatalf("song = %+v", song)
	}
	if gotQuery.Get("id") != "song-1" {
		t.Fatalf("query = %v", gotQuery)
	}
}
