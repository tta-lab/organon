package navidrome

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetInternetRadioStations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/getInternetRadioStations.view" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subsonic-response": map[string]any{
				"status": "ok",
				"internetRadioStations": map[string]any{
					"internetRadioStation": []map[string]string{{
						"id": "radio-1", "name": "Lofi", "streamUrl": "https://example.test/lofi", "homePageUrl": "https://example.test",
					}},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{
		Server: server.URL, Username: "u", Password: "p", Client: "nd-playlist", APIVersion: "1.16.1",
	})
	stations, err := client.GetInternetRadioStations(context.Background())
	if err != nil {
		t.Fatalf("GetInternetRadioStations: %v", err)
	}
	if len(stations) != 1 || stations[0].ID != "radio-1" || stations[0].Name != "Lofi" {
		t.Fatalf("stations = %+v", stations)
	}
}

func TestCreateInternetRadioStationPostsFields(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/createInternetRadioStation.view" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]any{"subsonic-response": map[string]string{"status": "ok"}})
	}))
	t.Cleanup(server.Close)

	client := NewClient(Config{
		Server: server.URL, Username: "u", Password: "p", Client: "nd-playlist", APIVersion: "1.16.1",
	})
	err := client.CreateInternetRadioStation(context.Background(), RadioStation{
		Name: "Lofi", StreamURL: "https://example.test/lofi", HomePageURL: "https://example.test",
	})
	if err != nil {
		t.Fatalf("CreateInternetRadioStation: %v", err)
	}
	if gotForm.Get("name") != "Lofi" ||
		gotForm.Get("streamUrl") != "https://example.test/lofi" ||
		gotForm.Get("homepageUrl") != "https://example.test" {
		t.Fatalf("form = %v", gotForm)
	}
}
