package navidrome

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// ReadSpec reads and validates a playlist YAML spec.
func ReadSpec(r io.Reader) (PlaylistSpec, error) {
	var spec PlaylistSpec
	if err := yaml.NewDecoder(r).Decode(&spec); err != nil {
		return PlaylistSpec{}, err
	}
	if spec.Name == "" {
		return PlaylistSpec{}, fmt.Errorf("playlist name is required")
	}
	if len(spec.Tracks) == 0 {
		return PlaylistSpec{}, fmt.Errorf("playlist tracks are required")
	}
	for i, track := range spec.Tracks {
		if track.ID == "" && (track.Title == "" || track.Artist == "") {
			return PlaylistSpec{}, fmt.Errorf("track %d needs id or title and artist", i+1)
		}
	}
	return spec, nil
}

// WriteSpec writes a playlist YAML spec.
func WriteSpec(w io.Writer, spec PlaylistSpec) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(spec); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}
