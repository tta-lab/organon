package navidrome

import (
	"fmt"
)

// PlaylistSpec is the YAML playlist-as-code format.
type PlaylistSpec struct {
	Name        string      `yaml:"name" json:"name"`
	NavidromeID string      `yaml:"navidrome_id,omitempty" json:"navidrome_id,omitempty"`
	Comment     string      `yaml:"comment,omitempty" json:"comment,omitempty"`
	Public      *bool       `yaml:"public,omitempty" json:"public,omitempty"`
	Tracks      []TrackSpec `yaml:"tracks" json:"tracks"`
}

// TrackSpec identifies a desired song. ID pins server identity when present.
type TrackSpec struct {
	ID     string `yaml:"id,omitempty" json:"id,omitempty"`
	Title  string `yaml:"title" json:"title"`
	Artist string `yaml:"artist" json:"artist"`
	Album  string `yaml:"album,omitempty" json:"album,omitempty"`
}

// Song is the subset of a Subsonic song needed for playlist operations.
type Song struct {
	ID     string `json:"id" yaml:"id"`
	Title  string `json:"title" yaml:"title"`
	Artist string `json:"artist" yaml:"artist"`
	Album  string `json:"album,omitempty" yaml:"album,omitempty"`
}

// Playlist is the subset of a Subsonic playlist needed for selection.
type Playlist struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Public  bool   `json:"public"`
	Comment string `json:"comment"`
}

// TrackDiff summarizes full-replacement playlist changes.
type TrackDiff struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Reordered int `json:"reordered"`
}

// HasChanges reports whether the desired playlist differs from current state.
func (d TrackDiff) HasChanges() bool {
	return d.Added != 0 || d.Removed != 0 || d.Reordered != 0
}

// DiffTracks compares current server order to desired spec order.
func DiffTracks(current, desired []Song) TrackDiff {
	currentPos := make(map[string]int, len(current))
	desiredPos := make(map[string]int, len(desired))
	for i, song := range current {
		currentPos[song.ID] = i
	}
	for i, song := range desired {
		desiredPos[song.ID] = i
	}

	var diff TrackDiff
	for _, song := range desired {
		if _, ok := currentPos[song.ID]; !ok {
			diff.Added++
		}
	}
	for _, song := range current {
		pos, ok := desiredPos[song.ID]
		if !ok {
			diff.Removed++
			continue
		}
		if currentPos[song.ID] != pos {
			diff.Reordered++
		}
	}
	return diff
}

// ChoosePlaylist resolves the target server playlist for a spec.
func ChoosePlaylist(spec PlaylistSpec, playlists []Playlist, owner string) (Playlist, error) {
	if spec.NavidromeID != "" {
		for _, playlist := range playlists {
			if playlist.ID == spec.NavidromeID {
				return playlist, nil
			}
		}
		return Playlist{}, fmt.Errorf("playlist id %q not found", spec.NavidromeID)
	}

	var matches []Playlist
	for _, playlist := range playlists {
		if playlist.Name == spec.Name && (owner == "" || playlist.Owner == "" || playlist.Owner == owner) {
			matches = append(matches, playlist)
		}
	}
	switch len(matches) {
	case 0:
		return Playlist{}, ErrNotFound
	case 1:
		return matches[0], nil
	default:
		return Playlist{}, fmt.Errorf("multiple playlists named %q found for owner %q; set navidrome_id", spec.Name, owner)
	}
}
