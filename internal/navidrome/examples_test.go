package navidrome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaylistSpecsParse(t *testing.T) {
	paths, err := filepath.Glob("../../playlists/navidrome/*.yaml")
	if err != nil {
		t.Fatalf("glob playlist specs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no playlist specs found")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open spec: %v", err)
			}
			defer file.Close()

			if _, err := ReadSpec(file); err != nil {
				t.Fatalf("ReadSpec: %v", err)
			}
		})
	}
}
