package navidrome

import (
	"os"
	"testing"
)

func TestExamplePlaylistSpecParses(t *testing.T) {
	file, err := os.Open("../../playlists/navidrome/example.yaml")
	if err != nil {
		t.Fatalf("open example: %v", err)
	}
	defer file.Close()

	spec, err := ReadSpec(file)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if spec.Name != "Example Playlist" {
		t.Fatalf("Name = %q", spec.Name)
	}
}
