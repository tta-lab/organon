package navidrome

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadSpecValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing name", content: "tracks:\n  - id: song-1\n"},
		{name: "missing tracks", content: "name: Night\n"},
		{name: "track needs id or title artist", content: "name: Night\ntracks:\n  - title: Song\n"},
		{name: "malformed yaml", content: "name: [\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ReadSpec(strings.NewReader(tt.content)); err == nil {
				t.Fatal("ReadSpec succeeded, want error")
			}
		})
	}
}

func TestWriteSpecRoundTrips(t *testing.T) {
	public := false
	in := PlaylistSpec{
		Name:        "Night",
		NavidromeID: "playlist-1",
		Comment:     "slow songs",
		Public:      &public,
		Tracks: []TrackSpec{{
			ID:     "song-1",
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
		}},
	}

	var buf bytes.Buffer
	if err := WriteSpec(&buf, in); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}
	out, err := ReadSpec(&buf)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if out.Name != in.Name || out.NavidromeID != in.NavidromeID || out.Comment != in.Comment {
		t.Fatalf("round trip = %+v", out)
	}
	if out.Public == nil || *out.Public != false {
		t.Fatalf("Public = %v", out.Public)
	}
	if len(out.Tracks) != 1 || out.Tracks[0].ID != "song-1" || out.Tracks[0].Album != "Album" {
		t.Fatalf("Tracks = %+v", out.Tracks)
	}
}
